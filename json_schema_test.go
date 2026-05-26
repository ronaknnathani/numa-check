package main

import (
	"encoding/json"
	"strings"
	"testing"
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
	out := jsonMachineTopology{
		APIVersion: "numa-check/v1",
		Kind:       "MachineTopology",
		Metadata: jsonMetadata{
			Timestamp:        "2026-05-25T19:30:00Z",
			Host:             "node-foo",
			NumaCheckVersion: "v0.5.0",
		},
		Machine: jsonMachine{
			CPU:    jsonCPUSummary{Total: 4, PhysicalCores: 2, Sockets: 1},
			Memory: jsonMemory{TotalBytes: 8 << 30},
			NUMANodes: []jsonNUMANode{
				{ID: 0, SocketID: 0, CPUs: []int{0, 1, 2, 3}, Memory: jsonMemory{TotalBytes: 8 << 30, FreeBytes: 4 << 30}},
			},
		},
	}

	m := toMap(t, out)
	checks := map[string]any{
		"apiVersion":                "numa-check/v1",
		"kind":                      "MachineTopology",
		"metadata.timestamp":        "2026-05-25T19:30:00Z",
		"metadata.host":             "node-foo",
		"metadata.numaCheckVersion": "v0.5.0",
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
	out := jsonProcessReport{
		APIVersion: "numa-check/v1",
		Kind:       "ProcessReport",
		Metadata: jsonMetadata{
			Timestamp:        "2026-05-25T19:30:00Z",
			Host:             "node-foo",
			NumaCheckVersion: "v0.5.0",
			PID:              12345,
			Pod:              "my-pod",
			Container:        "my-container",
		},
		Machine: jsonMachine{
			CPU:    jsonCPUSummary{Total: 4, PhysicalCores: 2, Sockets: 1},
			Memory: jsonMemory{TotalBytes: 8 << 30},
			NUMANodes: []jsonNUMANode{
				{ID: 0, SocketID: 0, CPUs: []int{0, 1, 2, 3}, Memory: jsonMemory{TotalBytes: 8 << 30}},
			},
			GPUs: []jsonGPU{{Index: 0, UUID: "GPU-abc", PCIID: "0000:17:00.0", NUMANode: 0}},
		},
		Process: &jsonProcess{
			CurrentCPU:      2,
			CurrentNUMANode: 0,
			AllowedCPUs:     []int{0, 1, 2, 3},
			AllowedCPUCount: 4,
			SystemCPUCount:  4,
			Pinned:          false,
			MemoryBytes:     2 << 30,
			AllowedGPUs:     []string{"GPU-abc"},
			MemoryPerNUMANode: []jsonProcessMemPerNode{
				{ID: 0, Bytes: 2 << 30},
			},
			ContainerResources: &jsonResources{
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
		"apiVersion":                                  "numa-check/v1",
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

func TestProcessReportOmitsAbsentSections(t *testing.T) {
	out := jsonProcessReport{
		APIVersion: "numa-check/v1",
		Kind:       "ProcessReport",
		Metadata: jsonMetadata{
			Timestamp:        "2026-05-25T19:30:00Z",
			Host:             "node-foo",
			NumaCheckVersion: "v0.5.0",
			PID:              42,
		},
		Machine: jsonMachine{
			CPU:    jsonCPUSummary{Total: 1},
			Memory: jsonMemory{TotalBytes: 1024},
			NUMANodes: []jsonNUMANode{
				{ID: 0, CPUs: []int{0}},
			},
		},
		Process: &jsonProcess{
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
