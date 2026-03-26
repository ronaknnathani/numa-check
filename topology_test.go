package main

import (
	"fmt"
	"testing"
)

// mockFS implements FileSystem using in-memory maps for testing.
type mockFS struct {
	files map[string]string
	globs map[string][]string
}

func (m *mockFS) ReadFile(path string) ([]byte, error) {
	if content, ok := m.files[path]; ok {
		return []byte(content), nil
	}
	return nil, fmt.Errorf("file not found: %s", path)
}

func (m *mockFS) Glob(pattern string) ([]string, error) {
	if matches, ok := m.globs[pattern]; ok {
		return matches, nil
	}
	return nil, nil
}

func TestBuildNUMANodesWithFS(t *testing.T) {
	t.Run("two node setup", func(t *testing.T) {
		fs := &mockFS{
			files: map[string]string{
				"/sys/devices/system/cpu/cpu0/topology/physical_package_id": "0",
				"/sys/devices/system/cpu/cpu0/topology/core_id":             "0",
				"/sys/devices/system/cpu/cpu1/topology/physical_package_id": "0",
				"/sys/devices/system/cpu/cpu1/topology/core_id":             "1",
				"/sys/devices/system/cpu/cpu2/topology/physical_package_id": "1",
				"/sys/devices/system/cpu/cpu2/topology/core_id":             "0",
				"/sys/devices/system/cpu/cpu3/topology/physical_package_id": "1",
				"/sys/devices/system/cpu/cpu3/topology/core_id":             "1",
			},
		}
		numaMap := map[int]int{0: 0, 1: 0, 2: 1, 3: 1}

		nodes := buildNUMANodes(fs, numaMap, nil)
		if len(nodes) != 2 {
			t.Fatalf("expected 2 nodes, got %d", len(nodes))
		}
		// Sorted by ID.
		if nodes[0].ID != 0 || nodes[1].ID != 1 {
			t.Errorf("nodes not sorted: IDs = %d, %d", nodes[0].ID, nodes[1].ID)
		}
		// Node 0 should have CPUs 0,1.
		if len(nodes[0].CPUs) != 2 || nodes[0].CPUs[0] != 0 || nodes[0].CPUs[1] != 1 {
			t.Errorf("node 0 CPUs = %v, want [0 1]", nodes[0].CPUs)
		}
		// Node 1 should have CPUs 2,3.
		if len(nodes[1].CPUs) != 2 || nodes[1].CPUs[0] != 2 || nodes[1].CPUs[1] != 3 {
			t.Errorf("node 1 CPUs = %v, want [2 3]", nodes[1].CPUs)
		}
		// Socket IDs from topology files.
		if nodes[0].SocketID != 0 {
			t.Errorf("node 0 SocketID = %d, want 0", nodes[0].SocketID)
		}
		if nodes[1].SocketID != 1 {
			t.Errorf("node 1 SocketID = %d, want 1", nodes[1].SocketID)
		}
	})

	t.Run("GPU assigned to correct node", func(t *testing.T) {
		fs := &mockFS{
			files: map[string]string{
				"/sys/devices/system/cpu/cpu0/topology/physical_package_id": "0",
				"/sys/devices/system/cpu/cpu0/topology/core_id":             "0",
				"/sys/devices/system/cpu/cpu2/topology/physical_package_id": "1",
				"/sys/devices/system/cpu/cpu2/topology/core_id":             "0",
			},
		}
		numaMap := map[int]int{0: 0, 2: 1}
		gpus := []GPUDevice{
			{Index: 0, UUID: "GPU-aaa", PCIID: "0000:3b:00.0", NUMANode: 0},
		}

		nodes := buildNUMANodes(fs, numaMap, gpus)
		if len(nodes) != 2 {
			t.Fatalf("expected 2 nodes, got %d", len(nodes))
		}
		if len(nodes[0].GPUs) != 1 {
			t.Errorf("node 0 should have 1 GPU, got %d", len(nodes[0].GPUs))
		}
		if len(nodes[1].GPUs) != 0 {
			t.Errorf("node 1 should have 0 GPUs, got %d", len(nodes[1].GPUs))
		}
	})

	t.Run("GPU with unknown NUMA goes to all nodes", func(t *testing.T) {
		fs := &mockFS{
			files: map[string]string{
				"/sys/devices/system/cpu/cpu0/topology/physical_package_id": "0",
				"/sys/devices/system/cpu/cpu0/topology/core_id":             "0",
				"/sys/devices/system/cpu/cpu2/topology/physical_package_id": "1",
				"/sys/devices/system/cpu/cpu2/topology/core_id":             "0",
			},
		}
		numaMap := map[int]int{0: 0, 2: 1}
		gpus := []GPUDevice{
			{Index: 0, UUID: "GPU-orphan", PCIID: "0000:3b:00.0", NUMANode: -1},
		}

		nodes := buildNUMANodes(fs, numaMap, gpus)
		if len(nodes) != 2 {
			t.Fatalf("expected 2 nodes, got %d", len(nodes))
		}
		// GPU with NUMANode=-1 (not matching any node) should appear in all nodes.
		if len(nodes[0].GPUs) != 1 {
			t.Errorf("node 0 should have 1 GPU (broadcast), got %d", len(nodes[0].GPUs))
		}
		if len(nodes[1].GPUs) != 1 {
			t.Errorf("node 1 should have 1 GPU (broadcast), got %d", len(nodes[1].GPUs))
		}
	})

	t.Run("missing topology falls back to socket -1", func(t *testing.T) {
		fs := &mockFS{
			files: map[string]string{},
		}
		numaMap := map[int]int{0: 0}

		nodes := buildNUMANodes(fs, numaMap, nil)
		if len(nodes) != 1 {
			t.Fatalf("expected 1 node, got %d", len(nodes))
		}
		if nodes[0].SocketID != -1 {
			t.Errorf("expected SocketID=-1 when topology missing, got %d", nodes[0].SocketID)
		}
	})
}

