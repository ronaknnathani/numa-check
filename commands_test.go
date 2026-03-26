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

func TestGetPIDFromContainer(t *testing.T) {
	t.Run("successful lookup", func(t *testing.T) {
		psJSON := `{"containers":[{"id":"abc123","metadata":{"name":"mycontainer"},"labels":{"io.kubernetes.pod.name":"mypod"}}]}`
		inspectJSON := `{"info":{"pid":4567}}`

		cmd := &mockCmd{calls: []mockCall{
			{output: []byte(psJSON)},
			{output: []byte(inspectJSON)},
		}}

		pid, err := getPIDFromContainer(cmd, "mypod", "mycontainer")
		if err != nil {
			t.Fatalf("getPIDFromContainer() unexpected error: %v", err)
		}
		if pid != 4567 {
			t.Errorf("getPIDFromContainer() = %d, want 4567", pid)
		}
	})

	t.Run("container not found", func(t *testing.T) {
		psJSON := `{"containers":[{"id":"abc123","metadata":{"name":"other"},"labels":{"io.kubernetes.pod.name":"otherpod"}}]}`

		cmd := &mockCmd{calls: []mockCall{
			{output: []byte(psJSON)},
		}}

		_, err := getPIDFromContainer(cmd, "mypod", "mycontainer")
		if err == nil {
			t.Error("getPIDFromContainer() expected error for missing container")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("getPIDFromContainer() error = %q, want it to contain 'not found'", err.Error())
		}
	})

	t.Run("matching container name but wrong pod", func(t *testing.T) {
		psJSON := `{"containers":[{"id":"abc123","metadata":{"name":"mycontainer"},"labels":{"io.kubernetes.pod.name":"wrongpod"}}]}`

		cmd := &mockCmd{calls: []mockCall{
			{output: []byte(psJSON)},
		}}

		_, err := getPIDFromContainer(cmd, "mypod", "mycontainer")
		if err == nil {
			t.Error("getPIDFromContainer() expected error when pod name doesn't match")
		}
	})

	t.Run("invalid PID zero", func(t *testing.T) {
		psJSON := `{"containers":[{"id":"abc123","metadata":{"name":"mycontainer"},"labels":{"io.kubernetes.pod.name":"mypod"}}]}`
		inspectJSON := `{"info":{"pid":0}}`

		cmd := &mockCmd{calls: []mockCall{
			{output: []byte(psJSON)},
			{output: []byte(inspectJSON)},
		}}

		_, err := getPIDFromContainer(cmd, "mypod", "mycontainer")
		if err == nil {
			t.Error("getPIDFromContainer() expected error for PID 0")
		}
		if !strings.Contains(err.Error(), "invalid PID") {
			t.Errorf("getPIDFromContainer() error = %q, want it to contain 'invalid PID'", err.Error())
		}
	})

	t.Run("invalid PID negative", func(t *testing.T) {
		psJSON := `{"containers":[{"id":"abc123","metadata":{"name":"mycontainer"},"labels":{"io.kubernetes.pod.name":"mypod"}}]}`
		inspectJSON := `{"info":{"pid":-1}}`

		cmd := &mockCmd{calls: []mockCall{
			{output: []byte(psJSON)},
			{output: []byte(inspectJSON)},
		}}

		_, err := getPIDFromContainer(cmd, "mypod", "mycontainer")
		if err == nil {
			t.Error("getPIDFromContainer() expected error for negative PID")
		}
	})

	t.Run("crictl ps fails", func(t *testing.T) {
		cmd := &mockCmd{calls: []mockCall{
			{err: fmt.Errorf("crictl not found")},
		}}

		_, err := getPIDFromContainer(cmd, "mypod", "mycontainer")
		if err == nil {
			t.Error("getPIDFromContainer() expected error when crictl ps fails")
		}
	})

	t.Run("crictl inspect fails", func(t *testing.T) {
		psJSON := `{"containers":[{"id":"abc123","metadata":{"name":"mycontainer"},"labels":{"io.kubernetes.pod.name":"mypod"}}]}`

		cmd := &mockCmd{calls: []mockCall{
			{output: []byte(psJSON)},
			{err: fmt.Errorf("inspect failed")},
		}}

		_, err := getPIDFromContainer(cmd, "mypod", "mycontainer")
		if err == nil {
			t.Error("getPIDFromContainer() expected error when crictl inspect fails")
		}
	})
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
