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

type CPU struct {
	LogicalCores  int `json:"logicalCores"`
	PhysicalCores int `json:"physicalCores"`
	Sockets       int `json:"sockets"`
}

type Memory struct {
	TotalBytes int64 `json:"totalBytes,omitempty"`
	FreeBytes  int64 `json:"freeBytes,omitempty"`
}

type GPU struct {
	Index    int    `json:"index"`
	UUID     string `json:"uuid,omitempty"`
	PCIID    string `json:"pciId"`
	NUMANode int    `json:"numaNode"`
}

type MachineResources struct {
	CPU    CPU     `json:"cpu"`
	Memory *Memory `json:"memory,omitempty"`
	GPUs   []GPU   `json:"gpus,omitempty"`
}

type NUMANode struct {
	ID       int     `json:"id"`
	SocketID int     `json:"socketId"`
	CPUs     []int   `json:"cpus"`
	GPUs     []int   `json:"gpus,omitempty"`
	Memory   *Memory `json:"memory,omitempty"`
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

type MachineData struct {
	Resources  MachineResources `json:"resources"`
	NUMANodes  []NUMANode       `json:"numaNodes"`
	CPUManager *CPUManager      `json:"cpuManager,omitempty"`
}

type Placement struct {
	CurrentCPU      int  `json:"currentCpu"`
	CurrentNUMANode int  `json:"currentNumaNode"`
	Pinned          bool `json:"pinned"`
}

type ProcessMemPerNode struct {
	ID    int   `json:"id"`
	Bytes int64 `json:"bytes"`
}

type ProcessMemory struct {
	Bytes       int64               `json:"bytes,omitempty"`
	PerNUMANode []ProcessMemPerNode `json:"perNumaNode,omitempty"`
}

type Affinity struct {
	CPUs   []int          `json:"cpus"`
	GPUs   []string       `json:"gpus,omitempty"`
	Memory *ProcessMemory `json:"memory,omitempty"`
}

// ResourceList mirrors k8s ResourceList: a map of resource name to a
// resource.Quantity string (e.g., "4", "500m", "16Gi", "1").
type ResourceList map[string]string

type ContainerResources struct {
	Requests ResourceList `json:"requests,omitempty"`
	Limits   ResourceList `json:"limits,omitempty"`
}

type Container struct {
	Resources ContainerResources `json:"resources"`
}

type Process struct {
	Placement Placement  `json:"placement"`
	Affinity  Affinity   `json:"affinity"`
	Container *Container `json:"container,omitempty"`
}

type ProcessData struct {
	Process Process `json:"process"`
}

type MachineTopology struct {
	APIVersion string      `json:"apiVersion"`
	Kind       string      `json:"kind"`
	Metadata   Metadata    `json:"metadata"`
	Data       MachineData `json:"data"`
}

type ProcessReport struct {
	APIVersion string      `json:"apiVersion"`
	Kind       string      `json:"kind"`
	Metadata   Metadata    `json:"metadata"`
	Data       ProcessData `json:"data"`
}
