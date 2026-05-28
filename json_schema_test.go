package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	apiv1 "numacheck/api/v1"
)

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
		Data: apiv1.MachineData{
			Resources: apiv1.MachineResources{
				CPU:    apiv1.CPU{LogicalCores: 4, PhysicalCores: 2, Sockets: 1},
				Memory: &apiv1.Memory{TotalBytes: 8 << 30, FreeBytes: 4 << 30},
				GPUs:   []apiv1.GPU{{Index: 0, UUID: "GPU-abc", PCIID: "0000:17:00.0", NUMANode: 0}},
			},
			NUMANodes: []apiv1.NUMANode{
				{ID: 0, SocketID: 0, CPUs: []int{0, 1, 2, 3}, GPUs: []int{0}, Memory: &apiv1.Memory{TotalBytes: 8 << 30, FreeBytes: 4 << 30}},
			},
		},
	}

	m := toMap(t, out)
	checks := map[string]any{
		"apiVersion":                       "numacheck/v1",
		"kind":                             "MachineTopology",
		"metadata.timestamp":               "2026-05-25T19:30:00Z",
		"metadata.host":                    "node-foo",
		"metadata.numacheckVersion":        "v0.5.0",
		"data.resources.cpu.logicalCores":  float64(4),
		"data.resources.cpu.physicalCores": float64(2),
		"data.resources.cpu.sockets":       float64(1),
		"data.resources.memory.totalBytes": float64(8 << 30),
		"data.resources.memory.freeBytes":  float64(4 << 30),
	}
	for path, want := range checks {
		if got := getPath(m, path); got != want {
			t.Errorf("%s = %v (%T), want %v (%T)", path, got, got, want, want)
		}
	}

	nodes, _ := getPath(m, "data.numaNodes").([]any)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 numaNode, got %d", len(nodes))
	}
	node0 := nodes[0].(map[string]any)
	if got := getPath(node0, "memory.totalBytes"); got != float64(8<<30) {
		t.Errorf("numaNodes[0].memory.totalBytes = %v", got)
	}
	if got := node0["socketId"]; got != float64(0) {
		t.Errorf("numaNodes[0].socketId = %v", got)
	}
	gpus, _ := node0["gpus"].([]any)
	if len(gpus) != 1 || gpus[0] != float64(0) {
		t.Errorf("numaNodes[0].gpus = %v, want [0]", gpus)
	}

	machineGpus, _ := getPath(m, "data.resources.gpus").([]any)
	if len(machineGpus) != 1 {
		t.Fatalf("expected 1 GPU under data.resources.gpus, got %d", len(machineGpus))
	}
	if got := machineGpus[0].(map[string]any)["pciId"]; got != "0000:17:00.0" {
		t.Errorf("data.resources.gpus[0].pciId = %v", got)
	}
}

