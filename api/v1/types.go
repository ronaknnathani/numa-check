// Package v1 defines the wire types for numacheck's JSON output.
// Schema is versioned via the APIVersion field on top-level envelopes
// (e.g., "numacheck/v1"). JSON field tags are part of the contract and
// must not change without bumping the version.
//
// Callers conventionally alias this package as `apiv1`:
//
//	import apiv1 "numacheck/api/v1"
package v1

type Metadata struct {
	Timestamp        string `json:"timestamp"`
	Host             string `json:"host,omitempty"`
	NumacheckVersion string `json:"numacheckVersion"`
	PID              int    `json:"pid,omitempty"`
	Pod              string `json:"pod,omitempty"`
	Container        string `json:"container,omitempty"`
}

type CPUSummary struct {
	Total         int `json:"total"`
	PhysicalCores int `json:"physicalCores"`
	Sockets       int `json:"sockets"`
}

type Memory struct {
	TotalBytes int64 `json:"totalBytes,omitempty"`
	FreeBytes  int64 `json:"freeBytes,omitempty"`
}

type NUMANode struct {
	ID       int     `json:"id"`
	SocketID int     `json:"socketId"`
	CPUs     []int   `json:"cpus"`
	Memory   *Memory `json:"memory,omitempty"`
}

type GPU struct {
	Index    int    `json:"index"`
	UUID     string `json:"uuid,omitempty"`
	PCIID    string `json:"pciId"`
	NUMANode int    `json:"numaNode"`
}

type CPUManagerEntry struct {
	PodUID        string `json:"podUid"`
	ContainerName string `json:"containerName"`
	CPUs          []int  `json:"cpus"`
}

type CPUManagerNUMANode struct {
	NodeID        int `json:"nodeId"`
	ExclusiveCPUs int `json:"exclusiveCpus"`
	RemainingCPUs int `json:"remainingCpus"`
	TotalCPUs     int `json:"totalCpus"`
}

type CPUManager struct {
	PolicyName  string               `json:"policyName"`
	DefaultCPUs []int                `json:"defaultCpus,omitempty"`
	Entries     []CPUManagerEntry    `json:"entries,omitempty"`
	PerNUMANode []CPUManagerNUMANode `json:"perNumaNode,omitempty"`
}

type Resources struct {
	CPURequestCores  *float64 `json:"cpuRequestCores,omitempty"`
	CPULimitCores    *float64 `json:"cpuLimitCores,omitempty"`
	MemoryLimitBytes *int64   `json:"memoryLimitBytes,omitempty"`
	GPUCount         int      `json:"gpuCount,omitempty"`
}

type Machine struct {
	CPU        CPUSummary  `json:"cpu"`
	Memory     *Memory     `json:"memory,omitempty"`
	NUMANodes  []NUMANode  `json:"numaNodes"`
	GPUs       []GPU       `json:"gpus,omitempty"`
	CPUManager *CPUManager `json:"cpuManager,omitempty"`
}

type ProcessMemPerNode struct {
	ID    int   `json:"id"`
	Bytes int64 `json:"bytes"`
}

type Process struct {
	CurrentCPU         int                 `json:"currentCpu"`
	CurrentNUMANode    int                 `json:"currentNumaNode"`
	AllowedCPUs        []int               `json:"allowedCpus"`
	AllowedCPUCount    int                 `json:"allowedCpuCount"`
	SystemCPUCount     int                 `json:"systemCpuCount,omitempty"`
	Pinned             bool                `json:"pinned"`
	MemoryBytes        int64               `json:"memoryBytes,omitempty"`
	AllowedGPUs        []string            `json:"allowedGpus,omitempty"`
	MemoryPerNUMANode  []ProcessMemPerNode `json:"memoryPerNumaNode,omitempty"`
	ContainerResources *Resources          `json:"containerResources,omitempty"`
	Numastat           string              `json:"numastat,omitempty"`
}

type MachineTopology struct {
	APIVersion string   `json:"apiVersion"`
	Kind       string   `json:"kind"`
	Metadata   Metadata `json:"metadata"`
	Machine    Machine  `json:"machine"`
}

type ProcessReport struct {
	APIVersion string   `json:"apiVersion"`
	Kind       string   `json:"kind"`
	Metadata   Metadata `json:"metadata"`
	Machine    Machine  `json:"machine"`
	Process    *Process `json:"process,omitempty"`
}
