package main

import (
	"reflect"
	"testing"
)

func TestReadIntFile(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		path    string
		want    int
		wantErr bool
	}{
		{
			name:  "valid integer",
			files: map[string]string{"/sys/test": "42"},
			path:  "/sys/test",
			want:  42,
		},
		{
			name:  "integer with trailing whitespace",
			files: map[string]string{"/sys/test": "7\n"},
			path:  "/sys/test",
			want:  7,
		},
		{
			name:  "integer with leading and trailing whitespace",
			files: map[string]string{"/sys/test": " 99 \n"},
			path:  "/sys/test",
			want:  99,
		},
		{
			name:    "non-existent file",
			files:   map[string]string{},
			path:    "/sys/missing",
			wantErr: true,
		},
		{
			name:    "non-integer content",
			files:   map[string]string{"/sys/test": "hello"},
			path:    "/sys/test",
			wantErr: true,
		},
		{
			name:  "zero value",
			files: map[string]string{"/sys/test": "0"},
			path:  "/sys/test",
			want:  0,
		},
		{
			name:  "negative value",
			files: map[string]string{"/sys/test": "-1\n"},
			path:  "/sys/test",
			want:  -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &mockFS{files: tt.files}
			got, err := readIntFile(fs, tt.path)
			if tt.wantErr {
				if err == nil {
					t.Errorf("readIntFile(%q) expected error, got %d", tt.path, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("readIntFile(%q) unexpected error: %v", tt.path, err)
			}
			if got != tt.want {
				t.Errorf("readIntFile(%q) = %d, want %d", tt.path, got, tt.want)
			}
		})
	}
}