func TestProcessReportJSONSchema(t *testing.T) {
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
		Data: apiv1.ProcessData{
			Process: apiv1.Process{
				Placement: apiv1.Placement{CurrentCPU: 2, CurrentNUMANode: 0, Pinned: false},
				Affinity: apiv1.Affinity{
					CPUs: []int{0, 1, 2, 3},
					GPUs: []string{"GPU-abc"},
					Memory: &apiv1.ProcessMemory{
						Bytes:       2 << 30,
						PerNUMANode: []apiv1.ProcessMemPerNode{{ID: 0, Bytes: 2 << 30}},
					},
				},
				Container: &apiv1.Container{
					Resources: apiv1.ContainerResources{
						Requests: apiv1.ResourceList{"cpu": "4"},
						Limits: apiv1.ResourceList{
							"cpu":            "8",
							"memory":         "16Gi",
							"nvidia.com/gpu": "1",
						},
					},
				},
			},
		},
	}

	m := toMap(t, out)
	checks := map[string]any{
		"apiVersion":                             "numacheck/v1",
		"kind":                                   "ProcessReport",
		"metadata.pid":                           float64(12345),
		"metadata.pod":                           "my-pod",
		"metadata.container":                     "my-container",
		"data.process.placement.currentCpu":      float64(2),
		"data.process.placement.currentNumaNode": float64(0),
		"data.process.placement.pinned":          false,
		"data.process.affinity.memory.bytes":     float64(2 << 30),
	}
	for path, want := range checks {
		if got := getPath(m, path); got != want {
			t.Errorf("%s = %v, want %v", path, got, want)
		}
	}

	cpus, _ := getPath(m, "data.process.affinity.cpus").([]any)
	if len(cpus) != 4 {
		t.Fatalf("expected 4 cpus, got %v", cpus)
	}
	gpus, _ := getPath(m, "data.process.affinity.gpus").([]any)
	if len(gpus) != 1 || gpus[0] != "GPU-abc" {
		t.Errorf("affinity.gpus = %v, want [GPU-abc]", gpus)
	}

	reqs, _ := getPath(m, "data.process.container.resources.requests").(map[string]any)
	if reqs["cpu"] != "4" {
		t.Errorf("requests.cpu = %v, want \"4\"", reqs["cpu"])
	}
	lims, _ := getPath(m, "data.process.container.resources.limits").(map[string]any)
	if lims["cpu"] != "8" || lims["memory"] != "16Gi" || lims["nvidia.com/gpu"] != "1" {
		t.Errorf("limits = %v, want {cpu:8, memory:16Gi, nvidia.com/gpu:1}", lims)
	}

	memNodes, _ := getPath(m, "data.process.affinity.memory.perNumaNode").([]any)
	if len(memNodes) != 1 {
		t.Fatalf("expected 1 memoryPerNumaNode, got %d", len(memNodes))
	}
	node0 := memNodes[0].(map[string]any)
	if node0["id"] != float64(0) || node0["bytes"] != float64(2<<30) {
		t.Errorf("perNumaNode[0] = %v", node0)
	}
}

func TestBuildProcessMemory(t *testing.T) {
	nodes := []NUMANodeInfo{{ID: 0}, {ID: 1}, {ID: 2}}
	tests := []struct {
		name       string
		processMem map[int]int64
		wantNil    bool
		wantBytes  int64
		wantPer    []apiv1.ProcessMemPerNode
	}{
		{name: "nil returns nil", processMem: nil, wantNil: true},
		{
			name:       "no matching node ids returns nil (zero totals)",
			processMem: map[int]int64{},
			wantNil:    true,
		},
		{
			name:       "populated returns entries in node order",
			processMem: map[int]int64{2: 300, 0: 100, 1: 200},
			wantBytes:  600,
			wantPer: []apiv1.ProcessMemPerNode{
				{ID: 0, Bytes: 100},
				{ID: 1, Bytes: 200},
				{ID: 2, Bytes: 300},
			},
		},
		{
			name:       "non-matching node ids still report bytes total",
			processMem: map[int]int64{7: 100},
			wantBytes:  100,
			wantPer:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildProcessMemory(nodes, tt.processMem)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("got = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("got = nil, want non-nil")
			}
			if got.Bytes != tt.wantBytes {
				t.Errorf("bytes = %d, want %d", got.Bytes, tt.wantBytes)
			}
			if len(got.PerNUMANode) != len(tt.wantPer) {
				t.Fatalf("perNode len = %d, want %d (got=%v)", len(got.PerNUMANode), len(tt.wantPer), got.PerNUMANode)
			}
			for i := range got.PerNUMANode {
				if got.PerNUMANode[i] != tt.wantPer[i] {
					t.Errorf("entry[%d] = %+v, want %+v", i, got.PerNUMANode[i], tt.wantPer[i])
				}
			}
		})
	}
}

