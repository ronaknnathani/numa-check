package main

import (
	"reflect"
	"testing"
)

func TestReadCPUManagerState(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		path    string
		want    *CPUManagerState
		wantErr bool
	}{
		{
			name: "valid static policy with entries",
			files: map[string]string{
				"/var/lib/kubelet/cpu_manager_state": `{"policyName":"static","defaultCpuSet":"0-3","entries":{"pod-uid-1":{"container-a":"4-7"},"pod-uid-2":{"container-b":"8-11"}},"checksum":12345}`,
			},
			path: "/var/lib/kubelet/cpu_manager_state",
			want: &CPUManagerState{
				PolicyName:    "static",
				DefaultCPUSet: "0-3",
				Entries: map[string]map[string]string{
					"pod-uid-1": {"container-a": "4-7"},
					"pod-uid-2": {"container-b": "8-11"},
				},
			},
		},
		{
			name: "none policy no entries",
			files: map[string]string{
				"/var/lib/kubelet/cpu_manager_state": `{"policyName":"none","defaultCpuSet":"0-15","entries":{}}`,
			},
			path: "/var/lib/kubelet/cpu_manager_state",
			want: &CPUManagerState{
				PolicyName:    "none",
				DefaultCPUSet: "0-15",
				Entries:       map[string]map[string]string{},
			},
		},
		{
			name:    "file not found",
			files:   map[string]string{},
			path:    "/var/lib/kubelet/cpu_manager_state",
			wantErr: true,
		},
		{
			name: "malformed JSON",
			files: map[string]string{
				"/var/lib/kubelet/cpu_manager_state": `{invalid json`,
			},
			path:    "/var/lib/kubelet/cpu_manager_state",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &mockFS{files: tt.files}
			got, err := readCPUManagerState(fs, tt.path)
			if tt.wantErr {
				if err == nil {
					t.Errorf("readCPUManagerState() expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("readCPUManagerState() unexpected error: %v", err)
			}
			if got.PolicyName != tt.want.PolicyName {
				t.Errorf("PolicyName = %q, want %q", got.PolicyName, tt.want.PolicyName)
			}
			if got.DefaultCPUSet != tt.want.DefaultCPUSet {
				t.Errorf("DefaultCPUSet = %q, want %q", got.DefaultCPUSet, tt.want.DefaultCPUSet)
			}
			if !reflect.DeepEqual(got.Entries, tt.want.Entries) {
				t.Errorf("Entries = %v, want %v", got.Entries, tt.want.Entries)
			}
		})
	}
}

func TestParseCPUManagerEntries(t *testing.T) {
	tests := []struct {
		name string
		state *CPUManagerState
		want  []CPUManagerEntry
	}{
		{
			name: "multiple entries sorted",
			state: &CPUManagerState{
				Entries: map[string]map[string]string{
					"pod-uid-b": {"container-x": "8-11"},
					"pod-uid-a": {"container-y": "4-7"},
				},
			},
			want: []CPUManagerEntry{
				{PodUID: "pod-uid-a", ContainerName: "container-y", CPUs: []int{4, 5, 6, 7}, CPUSetRaw: "4-7"},
				{PodUID: "pod-uid-b", ContainerName: "container-x", CPUs: []int{8, 9, 10, 11}, CPUSetRaw: "8-11"},
			},
		},
		{
			name: "same pod multiple containers sorted by name",
			state: &CPUManagerState{
				Entries: map[string]map[string]string{
					"pod-uid-1": {
						"sidecar": "12-13",
						"main":    "4-7",
					},
				},
			},
			want: []CPUManagerEntry{
				{PodUID: "pod-uid-1", ContainerName: "main", CPUs: []int{4, 5, 6, 7}, CPUSetRaw: "4-7"},
				{PodUID: "pod-uid-1", ContainerName: "sidecar", CPUs: []int{12, 13}, CPUSetRaw: "12-13"},
			},
		},
		{
			name: "empty entries",
			state: &CPUManagerState{
				Entries: map[string]map[string]string{},
			},
			want: nil,
		},
		{
			name: "invalid CPU set skipped",
			state: &CPUManagerState{
				Entries: map[string]map[string]string{
					"pod-uid-1": {
						"good": "0-3",
						"bad":  "invalid",
					},
				},
			},
			want: []CPUManagerEntry{
				{PodUID: "pod-uid-1", ContainerName: "good", CPUs: []int{0, 1, 2, 3}, CPUSetRaw: "0-3"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCPUManagerEntries(tt.state)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseCPUManagerEntries() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToJSONCPUManager(t *testing.T) {
	state := &CPUManagerState{
		PolicyName:    "static",
		DefaultCPUSet: "0-3",
	}
	entries := []CPUManagerEntry{
		{PodUID: "pod-uid-1", ContainerName: "main", CPUs: []int{4, 5, 6, 7}},
	}
	nodes := []NUMANodeInfo{
		{ID: 0, CPUs: []int{0, 1, 2, 3, 4, 5, 6, 7}},
		{ID: 1, CPUs: []int{8, 9, 10, 11, 12, 13, 14, 15}},
	}

	got := toJSONCPUManager(state, entries, nodes)
	if got.PolicyName != "static" {
		t.Errorf("PolicyName = %q, want %q", got.PolicyName, "static")
	}
	if !reflect.DeepEqual(got.DefaultCPUs, []int{0, 1, 2, 3}) {
		t.Errorf("DefaultCPUs = %v, want [0 1 2 3]", got.DefaultCPUs)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("len(Entries) = %d, want 1", len(got.Entries))
	}
	if got.Entries[0].PodUID != "pod-uid-1" || got.Entries[0].ContainerName != "main" {
		t.Errorf("Entry = %+v, want pod-uid-1/main", got.Entries[0])
	}

	// Per-NUMA-node: CPUs 4-7 are exclusive on node 0, node 1 has none.
	if len(got.PerNUMANode) != 2 {
		t.Fatalf("len(PerNUMANode) = %d, want 2", len(got.PerNUMANode))
	}
	n0 := got.PerNUMANode[0]
	if n0.NodeID != 0 || n0.ExclusiveCPUs != 4 || n0.RemainingCPUs != 4 || n0.TotalCPUs != 8 {
		t.Errorf("Node 0 = %+v, want {0, 4, 4, 8}", n0)
	}
	n1 := got.PerNUMANode[1]
	if n1.NodeID != 1 || n1.ExclusiveCPUs != 0 || n1.RemainingCPUs != 8 || n1.TotalCPUs != 8 {
		t.Errorf("Node 1 = %+v, want {1, 0, 8, 8}", n1)
	}
}