func TestBuildNUMAMap(t *testing.T) {
	tests := []struct {
		name    string
		fs      *mockFS
		want    map[int]int
		wantErr bool
	}{
		{
			name: "two nodes with four CPUs",
			fs: &mockFS{
				files: map[string]string{
					"/sys/devices/system/node/node0/cpulist": "0-1",
					"/sys/devices/system/node/node1/cpulist": "2-3",
				},
				globs: map[string][]string{
					"/sys/devices/system/node/node[0-9]*": {
						"/sys/devices/system/node/node0",
						"/sys/devices/system/node/node1",
					},
				},
			},
			want: map[int]int{0: 0, 1: 0, 2: 1, 3: 1},
		},
		{
			name: "empty cpulist on one node skips it",
			fs: &mockFS{
				files: map[string]string{
					"/sys/devices/system/node/node0/cpulist": "0-1",
					"/sys/devices/system/node/node1/cpulist": "",
				},
				globs: map[string][]string{
					"/sys/devices/system/node/node[0-9]*": {
						"/sys/devices/system/node/node0",
						"/sys/devices/system/node/node1",
					},
				},
			},
			want: map[int]int{0: 0, 1: 0},
		},
		{
			name: "no NUMA nodes found",
			fs: &mockFS{
				globs: map[string][]string{
					"/sys/devices/system/node/node[0-9]*": {},
				},
			},
			wantErr: true,
		},
		{
			name: "nodes exist but all cpulists unreadable",
			fs: &mockFS{
				files: map[string]string{},
				globs: map[string][]string{
					"/sys/devices/system/node/node[0-9]*": {
						"/sys/devices/system/node/node0",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "single node with many CPUs",
			fs: &mockFS{
				files: map[string]string{
					"/sys/devices/system/node/node0/cpulist": "0-7",
				},
				globs: map[string][]string{
					"/sys/devices/system/node/node[0-9]*": {
						"/sys/devices/system/node/node0",
					},
				},
			},
			want: map[int]int{0: 0, 1: 0, 2: 0, 3: 0, 4: 0, 5: 0, 6: 0, 7: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildNUMAMap(tt.fs)
			if tt.wantErr {
				if err == nil {
					t.Errorf("buildNUMAMap() expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildNUMAMap() unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildNUMAMap() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetCPUTopology(t *testing.T) {
	tests := []struct {
		name    string
		cpu     int
		files   map[string]string
		want    CoreInfo
		wantErr bool
	}{
		{
			name: "valid topology for CPU 0",
			cpu:  0,
			files: map[string]string{
				"/sys/devices/system/cpu/cpu0/topology/physical_package_id": "0",
				"/sys/devices/system/cpu/cpu0/topology/core_id":            "0",
			},
			want: CoreInfo{PhysicalID: 0, CoreID: 0},
		},
		{
			name: "CPU on second socket",
			cpu:  8,
			files: map[string]string{
				"/sys/devices/system/cpu/cpu8/topology/physical_package_id": "1",
				"/sys/devices/system/cpu/cpu8/topology/core_id":            "4",
			},
			want: CoreInfo{PhysicalID: 1, CoreID: 4},
		},
		{
			name: "missing physical_package_id",
			cpu:  0,
			files: map[string]string{
				"/sys/devices/system/cpu/cpu0/topology/core_id": "0",
			},
			wantErr: true,
		},
		{
			name: "missing core_id",
			cpu:  0,
			files: map[string]string{
				"/sys/devices/system/cpu/cpu0/topology/physical_package_id": "0",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &mockFS{files: tt.files}
			got, err := getCPUTopology(fs, tt.cpu)
			if tt.wantErr {
				if err == nil {
					t.Errorf("getCPUTopology(fs, %d) expected error, got %+v", tt.cpu, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("getCPUTopology(fs, %d) unexpected error: %v", tt.cpu, err)
			}
			if got != tt.want {
				t.Errorf("getCPUTopology(fs, %d) = %+v, want %+v", tt.cpu, got, tt.want)
			}
		})
	}
}

func TestGetSystemCPUCount(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		want    int
		wantErr bool
	}{
		{
			name:  "range 0-3 gives 4 CPUs",
			files: map[string]string{"/sys/devices/system/cpu/possible": "0-3"},
			want:  4,
		},
		{
			name:  "range 0-127 gives 128 CPUs",
			files: map[string]string{"/sys/devices/system/cpu/possible": "0-127\n"},
			want:  128,
		},
		{
			name:  "single CPU",
			files: map[string]string{"/sys/devices/system/cpu/possible": "0"},
			want:  1,
		},
		{
			name:  "disjoint ranges",
			files: map[string]string{"/sys/devices/system/cpu/possible": "0-1,4-5"},
			want:  4,
		},
		{
			name:    "file not found",
			files:   map[string]string{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &mockFS{files: tt.files}
			got, err := getSystemCPUCount(fs)
			if tt.wantErr {
				if err == nil {
					t.Errorf("getSystemCPUCount() expected error, got %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("getSystemCPUCount() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("getSystemCPUCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestGetCurrentCPU(t *testing.T) {
	// Realistic /proc/PID/stat line. Field 39 (processor) is "2".
	// Format: pid (comm) state ppid ... field39 ...
	statLine := "1234 (test) S 1 1234 1234 0 -1 4194560 100 0 0 0 10 5 0 0 20 0 1 0 100 1000000 100 18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 2 0 0 0 0 0 0 0 0 0 0 0 0 0"

	tests := []struct {
		name    string
		pid     int
		files   map[string]string
		want    int
		wantErr bool
	}{
		{
			name:  "parses CPU from field 39",
			pid:   1234,
			files: map[string]string{"/proc/1234/stat": statLine},
			want:  2,
		},
		{
			name:    "file not found",
			pid:     9999,
			files:   map[string]string{},
			wantErr: true,
		},
		{
			name:    "malformed stat no closing paren",
			pid:     1234,
			files:   map[string]string{"/proc/1234/stat": "1234 (test S 1"},
			wantErr: true,
		},
		{
			name:    "too few fields",
			pid:     1234,
			files:   map[string]string{"/proc/1234/stat": "1234 (test) S 1 2 3"},
			wantErr: true,
		},
		{
			name: "comm with spaces and parens",
			pid:  5678,
			files: map[string]string{
				"/proc/5678/stat": "5678 (my (weird) app) S 1 5678 5678 0 -1 4194560 100 0 0 0 10 5 0 0 20 0 1 0 100 1000000 100 18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 5 0 0 0 0 0 0 0 0 0 0 0 0 0",
			},
			want: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &mockFS{files: tt.files}
			got, err := getCurrentCPU(fs, tt.pid)
			if tt.wantErr {
				if err == nil {
					t.Errorf("getCurrentCPU(fs, %d) expected error, got %d", tt.pid, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("getCurrentCPU(fs, %d) unexpected error: %v", tt.pid, err)
			}
			if got != tt.want {
				t.Errorf("getCurrentCPU(fs, %d) = %d, want %d", tt.pid, got, tt.want)
			}
		})
	}
}

func TestGetParentPID(t *testing.T) {
	tests := []struct {
		name    string
		pid     int
		files   map[string]string
		want    int
		wantErr bool
	}{
		{
			name: "parses PPid",
			pid:  1234,
			files: map[string]string{
				"/proc/1234/status": "Name:\ttest\nState:\tS (sleeping)\nPPid:\t567\nUid:\t1000\n",
			},
			want: 567,
		},
		{
			name: "PPid is 1 (init)",
			pid:  42,
			files: map[string]string{
				"/proc/42/status": "Name:\tinit-child\nPPid:\t1\n",
			},
			want: 1,
		},
		{
			name:    "file not found",
			pid:     9999,
			files:   map[string]string{},
			wantErr: true,
		},
		{
			name: "no PPid line",
			pid:  1234,
			files: map[string]string{
				"/proc/1234/status": "Name:\ttest\nState:\tS\n",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &mockFS{files: tt.files}
			got, err := getParentPID(fs, tt.pid)
			if tt.wantErr {
				if err == nil {
					t.Errorf("getParentPID(fs, %d) expected error, got %d", tt.pid, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("getParentPID(fs, %d) unexpected error: %v", tt.pid, err)
			}
			if got != tt.want {
				t.Errorf("getParentPID(fs, %d) = %d, want %d", tt.pid, got, tt.want)
			}
		})
	}
}

func TestGetAllowedGPUs(t *testing.T) {
	gpus := []GPUDevice{
		{Index: 0, UUID: "GPU-aaa", PCIID: "0000:3b:00.0", NUMANode: 0},
		{Index: 1, UUID: "GPU-bbb", PCIID: "0000:86:00.0", NUMANode: 1},
		{Index: 2, UUID: "GPU-ccc", PCIID: "0000:af:00.0", NUMANode: 1},
	}

	tests := []struct {
		name    string
		pid     int
		gpus    []GPUDevice
		files   map[string]string
		want    []string
		wantErr bool
	}{
		{
			name: "UUID env var on the process itself",
			pid:  100,
			gpus: gpus,
			files: map[string]string{
				"/proc/100/environ": "PATH=/usr/bin\x00NVIDIA_VISIBLE_DEVICES=GPU-aaa,GPU-bbb\x00HOME=/root",
			},
			want: []string{"GPU-aaa", "GPU-bbb"},
		},
		{
			name: "numeric index env var resolved to UUIDs",
			pid:  100,
			gpus: gpus,
			files: map[string]string{
				"/proc/100/environ": "NVIDIA_VISIBLE_DEVICES=0,2\x00",
			},
			want: []string{"GPU-aaa", "GPU-ccc"},
		},
		{
			name: "env var set to none",
			pid:  100,
			gpus: gpus,
			files: map[string]string{
				"/proc/100/environ": "NVIDIA_VISIBLE_DEVICES=none\x00",
			},
			want: nil,
		},
		{
			name: "no env var returns nil",
			pid:  100,
			gpus: gpus,
			files: map[string]string{
				"/proc/100/environ": "PATH=/usr/bin\x00HOME=/root\x00",
				"/proc/100/status":  "Name:\ttest\nPPid:\t1\n",
			},
			want: nil,
		},
		{
			name:    "cannot read environ for target pid",
			pid:     100,
			gpus:    gpus,
			files:   map[string]string{},
			wantErr: true,
		},
		{
			name: "walks up to parent to find env var",
			pid:  200,
			gpus: gpus,
			files: map[string]string{
				"/proc/200/environ": "PATH=/usr/bin\x00",
				"/proc/200/status":  "Name:\tchild\nPPid:\t150\n",
				"/proc/150/environ": "NVIDIA_VISIBLE_DEVICES=GPU-bbb\x00",
			},
			want: []string{"GPU-bbb"},
		},
		{
			name: "env var set to void",
			pid:  100,
			gpus: gpus,
			files: map[string]string{
				"/proc/100/environ": "NVIDIA_VISIBLE_DEVICES=void\x00",
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &mockFS{files: tt.files}
			got, err := getAllowedGPUs(fs, tt.pid, tt.gpus)
			if tt.wantErr {
				if err == nil {
					t.Errorf("getAllowedGPUs(fs, %d, gpus) expected error, got %v", tt.pid, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("getAllowedGPUs(fs, %d, gpus) unexpected error: %v", tt.pid, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getAllowedGPUs(fs, %d, gpus) = %v, want %v", tt.pid, got, tt.want)
			}
		})
	}
}

func TestResolveGPUIDs(t *testing.T) {
	gpus := []GPUDevice{
		{Index: 0, UUID: "GPU-aaa", PCIID: "0000:3b:00.0"},
		{Index: 1, UUID: "GPU-bbb", PCIID: "0000:86:00.0"},
		{Index: 2, UUID: "GPU-ccc", PCIID: "0000:af:00.0"},
	}

	tests := []struct {
		name string
		ids  []string
		gpus []GPUDevice
		want []string
	}{
		{
			name: "numeric indices resolved to UUIDs",
			ids:  []string{"0", "2"},
			gpus: gpus,
			want: []string{"GPU-aaa", "GPU-ccc"},
		},
		{
			name: "UUID passthrough",
			ids:  []string{"GPU-aaa", "GPU-bbb"},
			gpus: gpus,
			want: []string{"GPU-aaa", "GPU-bbb"},
		},
		{
			name: "out of range index skipped",
			ids:  []string{"0", "99"},
			gpus: gpus,
			want: []string{"GPU-aaa"},
		},
		{
			name: "empty input",
			ids:  []string{},
			gpus: gpus,
			want: nil,
		},
		{
			name: "single numeric index",
			ids:  []string{"1"},
			gpus: gpus,
			want: []string{"GPU-bbb"},
		},
		{
			name: "numeric with whitespace",
			ids:  []string{" 0 ", " 1 "},
			gpus: gpus,
			want: []string{"GPU-aaa", "GPU-bbb"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveGPUIDs(tt.ids, tt.gpus)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("resolveGPUIDs(%v, gpus) = %v, want %v", tt.ids, got, tt.want)
			}
		})
	}
}

