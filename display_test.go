package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout runs fn while redirecting os.Stdout to a pipe and returns the captured output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan string)
	go func() {
		var sb strings.Builder
		_, _ = io.Copy(&sb, r)
		done <- sb.String()
	}()

	fn()
	_ = w.Close()
	return <-done
}

func TestNodeHeader(t *testing.T) {
	tests := []struct {
		name string
		node NUMANodeInfo
		want string
	}{
		{
			name: "with socket ID",
			node: NUMANodeInfo{ID: 0, SocketID: 1},
			want: "NUMA Node 0 — Socket 1",
		},
		{
			name: "without socket ID",
			node: NUMANodeInfo{ID: 2, SocketID: -1},
			want: "NUMA Node 2",
		},
		{
			name: "socket zero",
			node: NUMANodeInfo{ID: 0, SocketID: 0},
			want: "NUMA Node 0 — Socket 0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nodeHeader(&tt.node)
			if got != tt.want {
				t.Errorf("nodeHeader() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCPUFooter(t *testing.T) {
	useColor = false

	tests := []struct {
		name       string
		node       NUMANodeInfo
		mode       DisplayMode
		allowedSet map[int]bool
		want       string
	}{
		{
			name:       "machine mode shows range",
			node:       NUMANodeInfo{CPUs: []int{0, 1, 2, 3}},
			mode:       ModeMachine,
			allowedSet: nil,
			want:       "4 CPUs (0–3)",
		},
		{
			name:       "process mode shows count",
			node:       NUMANodeInfo{CPUs: []int{0, 1, 2, 3}},
			mode:       ModeProcess,
			allowedSet: map[int]bool{0: true, 2: true},
			want:       "2 of 4 CPUs",
		},
		{
			name:       "process mode all allowed",
			node:       NUMANodeInfo{CPUs: []int{0, 1}},
			mode:       ModeProcess,
			allowedSet: map[int]bool{0: true, 1: true},
			want:       "2 of 2 CPUs",
		},
		{
			name:       "process mode none allowed",
			node:       NUMANodeInfo{CPUs: []int{4, 5, 6, 7}},
			mode:       ModeProcess,
			allowedSet: map[int]bool{0: true, 1: true},
			want:       "0 of 4 CPUs",
		},
		{
			name:       "empty CPUs does not panic",
			node:       NUMANodeInfo{CPUs: nil},
			mode:       ModeMachine,
			allowedSet: nil,
			want:       "0 CPUs",
		},
		{
			name:       "empty CPUs process mode",
			node:       NUMANodeInfo{CPUs: []int{}},
			mode:       ModeProcess,
			allowedSet: map[int]bool{0: true},
			want:       "0 CPUs",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cpuFooter(&tt.node, tt.mode, tt.allowedSet)
			if got != tt.want {
				t.Errorf("cpuFooter() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMemoryFooter(t *testing.T) {
	useColor = false
	t.Cleanup(func() { useColor = false })

	tests := []struct {
		name       string
		node       NUMANodeInfo
		mode       DisplayMode
		processMem map[int]int64
		want       string
	}{
		{
			name: "machine mode with memory",
			node: NUMANodeInfo{ID: 0, MemTotalBytes: 256 * 1024 * 1024 * 1024, MemFreeBytes: 240 * 1024 * 1024 * 1024},
			mode: ModeMachine,
			want: "256.0 GiB total, 240.0 GiB free",
		},
		{
			name: "machine mode with unknown memory",
			node: NUMANodeInfo{ID: 0, MemTotalBytes: 0},
			mode: ModeMachine,
			want: "memory: unknown",
		},
		{
			name:       "process mode with per-node memory",
			node:       NUMANodeInfo{ID: 0, MemTotalBytes: 256 * 1024 * 1024 * 1024},
			mode:       ModeProcess,
			processMem: map[int]int64{0: 1536 * 1024 * 1024},
			want:       "1.5 GiB on this node / 256.0 GiB",
		},
		{
			name:       "process mode with zero usage on node",
			node:       NUMANodeInfo{ID: 1, MemTotalBytes: 256 * 1024 * 1024 * 1024},
			mode:       ModeProcess,
			processMem: map[int]int64{0: 1536 * 1024 * 1024},
			want:       "0 B on this node / 256.0 GiB",
		},
		{
			name:       "process mode with no process-mem data",
			node:       NUMANodeInfo{ID: 0, MemTotalBytes: 256 * 1024 * 1024 * 1024},
			mode:       ModeProcess,
			processMem: nil,
			want:       "256.0 GiB total",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := memoryFooter(&tt.node, tt.mode, tt.processMem)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGPUFooter(t *testing.T) {
	useColor = false

	tests := []struct {
		name string
		node NUMANodeInfo
		want string
	}{
		{
			name: "with GPUs",
			node: NUMANodeInfo{GPUs: []GPUDevice{{Index: 0}, {Index: 1}, {Index: 2}}},
			want: "3 GPUs",
		},
		{
			name: "no GPUs",
			node: NUMANodeInfo{GPUs: nil},
			want: "",
		},
		{
			name: "single GPU",
			node: NUMANodeInfo{GPUs: []GPUDevice{{Index: 0}}},
			want: "1 GPUs",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gpuFooter(&tt.node)
			if got != tt.want {
				t.Errorf("gpuFooter() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGridRowVisualWidth(t *testing.T) {
	tests := []struct {
		name string
		cpus []int
		row  int
		want int
	}{
		{
			name: "full row of 16",
			cpus: make([]int, 16),
			row:  0,
			want: 31, // 16*2-1
		},
		{
			name: "partial row 4 cpus",
			cpus: make([]int, 20),
			row:  1,
			want: 7, // 4*2-1
		},
		{
			name: "empty cpus",
			cpus: nil,
			row:  0,
			want: 0,
		},
		{
			name: "row beyond cpus",
			cpus: make([]int, 8),
			row:  1,
			want: 0,
		},
		{
			name: "exactly two rows",
			cpus: make([]int, 32),
			row:  1,
			want: 31,
		},
		{
			name: "single cpu",
			cpus: make([]int, 1),
			row:  0,
			want: 1, // 1*2-1
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gridRowVisualWidth(tt.cpus, tt.row)
			if got != tt.want {
				t.Errorf("gridRowVisualWidth(cpus[%d], row=%d) = %d, want %d",
					len(tt.cpus), tt.row, got, tt.want)
			}
		})
	}
}

func TestPad(t *testing.T) {
	tests := []struct {
		name  string
		s     string
		width int
		want  string
	}{
		{
			name:  "shorter than width",
			s:     "hi",
			width: 5,
			want:  "hi   ",
		},
		{
			name:  "exact width",
			s:     "hello",
			width: 5,
			want:  "hello",
		},
		{
			name:  "longer than width",
			s:     "hello world",
			width: 5,
			want:  "hello world",
		},
		{
			name:  "empty string",
			s:     "",
			width: 3,
			want:  "   ",
		},
		{
			name:  "width zero",
			s:     "hi",
			width: 0,
			want:  "hi",
		},
		{
			name:  "unicode string",
			s:     "—",
			width: 4,
			want:  "—   ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pad(tt.s, tt.width)
			if got != tt.want {
				t.Errorf("pad(%q, %d) = %q, want %q", tt.s, tt.width, got, tt.want)
			}
		})
	}
}

func TestRenderGrid(t *testing.T) {
	useColor = false

	t.Run("machine mode all filled", func(t *testing.T) {
		cpus := []int{0, 1, 2, 3}
		rows := renderGrid(cpus, ModeMachine, nil, -1)
		if len(rows) != 1 {
			t.Fatalf("expected 1 row, got %d", len(rows))
		}
		// In machine mode with color off, every CPU is "■".
		expected := "■ ■ ■ ■"
		if rows[0] != expected {
			t.Errorf("renderGrid machine = %q, want %q", rows[0], expected)
		}
	})

	t.Run("process mode mixed", func(t *testing.T) {
		cpus := []int{0, 1, 2, 3}
		allowedSet := map[int]bool{0: true, 2: true}
		currentCPU := 2
		rows := renderGrid(cpus, ModeProcess, allowedSet, currentCPU)
		if len(rows) != 1 {
			t.Fatalf("expected 1 row, got %d", len(rows))
		}
		// CPU 0: allowed (■), CPU 1: not allowed (□), CPU 2: current (★), CPU 3: not allowed (□)
		expected := "■ □ ★ □"
		if rows[0] != expected {
			t.Errorf("renderGrid process = %q, want %q", rows[0], expected)
		}
	})

	t.Run("multiple rows", func(t *testing.T) {
		cpus := make([]int, 20)
		for i := range cpus {
			cpus[i] = i
		}
		rows := renderGrid(cpus, ModeMachine, nil, -1)
		if len(rows) != 2 {
			t.Fatalf("expected 2 rows, got %d", len(rows))
		}
		// First row: 16 CPUs.
		if count := strings.Count(rows[0], "■"); count != 16 {
			t.Errorf("first row has %d squares, want 16", count)
		}
		// Second row: 4 CPUs.
		if count := strings.Count(rows[1], "■"); count != 4 {
			t.Errorf("second row has %d squares, want 4", count)
		}
	})

	t.Run("empty cpus", func(t *testing.T) {
		rows := renderGrid(nil, ModeMachine, nil, -1)
		if len(rows) != 0 {
			t.Errorf("expected 0 rows, got %d", len(rows))
		}
	})
}

func TestRenderGPURows(t *testing.T) {
	useColor = false

	t.Run("machine mode", func(t *testing.T) {
		gpus := []GPUDevice{
			{Index: 0, UUID: "GPU-aaa", NUMANode: 0},
			{Index: 1, UUID: "GPU-bbb", NUMANode: 0},
		}
		rows, widths := renderGPURows(gpus, ModeMachine, 0, nil, nil)
		if len(rows) != 1 {
			t.Fatalf("expected 1 row, got %d", len(rows))
		}
		// Two GPUs on one row, each "▀▀ GPU N".
		if !strings.Contains(rows[0], "GPU 0") || !strings.Contains(rows[0], "GPU 1") {
			t.Errorf("expected GPU 0 and GPU 1 in row, got %q", rows[0])
		}
		if widths[0] <= 0 {
			t.Errorf("expected positive width, got %d", widths[0])
		}
	})

	t.Run("process mode allowed GPU", func(t *testing.T) {
		gpus := []GPUDevice{
			{Index: 0, UUID: "GPU-aaa", NUMANode: 0},
		}
		processNodes := map[int]bool{0: true}
		allowedGPUs := map[string]bool{"GPU-aaa": true}
		rows, _ := renderGPURows(gpus, ModeProcess, 0, processNodes, allowedGPUs)
		if len(rows) != 1 {
			t.Fatalf("expected 1 row, got %d", len(rows))
		}
		if !strings.Contains(rows[0], "GPU 0") {
			t.Errorf("expected GPU 0 in row, got %q", rows[0])
		}
	})

	t.Run("process mode disallowed GPU", func(t *testing.T) {
		gpus := []GPUDevice{
			{Index: 0, UUID: "GPU-aaa", NUMANode: 0},
			{Index: 1, UUID: "GPU-bbb", NUMANode: 0},
		}
		processNodes := map[int]bool{0: true}
		allowedGPUs := map[string]bool{"GPU-aaa": true} // GPU-bbb not allowed
		rows, _ := renderGPURows(gpus, ModeProcess, 0, processNodes, allowedGPUs)
		if len(rows) != 1 {
			t.Fatalf("expected 1 row, got %d", len(rows))
		}
		// Both GPUs should appear in the output regardless of allowed status.
		if !strings.Contains(rows[0], "GPU 0") || !strings.Contains(rows[0], "GPU 1") {
			t.Errorf("expected both GPUs in row, got %q", rows[0])
		}
	})

	t.Run("no GPUs", func(t *testing.T) {
		rows, widths := renderGPURows(nil, ModeMachine, 0, nil, nil)
		if rows != nil {
			t.Errorf("expected nil rows, got %v", rows)
		}
		if widths != nil {
			t.Errorf("expected nil widths, got %v", widths)
		}
	})

	t.Run("GPU with unknown NUMA shows question mark", func(t *testing.T) {
		gpus := []GPUDevice{
			{Index: 0, UUID: "GPU-aaa", NUMANode: -1},
		}
		rows, _ := renderGPURows(gpus, ModeMachine, 0, nil, nil)
		if len(rows) != 1 {
			t.Fatalf("expected 1 row, got %d", len(rows))
		}
		if !strings.Contains(rows[0], "?") {
			t.Errorf("expected '?' suffix for unknown NUMA, got %q", rows[0])
		}
	})

	t.Run("odd number of GPUs", func(t *testing.T) {
		gpus := []GPUDevice{
			{Index: 0, UUID: "GPU-aaa", NUMANode: 0},
			{Index: 1, UUID: "GPU-bbb", NUMANode: 0},
			{Index: 2, UUID: "GPU-ccc", NUMANode: 0},
		}
		rows, _ := renderGPURows(gpus, ModeMachine, 0, nil, nil)
		if len(rows) != 2 {
			t.Fatalf("expected 2 rows for 3 GPUs, got %d", len(rows))
		}
	})
}

func TestJSONTopoOutputContainsMemory(t *testing.T) {
	out := jsonMachineTopology{
		APIVersion: "numacheck/v1",
		Kind:       "MachineTopology",
		Metadata:   jsonMetadata{Timestamp: "2026-05-25T19:30:00Z", NumacheckVersion: "test"},
		Machine: jsonMachine{
			CPU:    jsonCPUSummary{Total: 4, PhysicalCores: 2, Sockets: 1},
			Memory: &jsonMemory{TotalBytes: 8 * 1024 * 1024 * 1024},
			NUMANodes: []jsonNUMANode{
				{ID: 0, SocketID: 0, CPUs: []int{0, 1, 2, 3},
					Memory: &jsonMemory{TotalBytes: 8 * 1024 * 1024 * 1024, FreeBytes: 4 * 1024 * 1024 * 1024}},
			},
		},
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	for _, want := range []string{`"totalBytes"`, `"freeBytes"`, `"machine"`, `"memory"`} {
		if !strings.Contains(s, want) {
			t.Errorf("JSON missing %s: %s", want, s)
		}
	}
}

func TestPrintNodesGridIncludesMemoryFooter(t *testing.T) {
	useColor = false
	t.Cleanup(func() { useColor = false })

	nodes := []NUMANodeInfo{
		{
			ID:            0,
			SocketID:      0,
			CPUs:          []int{0, 1, 2, 3},
			MemTotalBytes: 256 * 1024 * 1024 * 1024,
			MemFreeBytes:  240 * 1024 * 1024 * 1024,
		},
		{
			ID:            1,
			SocketID:      1,
			CPUs:          []int{4, 5, 6, 7},
			MemTotalBytes: 128 * 1024 * 1024 * 1024,
			MemFreeBytes:  100 * 1024 * 1024 * 1024,
		},
	}

	t.Run("machine mode shows total and free", func(t *testing.T) {
		out := captureStdout(t, func() {
			printNodesGrid(nodes, ModeMachine, nil, -1, nil, nil, nil)
		})
		for _, want := range []string{
			"256.0 GiB total, 240.0 GiB free",
			"128.0 GiB total, 100.0 GiB free",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("output missing %q\n--- output ---\n%s", want, out)
			}
		}
	})

	t.Run("process mode shows per-node usage", func(t *testing.T) {
		processMem := map[int]int64{
			0: 4 * 1024 * 1024 * 1024,
			1: 1536 * 1024 * 1024,
		}
		allowed := map[int]bool{0: true, 1: true, 4: true}
		out := captureStdout(t, func() {
			printNodesGrid(nodes, ModeProcess, allowed, 0, map[int]bool{0: true}, nil, processMem)
		})
		for _, want := range []string{
			"4.0 GiB on this node / 256.0 GiB",
			"1.5 GiB on this node / 128.0 GiB",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("output missing %q\n--- output ---\n%s", want, out)
			}
		}
	})
}
