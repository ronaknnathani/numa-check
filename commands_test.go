package main

import (
	"fmt"
	"strings"
	"testing"
)

// mockCmd implements CommandRunner for testing.
// calls is a list of expected (name+args -> output) pairs, consumed in order.
type mockCmd struct {
	calls []mockCall
	idx   int
}

type mockCall struct {
	output []byte
	err    error
}

func (m *mockCmd) Run(name string, args ...string) ([]byte, error) {
	if m.idx >= len(m.calls) {
		return nil, fmt.Errorf("unexpected call #%d: %s %v", m.idx, name, args)
	}
	call := m.calls[m.idx]
	m.idx++
	return call.output, call.err
}

func TestGetGPUInfo(t *testing.T) {
	t.Run("valid nvidia-smi output", func(t *testing.T) {
		cmd := &mockCmd{calls: []mockCall{
			{output: []byte("GPU-uuid-1, 00000000:3B:00.0\nGPU-uuid-2, 0000:86:00.0\n")},
		}}

		got, err := getGPUInfo(cmd)
		if err != nil {
			t.Fatalf("getGPUInfo() unexpected error: %v", err)
		}

		// 00000000:3B:00.0 should be normalized to 0000:3b:00.0
		if pci, ok := got["GPU-uuid-1"]; !ok || pci != "0000:3b:00.0" {
			t.Errorf("GPU-uuid-1: got %q, want %q", pci, "0000:3b:00.0")
		}
		// 0000:86:00.0 should be normalized to 0000:86:00.0 (lowercase)
		if pci, ok := got["GPU-uuid-2"]; !ok || pci != "0000:86:00.0" {
			t.Errorf("GPU-uuid-2: got %q, want %q", pci, "0000:86:00.0")
		}
		if len(got) != 2 {
			t.Errorf("getGPUInfo() returned %d entries, want 2", len(got))
		}
	})

	t.Run("nvidia-smi error", func(t *testing.T) {
		cmd := &mockCmd{calls: []mockCall{
			{err: fmt.Errorf("exec: nvidia-smi: executable file not found in $PATH")},
		}}

		got, err := getGPUInfo(cmd)
		if err == nil {
			t.Errorf("getGPUInfo() expected error, got %v", got)
		}
	})

	t.Run("empty output", func(t *testing.T) {
		cmd := &mockCmd{calls: []mockCall{
			{output: []byte("")},
		}}

		got, err := getGPUInfo(cmd)
		if err != nil {
			t.Fatalf("getGPUInfo() unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("getGPUInfo() returned %d entries, want 0", len(got))
		}
	})
}

func TestGetContainerInfo(t *testing.T) {
	t.Run("successful lookup with resources", func(t *testing.T) {
		psJSON := `{"containers":[{"id":"abc123","metadata":{"name":"mycontainer"},"labels":{"io.kubernetes.pod.name":"mypod"}}]}`
		inspectJSON := `{"info":{"pid":4567,"config":{"linux":{"resources":{"cpu_period":100000,"cpu_quota":400000,"cpu_shares":4096,"memory_limit_in_bytes":8589934592}}}}}`

		cmd := &mockCmd{calls: []mockCall{
			{output: []byte(psJSON)},
			{output: []byte(inspectJSON)},
		}}

		info, err := getContainerInfo(cmd, "mypod", "mycontainer")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.PID != 4567 {
			t.Errorf("PID = %d, want 4567", info.PID)
		}
		if info.Resources.CPUQuota != 400000 {
			t.Errorf("CPUQuota = %d, want 400000", info.Resources.CPUQuota)
		}
		if info.Resources.CPUPeriod != 100000 {
			t.Errorf("CPUPeriod = %d, want 100000", info.Resources.CPUPeriod)
		}
		if info.Resources.CPUShares != 4096 {
			t.Errorf("CPUShares = %d, want 4096", info.Resources.CPUShares)
		}
		if info.Resources.MemoryLimitInBytes != 8589934592 {
			t.Errorf("MemoryLimitInBytes = %d, want 8589934592", info.Resources.MemoryLimitInBytes)
		}
	})

	t.Run("minimal inspect JSON (PID only)", func(t *testing.T) {
		psJSON := `{"containers":[{"id":"abc123","metadata":{"name":"mycontainer"},"labels":{"io.kubernetes.pod.name":"mypod"}}]}`
		inspectJSON := `{"info":{"pid":4567}}`

		cmd := &mockCmd{calls: []mockCall{
			{output: []byte(psJSON)},
			{output: []byte(inspectJSON)},
		}}

		info, err := getContainerInfo(cmd, "mypod", "mycontainer")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.PID != 4567 {
			t.Errorf("PID = %d, want 4567", info.PID)
		}
		if info.Resources.CPUQuota != 0 {
			t.Errorf("CPUQuota = %d, want 0 (missing)", info.Resources.CPUQuota)
		}
	})

	t.Run("container not found", func(t *testing.T) {
		psJSON := `{"containers":[{"id":"abc123","metadata":{"name":"other"},"labels":{"io.kubernetes.pod.name":"otherpod"}}]}`

		cmd := &mockCmd{calls: []mockCall{
			{output: []byte(psJSON)},
		}}

		_, err := getContainerInfo(cmd, "mypod", "mycontainer")
		if err == nil {
			t.Error("expected error for missing container")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("error = %q, want it to contain 'not found'", err.Error())
		}
	})

	t.Run("matching container name but wrong pod", func(t *testing.T) {
		psJSON := `{"containers":[{"id":"abc123","metadata":{"name":"mycontainer"},"labels":{"io.kubernetes.pod.name":"wrongpod"}}]}`

		cmd := &mockCmd{calls: []mockCall{
			{output: []byte(psJSON)},
		}}

		_, err := getContainerInfo(cmd, "mypod", "mycontainer")
		if err == nil {
			t.Error("expected error when pod name doesn't match")
		}
	})

	t.Run("invalid PID zero", func(t *testing.T) {
		psJSON := `{"containers":[{"id":"abc123","metadata":{"name":"mycontainer"},"labels":{"io.kubernetes.pod.name":"mypod"}}]}`
		inspectJSON := `{"info":{"pid":0}}`

		cmd := &mockCmd{calls: []mockCall{
			{output: []byte(psJSON)},
			{output: []byte(inspectJSON)},
		}}

		_, err := getContainerInfo(cmd, "mypod", "mycontainer")
		if err == nil {
			t.Error("expected error for PID 0")
		}
	})

	t.Run("crictl ps fails", func(t *testing.T) {
		cmd := &mockCmd{calls: []mockCall{
			{err: fmt.Errorf("crictl not found")},
		}}

		_, err := getContainerInfo(cmd, "mypod", "mycontainer")
		if err == nil {
			t.Error("expected error when crictl ps fails")
		}
	})

	t.Run("crictl inspect fails", func(t *testing.T) {
		psJSON := `{"containers":[{"id":"abc123","metadata":{"name":"mycontainer"},"labels":{"io.kubernetes.pod.name":"mypod"}}]}`

		cmd := &mockCmd{calls: []mockCall{
			{output: []byte(psJSON)},
			{err: fmt.Errorf("inspect failed")},
		}}

		_, err := getContainerInfo(cmd, "mypod", "mycontainer")
		if err == nil {
			t.Error("expected error when crictl inspect fails")
		}
	})
}

func TestFormatResources(t *testing.T) {
	t.Run("full resources", func(t *testing.T) {
		res := crictlResources{
			CPUPeriod:          100000,
			CPUQuota:           400000,
			CPUShares:          4096,
			MemoryLimitInBytes: 8589934592,
		}
		got := formatResources(res, 2)
		if !strings.Contains(got, "4.0 cores") {
			t.Errorf("expected CPU request '4.0 cores', got:\n%s", got)
		}
		if !strings.Contains(got, "4.0 cores") {
			t.Errorf("expected CPU limit '4.0 cores', got:\n%s", got)
		}
		if !strings.Contains(got, "8.0 GiB") {
			t.Errorf("expected memory '8.0 GiB', got:\n%s", got)
		}
		if !strings.Contains(got, "2") {
			t.Errorf("expected GPU count '2', got:\n%s", got)
		}
	})

	t.Run("no resources (all zero)", func(t *testing.T) {
		got := formatResources(crictlResources{}, 0)
		if got != "" {
			t.Errorf("expected empty string for zero resources, got:\n%s", got)
		}
	})

	t.Run("cpu only no memory", func(t *testing.T) {
		res := crictlResources{CPUPeriod: 100000, CPUQuota: 200000}
		got := formatResources(res, 0)
		if !strings.Contains(got, "2.0 cores") {
			t.Errorf("expected CPU limit '2.0 cores', got:\n%s", got)
		}
		if strings.Contains(got, "Memory") {
			t.Errorf("should not contain Memory line, got:\n%s", got)
		}
	})

	t.Run("default cpu shares (2) omitted", func(t *testing.T) {
		res := crictlResources{CPUShares: 2}
		got := formatResources(res, 0)
		if strings.Contains(got, "request") {
			t.Errorf("default CPU shares should be omitted, got:\n%s", got)
		}
	})
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{8589934592, "8.0 GiB"},
		{1073741824, "1.0 GiB"},
		{536870912, "512.0 MiB"},
		{1048576, "1.0 MiB"},
		{1024, "1024 B"},
	}
	for _, tt := range tests {
		got := formatBytes(tt.input)
		if got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRunNumastat(t *testing.T) {
	t.Run("output is indented with 2 spaces", func(t *testing.T) {
		numastatOutput := "Per-node process memory usage (in MBs) for PID 1234\n         Node 0   Node 1    Total\n         ------   ------    -----\nHuge      0.00     0.00      0.00\nHeap     10.00     0.00     10.00\n"

		cmd := &mockCmd{calls: []mockCall{
			{output: []byte(numastatOutput)},
		}}

		got, err := runNumastat(cmd, 1234)
		if err != nil {
			t.Fatalf("runNumastat() unexpected error: %v", err)
		}

		lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
		for i, line := range lines {
			if !strings.HasPrefix(line, "  ") {
				t.Errorf("line %d not indented with 2 spaces: %q", i, line)
			}
		}
	})

	t.Run("numastat error", func(t *testing.T) {
		cmd := &mockCmd{calls: []mockCall{
			{err: fmt.Errorf("numastat not found")},
		}}

		_, err := runNumastat(cmd, 1234)
		if err == nil {
			t.Error("runNumastat() expected error")
		}
	})
}
