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
	PID    int                   `json:"pid"`
	Config crictlContainerConfig `json:"config"`
}

type crictlContainerConfig struct {
	Linux crictlLinux `json:"linux"`
}

type crictlLinux struct {
	Resources crictlResources `json:"resources"`
}

type crictlResources struct {
	CPUPeriod          int64 `json:"cpu_period"`
	CPUQuota           int64 `json:"cpu_quota"`
	CPUShares          int64 `json:"cpu_shares"`
	MemoryLimitInBytes int64 `json:"memory_limit_in_bytes"`
}

// ContainerInfo holds PID and resource info from crictl inspect.
type ContainerInfo struct {
	PID       int
	Resources crictlResources
}

// JSON output types.

type jsonTopoOutput struct {
	TotalCPUs     int            `json:"total_cpus"`
	PhysicalCores int            `json:"physical_cores"`
	Sockets       int            `json:"sockets"`
	NUMANodes     []jsonNUMANode `json:"numa_nodes"`
	GPUs          []jsonGPU      `json:"gpus,omitempty"`
}

type jsonProcessOutput struct {
	PID          int              `json:"pid"`
	AllowedCPUs  []int            `json:"allowed_cpus"`
	SystemCPUs   int              `json:"system_cpus,omitempty"`
	Pinned       bool             `json:"pinned"`
	CurrentCPU   int              `json:"current_cpu"`
	CurrentNUMA  int              `json:"current_numa_node"`
	NUMANodes    []jsonNUMANode   `json:"numa_nodes"`
	GPUs         []jsonGPU        `json:"gpus,omitempty"`
	AllowedGPUs  []string         `json:"allowed_gpus,omitempty"`
	Resources    *jsonResources   `json:"container_resources,omitempty"`
	Numastat     string           `json:"numastat,omitempty"`
}

type jsonNUMANode struct {
	ID       int   `json:"id"`
	SocketID int   `json:"socket_id"`
	CPUs     []int `json:"cpus"`
}

type jsonGPU struct {
	Index    int    `json:"index"`
	UUID     string `json:"uuid,omitempty"`
	PCIID    string `json:"pci_id"`
	NUMANode int    `json:"numa_node"`
}

type jsonResources struct {
	CPURequest         *float64 `json:"cpu_request_cores,omitempty"`
	CPULimit           *float64 `json:"cpu_limit_cores,omitempty"`
	MemoryLimitBytes   *int64   `json:"memory_limit_bytes,omitempty"`
	GPUCount           int      `json:"gpu_count,omitempty"`
}
