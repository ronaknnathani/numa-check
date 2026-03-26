package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

// FileSystem abstracts sysfs/procfs access for testing.
type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	Glob(pattern string) ([]string, error)
}

// CommandRunner abstracts external command execution for testing.
type CommandRunner interface {
	Run(name string, args ...string) ([]byte, error)
}

// osFS is the real filesystem implementation.
type osFS struct{}

func (osFS) ReadFile(path string) ([]byte, error)       { return os.ReadFile(path) }
func (osFS) Glob(pattern string) ([]string, error)       { return filepath.Glob(pattern) }

// execRunner is the real command execution implementation.
type execRunner struct{}

func (execRunner) Run(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// DisplayMode controls how the grid is rendered.
type DisplayMode int

const (
	ModeMachine DisplayMode = iota
	ModeProcess
)

// CoreInfo holds the physical socket and core ID for a CPU.
type CoreInfo struct {
	PhysicalID int
	CoreID     int
}

// GPUDevice represents a discovered GPU with its NUMA affinity.
type GPUDevice struct {
	Index    int
	UUID     string
	PCIID    string
	NUMANode int
}

// NUMANodeInfo groups CPUs and GPUs belonging to a NUMA node.
type NUMANodeInfo struct {
	ID       int
	SocketID int
	CPUs     []int
	GPUs     []GPUDevice
}

// Types for crictl JSON output.

type crictlPSOutput struct {
	Containers []crictlContainer `json:"containers"`
}

type crictlContainer struct {
	ID       string            `json:"id"`
	Metadata crictlMetadata    `json:"metadata"`
	Labels   map[string]string `json:"labels"`
}

type crictlMetadata struct {
	Name string `json:"name"`
}

type crictlInspectOutput struct {
	Info crictlInspectInfo `json:"info"`
}

type crictlInspectInfo struct {
	PID int `json:"pid"`
}