func TestDetectNVIDIAGPUsPCI(t *testing.T) {
	t.Run("detects NVIDIA 3D controller", func(t *testing.T) {
		fs := &mockFS{
			globs: map[string][]string{
				"/sys/bus/pci/devices/*": {
					"/sys/bus/pci/devices/0000:3b:00.0",
				},
			},
			files: map[string]string{
				"/sys/bus/pci/devices/0000:3b:00.0/vendor": "0x10de\n",
				"/sys/bus/pci/devices/0000:3b:00.0/class":  "0x030200\n",
			},
		}
		got := detectNVIDIAGPUsPCI(fs)
		if len(got) != 1 || got[0] != "0000:3b:00.0" {
			t.Errorf("detectNVIDIAGPUsPCI() = %v, want [0000:3b:00.0]", got)
		}
	})

	t.Run("detects NVIDIA VGA controller", func(t *testing.T) {
		fs := &mockFS{
			globs: map[string][]string{
				"/sys/bus/pci/devices/*": {
					"/sys/bus/pci/devices/0000:86:00.0",
				},
			},
			files: map[string]string{
				"/sys/bus/pci/devices/0000:86:00.0/vendor": "0x10de\n",
				"/sys/bus/pci/devices/0000:86:00.0/class":  "0x030000\n",
			},
		}
		got := detectNVIDIAGPUsPCI(fs)
		if len(got) != 1 || got[0] != "0000:86:00.0" {
			t.Errorf("detectNVIDIAGPUsPCI() = %v, want [0000:86:00.0]", got)
		}
	})

	t.Run("skips non-NVIDIA vendor", func(t *testing.T) {
		fs := &mockFS{
			globs: map[string][]string{
				"/sys/bus/pci/devices/*": {
					"/sys/bus/pci/devices/0000:00:02.0",
				},
			},
			files: map[string]string{
				"/sys/bus/pci/devices/0000:00:02.0/vendor": "0x8086\n",
				"/sys/bus/pci/devices/0000:00:02.0/class":  "0x030000\n",
			},
		}
		got := detectNVIDIAGPUsPCI(fs)
		if len(got) != 0 {
			t.Errorf("detectNVIDIAGPUsPCI() = %v, want empty", got)
		}
	})

	t.Run("skips NVIDIA audio device", func(t *testing.T) {
		fs := &mockFS{
			globs: map[string][]string{
				"/sys/bus/pci/devices/*": {
					"/sys/bus/pci/devices/0000:3b:00.1",
				},
			},
			files: map[string]string{
				"/sys/bus/pci/devices/0000:3b:00.1/vendor": "0x10de\n",
				"/sys/bus/pci/devices/0000:3b:00.1/class":  "0x040300\n",
			},
		}
		got := detectNVIDIAGPUsPCI(fs)
		if len(got) != 0 {
			t.Errorf("detectNVIDIAGPUsPCI() = %v, want empty (audio device)", got)
		}
	})

	t.Run("mixed devices filters correctly", func(t *testing.T) {
		fs := &mockFS{
			globs: map[string][]string{
				"/sys/bus/pci/devices/*": {
					"/sys/bus/pci/devices/0000:00:02.0",
					"/sys/bus/pci/devices/0000:3b:00.0",
					"/sys/bus/pci/devices/0000:3b:00.1",
					"/sys/bus/pci/devices/0000:86:00.0",
				},
			},
			files: map[string]string{
				// Intel GPU — skipped.
				"/sys/bus/pci/devices/0000:00:02.0/vendor": "0x8086\n",
				"/sys/bus/pci/devices/0000:00:02.0/class":  "0x030000\n",
				// NVIDIA 3D controller — included.
				"/sys/bus/pci/devices/0000:3b:00.0/vendor": "0x10de\n",
				"/sys/bus/pci/devices/0000:3b:00.0/class":  "0x030200\n",
				// NVIDIA audio — skipped.
				"/sys/bus/pci/devices/0000:3b:00.1/vendor": "0x10de\n",
				"/sys/bus/pci/devices/0000:3b:00.1/class":  "0x040300\n",
				// NVIDIA VGA — included.
				"/sys/bus/pci/devices/0000:86:00.0/vendor": "0x10de\n",
				"/sys/bus/pci/devices/0000:86:00.0/class":  "0x030000\n",
			},
		}
		got := detectNVIDIAGPUsPCI(fs)
		if len(got) != 2 {
			t.Fatalf("detectNVIDIAGPUsPCI() returned %d devices, want 2: %v", len(got), got)
		}
		if got[0] != "0000:3b:00.0" || got[1] != "0000:86:00.0" {
			t.Errorf("detectNVIDIAGPUsPCI() = %v, want [0000:3b:00.0 0000:86:00.0]", got)
		}
	})

	t.Run("no PCI devices", func(t *testing.T) {
		fs := &mockFS{
			globs: map[string][]string{
				"/sys/bus/pci/devices/*": {},
			},
		}
		got := detectNVIDIAGPUsPCI(fs)
		if len(got) != 0 {
			t.Errorf("detectNVIDIAGPUsPCI() = %v, want empty", got)
		}
	})
}