func TestBuildContainer(t *testing.T) {
	tests := []struct {
		name     string
		res      crictlResources
		gpuCount int
		wantNil  bool
		wantReqs apiv1.ResourceList
		wantLims apiv1.ResourceList
	}{
		{
			name:    "no observable values returns nil",
			res:     crictlResources{},
			wantNil: true,
		},
		{
			name:     "only GPU count produces nvidia.com/gpu limit",
			gpuCount: 2,
			wantLims: apiv1.ResourceList{"nvidia.com/gpu": "2"},
		},
		{
			name: "full pod spec rendering",
			res: crictlResources{
				CPUShares:          4096, // 4 cores request
				CPUQuota:           800000,
				CPUPeriod:          100000, // 8 cores limit
				MemoryLimitInBytes: 16 << 30,
			},
			gpuCount: 1,
			wantReqs: apiv1.ResourceList{"cpu": "4"},
			wantLims: apiv1.ResourceList{
				"cpu":            "8",
				"memory":         "16Gi",
				"nvidia.com/gpu": "1",
			},
		},
		{
			name:     "fractional cpu request renders as millicores",
			res:      crictlResources{CPUShares: 512}, // 0.5 cores
			wantReqs: apiv1.ResourceList{"cpu": "500m"},
		},
		{
			name:     "no observed GPUs -> no nvidia.com/gpu limit",
			res:      crictlResources{CPUShares: 1024},
			gpuCount: 0,
			wantReqs: apiv1.ResourceList{"cpu": "1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildContainer(tt.res, tt.gpuCount)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("got = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("got = nil, want non-nil")
			}
			if !mapsEqual(got.Resources.Requests, tt.wantReqs) {
				t.Errorf("requests = %v, want %v", got.Resources.Requests, tt.wantReqs)
			}
			if !mapsEqual(got.Resources.Limits, tt.wantLims) {
				t.Errorf("limits = %v, want %v", got.Resources.Limits, tt.wantLims)
			}
		})
	}
}

func mapsEqual(a, b apiv1.ResourceList) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// TestAffinityCPUsEmptyMarshalsAsArray pins the contract that an empty
// affinity CPU slice marshals to `[]`, not `null`.
func TestAffinityCPUsEmptyMarshalsAsArray(t *testing.T) {
	p := apiv1.Process{Affinity: apiv1.Affinity{CPUs: []int{}}}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"cpus":[]`) {
		t.Errorf("expected cpus to marshal as []; got: %s", string(data))
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
		Data: apiv1.ProcessData{
			Process: apiv1.Process{
				Placement: apiv1.Placement{CurrentCPU: 0, CurrentNUMANode: 0, Pinned: false},
				Affinity:  apiv1.Affinity{CPUs: []int{0}},
			},
		},
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)

	mustOmit := []string{
		`"pod"`, `"container"`,
		`"gpus"`, `"memory"`,
		`"machine"`,
	}
	for _, k := range mustOmit {
		if strings.Contains(s, k) {
			t.Errorf("expected JSON to omit %s, got: %s", k, s)
		}
	}
}

func TestRunTopoOnlyJSONWiring(t *testing.T) {
	fs := &mockFS{
		files: map[string]string{
			"/sys/devices/system/node/node0/cpulist":                    "0-1",
			"/sys/devices/system/node/node1/cpulist":                    "2-3",
			"/sys/devices/system/node/node0/meminfo":                    "Node 0 MemTotal:     65536 kB\nNode 0 MemFree:      32768 kB\n",
			"/sys/devices/system/node/node1/meminfo":                    "Node 1 MemTotal:     65536 kB\nNode 1 MemFree:      16384 kB\n",
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
			"/sys/bus/pci/devices/*": {},
		},
	}
	cmd := &mockCmd{}

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
		"apiVersion":                       "numacheck/v1",
		"kind":                             "MachineTopology",
		"data.resources.cpu.logicalCores":  float64(4),
		"data.resources.cpu.physicalCores": float64(4),
		"data.resources.cpu.sockets":       float64(2),
		"data.resources.memory.totalBytes": float64(2 * 65536 * 1024),
		"data.resources.memory.freeBytes":  float64((32768 + 16384) * 1024),
	}
	for path, want := range checks {
		if got := getPath(m, path); got != want {
			t.Errorf("%s = %v (%T), want %v (%T)", path, got, got, want, want)
		}
	}

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

	nodes, _ := getPath(m, "data.numaNodes").([]any)
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
	if _, present := node0["gpus"]; present {
		t.Errorf("numaNodes[0].gpus should be omitted when no GPUs; got present in %s", out)
	}
}

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
	if md.PID != 0 || md.Pod != "" || md.Container != "" {
		t.Errorf("expected PID/Pod/Container zero, got pid=%d pod=%q container=%q",
			md.PID, md.Pod, md.Container)
	}
}
