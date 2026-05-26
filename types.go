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
	ID            int
	SocketID      int
	CPUs          []int
	GPUs          []GPUDevice
	MemTotalBytes int64
	MemFreeBytes  int64
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

// CPUManagerState represents the kubelet cpu_manager_state JSON file.
type CPUManagerState struct {
	PolicyName    string                       `json:"policyName"`
	DefaultCPUSet string                       `json:"defaultCpuSet"`
	Entries       map[string]map[string]string `json:"entries"`
}

// CPUManagerEntry is a parsed entry from CPU manager state.
type CPUManagerEntry struct {
	PodUID        string
	ContainerName string
	CPUs          []int
	CPUSetRaw     string
}

// JSON output types — k8s-style envelope, schema versioned via APIVersion.

type jsonMetadata struct {
	Timestamp        string `json:"timestamp"`
	Host             string `json:"host,omitempty"`
	NumaCheckVersion string `json:"numaCheckVersion"`
	PID              int    `json:"pid,omitempty"`
	Pod              string `json:"pod,omitempty"`
	Container        string `json:"container,omitempty"`
}

type jsonCPUSummary struct {
	Total         int `json:"total"`
	PhysicalCores int `json:"physicalCores"`
	Sockets       int `json:"sockets"`
}

type jsonMemory struct {
	TotalBytes int64 `json:"totalBytes,omitempty"`
	FreeBytes  int64 `json:"freeBytes,omitempty"`
}

type jsonNUMANode struct {
	ID       int         `json:"id"`
	SocketID int         `json:"socketId"`
	CPUs     []int       `json:"cpus"`
	Memory   *jsonMemory `json:"memory,omitempty"`
}

type jsonGPU struct {
	Index    int    `json:"index"`
	UUID     string `json:"uuid,omitempty"`
	PCIID    string `json:"pciId"`
	NUMANode int    `json:"numaNode"`
}

type jsonCPUManagerEntry struct {
	PodUID        string `json:"podUid"`
	ContainerName string `json:"containerName"`
	CPUs          []int  `json:"cpus"`
}

type jsonCPUManagerNUMANode struct {
	NodeID        int `json:"nodeId"`
	ExclusiveCPUs int `json:"exclusiveCpus"`
	RemainingCPUs int `json:"remainingCpus"`
	TotalCPUs     int `json:"totalCpus"`
}

type jsonCPUManager struct {
	PolicyName  string                   `json:"policyName"`
	DefaultCPUs []int                    `json:"defaultCpus,omitempty"`
	Entries     []jsonCPUManagerEntry    `json:"entries,omitempty"`
	PerNUMANode []jsonCPUManagerNUMANode `json:"perNumaNode,omitempty"`
}

type jsonResources struct {
	CPURequestCores  *float64 `json:"cpuRequestCores,omitempty"`
	CPULimitCores    *float64 `json:"cpuLimitCores,omitempty"`
	MemoryLimitBytes *int64   `json:"memoryLimitBytes,omitempty"`
	GPUCount         int      `json:"gpuCount,omitempty"`
}

type jsonMachine struct {
	CPU        jsonCPUSummary  `json:"cpu"`
	Memory     *jsonMemory     `json:"memory,omitempty"`
	NUMANodes  []jsonNUMANode  `json:"numaNodes"`
	GPUs       []jsonGPU       `json:"gpus,omitempty"`
	CPUManager *jsonCPUManager `json:"cpuManager,omitempty"`
}

type jsonProcessMemPerNode struct {
	ID    int   `json:"id"`
	Bytes int64 `json:"bytes"`
}

type jsonProcess struct {
	CurrentCPU         int                     `json:"currentCpu"`
	CurrentNUMANode    int                     `json:"currentNumaNode"`
	AllowedCPUs        []int                   `json:"allowedCpus"`
	AllowedCPUCount    int                     `json:"allowedCpuCount"`
	SystemCPUCount     int                     `json:"systemCpuCount,omitempty"`
	Pinned             bool                    `json:"pinned"`
	MemoryBytes        int64                   `json:"memoryBytes,omitempty"`
	AllowedGPUs        []string                `json:"allowedGpus,omitempty"`
	MemoryPerNUMANode  []jsonProcessMemPerNode `json:"memoryPerNumaNode,omitempty"`
	ContainerResources *jsonResources          `json:"containerResources,omitempty"`
	Numastat           string                  `json:"numastat,omitempty"`
}

type jsonMachineTopology struct {
	APIVersion string       `json:"apiVersion"`
	Kind       string       `json:"kind"`
	Metadata   jsonMetadata `json:"metadata"`
	Machine    jsonMachine  `json:"machine"`
}

type jsonProcessReport struct {
	APIVersion string       `json:"apiVersion"`
	Kind       string       `json:"kind"`
	Metadata   jsonMetadata `json:"metadata"`
	Machine    jsonMachine  `json:"machine"`
	Process    *jsonProcess `json:"process,omitempty"`
}