func TestSortAndIndexGPUs(t *testing.T) {
	t.Run("sorts by NUMA then PCI and assigns indices", func(t *testing.T) {
		gpus := []GPUDevice{
			{UUID: "GPU-ccc", PCIID: "0000:86:00.0", NUMANode: 1},
			{UUID: "GPU-aaa", PCIID: "0000:3b:00.0", NUMANode: 0},
			{UUID: "GPU-bbb", PCIID: "0000:af:00.0", NUMANode: 0},
			{UUID: "GPU-ddd", PCIID: "0000:41:00.0", NUMANode: 1},
		}
		sortAndIndexGPUs(gpus)

		// Expected order: node 0 (3b, af), node 1 (41, 86).
		expected := []struct {
			uuid     string
			pciID    string
			numaNode int
			index    int
		}{
			{"GPU-aaa", "0000:3b:00.0", 0, 0},
			{"GPU-bbb", "0000:af:00.0", 0, 1},
			{"GPU-ddd", "0000:41:00.0", 1, 2},
			{"GPU-ccc", "0000:86:00.0", 1, 3},
		}

		for i, exp := range expected {
			if gpus[i].UUID != exp.uuid {
				t.Errorf("gpu[%d].UUID = %q, want %q", i, gpus[i].UUID, exp.uuid)
			}
			if gpus[i].PCIID != exp.pciID {
				t.Errorf("gpu[%d].PCIID = %q, want %q", i, gpus[i].PCIID, exp.pciID)
			}
			if gpus[i].NUMANode != exp.numaNode {
				t.Errorf("gpu[%d].NUMANode = %d, want %d", i, gpus[i].NUMANode, exp.numaNode)
			}
			if gpus[i].Index != exp.index {
				t.Errorf("gpu[%d].Index = %d, want %d", i, gpus[i].Index, exp.index)
			}
		}
	})

	t.Run("single GPU gets index 0", func(t *testing.T) {
		gpus := []GPUDevice{
			{UUID: "GPU-solo", PCIID: "0000:3b:00.0", NUMANode: 0},
		}
		sortAndIndexGPUs(gpus)
		if gpus[0].Index != 0 {
			t.Errorf("single GPU index = %d, want 0", gpus[0].Index)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		gpus := []GPUDevice{}
		sortAndIndexGPUs(gpus) // should not panic
	})
}

