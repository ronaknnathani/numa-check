package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	apiv1 "numacheck/api/v1"
)

// helper: marshal v and unmarshal into a generic map for structural assertions.
func toMap(t *testing.T, v any) map[string]any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

// getPath looks up a dotted path through nested maps. Returns nil if any segment is missing.
func getPath(m map[string]any, path string) any {
	parts := strings.Split(path, ".")
	var cur any = m
	for _, p := range parts {
		mp, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mp[p]
	}
	return cur
}

func TestMachineTopologyJSONSchema(t *testing.T) {
	out := apiv1.MachineTopology{
		APIVersion: "numacheck/v1",
		Kind:       "MachineTopology",
		Metadata: apiv1.Metadata{
			Timestamp:        "2026-05-25T19:30:00Z",
			Host:             "node-foo",
			NumacheckVersion: "v0.5.0",
		},
		Machine: apiv1.Machine{
			CPU:    apiv1.CPUSummary{Total: 4, PhysicalCores: 2, Sockets: 1},
			Memory: &apiv1.Memory{TotalBytes: 8 << 30},
			NUMANodes: []apiv1.NUMANode{
				{ID: 0, SocketID: 0, CPUs: []int{0, 1, 2, 3}, Memory: &apiv1.Memory{TotalBytes: 8 << 30, FreeBytes: 4 << 30}},
			},
		},
	}

	m := toMap(t, out)
	checks := map[string]any{
		"apiVersion":                "numacheck/v1",
		"kind":                      "MachineTopology",
		"metadata.timestamp":        "2026-05-25T19:30:00Z",
		"metadata.host":             "node-foo",
		"metadata.numacheckVersion": "v0.5.0",
		"machine.cpu.total":         float64(4),
		"machine.cpu.physicalCores": float64(2),
		"machine.cpu.sockets":       float64(1),
		"machine.memory.totalBytes": float64(8 << 30),
	}
	for path, want := range checks {
		if got := getPath(m, path); got != want {
			t.Errorf("%s = %v (%T), want %v (%T)", path, got, got, want, want)
		}
	}
	// numaNodes[0].memory.totalBytes
	nodes, _ := getPath(m, "machine.numaNodes").([]any)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 numaNode, got %d", len(nodes))
	}
	node0 := nodes[0].(map[string]any)
	if got := getPath(node0, "memory.totalBytes"); got != float64(8<<30) {
		t.Errorf("numaNodes[0].memory.totalBytes = %v", got)
	}
	if got := getPath(node0, "socketId"); got != float64(0) {
		t.Errorf("numaNodes[0].socketId = %v", got)
	}
}

func TestProcessReportJSONSchema(t *testing.T) {
	limit := int64(16 << 30)
	cpuReq, cpuLim := 4.0, 8.0
	out := apiv1.ProcessReport{
		APIVersion: "numacheck/v1",
		Kind:       "ProcessReport",
		Metadata: apiv1.Metadata{
			Timestamp:        "2026-05-25T19:30:00Z",
			Host:             "node-foo",
			NumacheckVersion: "v0.5.0",
			PID:              12345,
			Pod:              "my-pod",
			Container:        "my-container",
		},
		Machine: apiv1.Machine{
			CPU:    apiv1.CPUSummary{Total: 4, PhysicalCores: 2, Sockets: 1},
			Memory: &apiv1.Memory{TotalBytes: 8 << 30},
			NUMANodes: []apiv1.NUMANode{
				{ID: 0, SocketID: 0, CPUs: []int{0, 1, 2, 3}, Memory: &apiv1.Memory{TotalBytes: 8 << 30}},
			},
			GPUs: []apiv1.GPU{{Index: 0, UUID: "GPU-abc", PCIID: "0000:17:00.0", NUMANode: 0}},
		},
		Process: &apiv1.Process{
			CurrentCPU:      2,
			CurrentNUMANode: 0,
			AllowedCPUs:     []int{0, 1, 2, 3},
			AllowedCPUCount: 4,
			SystemCPUCount:  4,
			Pinned:          false,
			MemoryBytes:     2 << 30,
			AllowedGPUs:     []string{"GPU-abc"},
			MemoryPerNUMANode: []apiv1.ProcessMemPerNode{
				{ID: 0, Bytes: 2 << 30},
			},
			ContainerResources: &apiv1.Resources{
				CPURequestCores:  &cpuReq,
				CPULimitCores:    &cpuLim,
				MemoryLimitBytes: &limit,
				GPUCount:         1,
			},
			Numastat: "raw numastat output",
		},
	}

	m := toMap(t, out)
	checks := map[string]any{
		"apiVersion":                                  "numacheck/v1",
		"kind":                                        "ProcessReport",
		"metadata.pid":                                float64(12345),
		"metadata.pod":                                "my-pod",
		"metadata.container":                          "my-container",
		"process.currentCpu":                          float64(2),
		"process.currentNumaNode":                     float64(0),
		"process.allowedCpuCount":                     float64(4),
		"process.systemCpuCount":                      float64(4),
		"process.pinned":                              false,
		"process.memoryBytes":                         float64(2 << 30),
		"process.numastat":                            "raw numastat output",
		"process.containerResources.cpuRequestCores":  float64(4),
		"process.containerResources.cpuLimitCores":    float64(8),
		"process.containerResources.memoryLimitBytes": float64(16 << 30),
		"process.containerResources.gpuCount":         float64(1),
	}
	for path, want := range checks {
		if got := getPath(m, path); got != want {
			t.Errorf("%s = %v, want %v", path, got, want)
		}
	}

	gpus, ok := getPath(m, "machine.gpus").([]any)
	if !ok || len(gpus) != 1 {
		t.Fatalf("expected 1 gpu, got %v", gpus)
	}
	gpu0 := gpus[0].(map[string]any)
	if got := gpu0["pciId"]; got != "0000:17:00.0" {
		t.Errorf("machine.gpus[0].pciId = %v", got)
	}
	if got := gpu0["numaNode"]; got != float64(0) {
		t.Errorf("machine.gpus[0].numaNode = %v", got)
	}

	memNodes, _ := getPath(m, "process.memoryPerNumaNode").([]any)
	if len(memNodes) != 1 {
		t.Fatalf("expected 1 memoryPerNumaNode, got %d", len(memNodes))
	}
	node0 := memNodes[0].(map[string]any)
	if node0["id"] != float64(0) || node0["bytes"] != float64(2<<30) {
		t.Errorf("memoryPerNumaNode[0] = %v", node0)
	}
}

func TestToAPIProcessMemPerNode(t *testing.T) {
	nodes := []NUMANodeInfo{
		{ID: 0},
		{ID: 1},
		{ID: 2},
	}
	tests := []struct {
		name       string
		processMem map[int]int64
		want       []apiv1.ProcessMemPerNode
	}{
		{
			name:       "nil processMem returns nil",
			processMem: nil,
			want:       nil,
		},
		{
			name:       "no matching node ids returns nil",
			processMem: map[int]int64{7: 100, 9: 200},
			want:       nil,
		},
		{
			name:       "populated returns entries in node order",
			processMem: map[int]int64{2: 300, 0: 100, 1: 200},
			want: []apiv1.ProcessMemPerNode{
				{ID: 0, Bytes: 100},
				{ID: 1, Bytes: 200},
				{ID: 2, Bytes: 300},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toAPIProcessMemPerNode(nodes, tt.processMem)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d (got=%v)", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("entry[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestProcessAllowedCPUsEmptyMarshalsAsArray pins the contract that an
// empty AllowedCPUs slice marshals to `[]`, not `null`.
func TestProcessAllowedCPUsEmptyMarshalsAsArray(t *testing.T) {
	p := apiv1.Process{AllowedCPUs: []int{}}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"allowedCpus":[]`) {
		t.Errorf("expected allowedCpus to marshal as []; got: %s", string(data))
	}
}

func TestProcessReportOmitsAbsentSections(t *testing.T) {
	out := apiv1.ProcessReport{
		APIVersion: "numacheck/v1",
		Kind:       "ProcessReport",
		Metadata: apiv1.Metadata{
			Timestamp:        "2026-05-25T19:30:00Z",
			Host:             "node-foo",
			NumacheckVersion: "v0.5.0",
			PID:              42,
		},
		Machine: apiv1.Machine{
			CPU:    apiv1.CPUSummary{Total: 1},
			Memory: &apiv1.Memory{TotalBytes: 1024},
			NUMANodes: []apiv1.NUMANode{
				{ID: 0, CPUs: []int{0}},
			},
		},
		Process: &apiv1.Process{
			CurrentCPU:      0,
			CurrentNUMANode: 0,
			AllowedCPUs:     []int{0},
			AllowedCPUCount: 1,
			SystemCPUCount:  1,
		},
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)

	mustOmit := []string{
		`"pod"`, `"container"`,
		`"gpus"`, `"cpuManager"`,
		`"containerResources"`, `"numastat"`,
		`"memoryPerNumaNode"`, `"allowedGpus"`,
		`"memoryBytes"`,
	}
	for _, k := range mustOmit {
		if strings.Contains(s, k) {
			t.Errorf("expected JSON to omit %s, got: %s", k, s)
		}
	}
}

// TestRunTopoOnlyJSONWiring exercises the JSON construction path end-to-end
// from a stubbed FileSystem so a field/source rename in main.go cannot slip
// through (the other tests in this file build structs by hand).
func TestRunTopoOnlyJSONWiring(t *testing.T) {
	fs := &mockFS{
		files: map[string]string{
			"/sys/devices/system/node/node0/cpulist":                   "0-1",
			"/sys/devices/system/node/node1/cpulist":                   "2-3",
			"/sys/devices/system/node/node0/meminfo":                   "Node 0 MemTotal:     65536 kB\nNode 0 MemFree:      32768 kB\n",
			"/sys/devices/system/node/node1/meminfo":                   "Node 1 MemTotal:     65536 kB\nNode 1 MemFree:      16384 kB\n",
			"/sys/devices/system/cpu/cpu0/topology/physical_package_id": "0",
			"/sys/devices/system/cpu/cpu0/topology/core_id":             "0",
			"/sys/devices/system/cpu/cpu1/topology/physical_package_id": "0",
			"/sys/devices/system/cpu/cpu1/topology/core_id":             "1",
			"/sys/devices/system/cpu/cpu2/topology/physical_package_id": "1",
			"/sys/devices/system/cpu/cpu2/topology/core_id":             "0",
			"/sys/devices/system/cpu/cpu3/topology/physical_package_id": "1",
			"/sys/devices/system/cpu/cpu3/topology/core_id":             "1",
		},
		globs: map[string][]string{
			"/sys/devices/system/node/node[0-9]*": {
				"/sys/devices/system/node/node0",
				"/sys/devices/system/node/node1",
			},
			// No PCI devices -> discoverGPUs returns nil, no error.
			"/sys/bus/pci/devices/*": {},
		},
	}
	cmd := &mockCmd{} // unused: no nvidia-smi call when PCI glob is empty.

	out := captureStdout(t, func() {
		if err := runTopoOnly(fs, cmd, true, ""); err != nil {
			t.Fatalf("runTopoOnly: %v", err)
		}
	})

	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("decode captured JSON: %v\noutput: %s", err, out)
	}

	checks := map[string]any{
		"apiVersion":                "numacheck/v1",
		"kind":                      "MachineTopology",
		"machine.cpu.total":         float64(4),
		"machine.cpu.physicalCores": float64(4), // 2 cores per socket × 2 sockets
		"machine.cpu.sockets":       float64(2),
		"machine.memory.totalBytes": float64(2 * 65536 * 1024),
	}
	for path, want := range checks {
		if got := getPath(m, path); got != want {
			t.Errorf("%s = %v (%T), want %v (%T)", path, got, got, want, want)
		}
	}

	// metadata.host should be present (possibly empty if os.Hostname fails).
	if _, ok := getPath(m, "metadata").(map[string]any)["host"]; !ok {
		t.Errorf("metadata.host key missing: %s", out)
	}
	ts, _ := getPath(m, "metadata.timestamp").(string)
	if _, err := time.Parse(time.RFC3339, ts); err != nil {
		t.Errorf("metadata.timestamp %q is not RFC3339: %v", ts, err)
	}
	if got := getPath(m, "metadata.numacheckVersion"); got != version {
		t.Errorf("metadata.numacheckVersion = %v, want %v", got, version)
	}

	// numaNodes should be ordered by ID with the expected CPU sets.
	nodes, _ := getPath(m, "machine.numaNodes").([]any)
	if len(nodes) != 2 {
		t.Fatalf("expected 2 numaNodes, got %d (out=%s)", len(nodes), out)
	}
	node0 := nodes[0].(map[string]any)
	if got := node0["id"]; got != float64(0) {
		t.Errorf("numaNodes[0].id = %v", got)
	}
	if got := node0["socketId"]; got != float64(0) {
		t.Errorf("numaNodes[0].socketId = %v", got)
	}
	cpus0, _ := node0["cpus"].([]any)
	if len(cpus0) != 2 || cpus0[0] != float64(0) || cpus0[1] != float64(1) {
		t.Errorf("numaNodes[0].cpus = %v, want [0 1]", cpus0)
	}
	if got := getPath(node0, "memory.totalBytes"); got != float64(65536*1024) {
		t.Errorf("numaNodes[0].memory.totalBytes = %v", got)
	}
	if got := getPath(node0, "memory.freeBytes"); got != float64(32768*1024) {
		t.Errorf("numaNodes[0].memory.freeBytes = %v", got)
	}
}

// TestBuildMetadata pins the basic shape of jsonMetadata produced by
// buildMetadata: RFC3339 timestamp, version wired through, hostname present.
// Swaps the hostnameFn/nowFn seams so timestamp and host are deterministic.
func TestBuildMetadata(t *testing.T) {
	origHostname := hostnameFn
	origNow := nowFn
	t.Cleanup(func() {
		hostnameFn = origHostname
		nowFn = origNow
	})
	hostnameFn = func() (string, error) { return "stub-host", nil }
	fixed := time.Date(2026, 5, 25, 19, 30, 0, 0, time.UTC)
	nowFn = func() time.Time { return fixed }

	md := buildMetadata()

	if md.Timestamp != "2026-05-25T19:30:00Z" {
		t.Errorf("Timestamp = %q, want 2026-05-25T19:30:00Z", md.Timestamp)
	}
	if _, err := time.Parse(time.RFC3339, md.Timestamp); err != nil {
		t.Errorf("Timestamp %q is not RFC3339: %v", md.Timestamp, err)
	}
	if md.Host != "stub-host" {
		t.Errorf("Host = %q, want stub-host", md.Host)
	}
	if md.NumacheckVersion != version {
		t.Errorf("NumacheckVersion = %q, want %q", md.NumacheckVersion, version)
	}
	// PID/Pod/Container are zero values here since buildMetadata only sets the
	// three fields above.
	if md.PID != 0 || md.Pod != "" || md.Container != "" {
		t.Errorf("expected PID/Pod/Container zero, got pid=%d pod=%q container=%q",
			md.PID, md.Pod, md.Container)
	}
}
