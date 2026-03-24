package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

type CrictlPSOutput struct {
	Containers []Container `json:"containers"`
}

type Container struct {
	ID       string            `json:"id"`
	Metadata ContainerMetadata `json:"metadata"`
	Labels   map[string]string `json:"labels"`
}

type ContainerMetadata struct {
	Name string `json:"name"`
}

type CoreInfo struct {
	PhysicalID int
	CoreID     int
}

var (
	gpuCheck      bool
	printNumastat bool
)

func main() {
	var pidFlag int
	var pod, container string

	flag.IntVar(&pidFlag, "pid", 0, "Process ID to analyze")
	flag.StringVar(&pod, "pod", "", "Pod name (for container lookup via crictl)")
	flag.StringVar(&container, "container", "", "Container name (in the pod)")
	flag.BoolVar(&gpuCheck, "gpu", false, "Perform GPU NUMA analysis")
	flag.BoolVar(&printNumastat, "numastat", false, "Print numastat output")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: numa-check [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Analyze NUMA topology for a Linux process.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	var pid int
	var err error

	if pidFlag != 0 {
		pid = pidFlag
	} else if pod != "" && container != "" {
		pid, err = getPIDFromContainer(pod, container)
		if err != nil {
			log.Fatalf("Error obtaining PID from container: %v", err)
		}
	} else {
		flag.Usage()
		os.Exit(1)
	}

	runAnalysis(pid)
}

func runAnalysis(pid int) {
	fmt.Printf("Analyzing PID: %d\n\n", pid)

	// CPU affinity via sched_getaffinity syscall.
	affinityList, err := getCPUAffinity(pid)
	if err != nil {
		log.Fatalf("Error getting CPU affinity: %v", err)
	}
	fmt.Printf("Allowed CPUs: %v\n", affinityList)

	// Check if the process is pinned to a subset of system CPUs.
	systemCPUs, err := getSystemCPUCount()
	if err != nil {
		fmt.Printf("Error reading system CPU count: %v\n", err)
	} else if len(affinityList) < systemCPUs {
		fmt.Printf("Process is pinned to %d of %d system CPUs.\n", len(affinityList), systemCPUs)
	} else {
		fmt.Printf("Process is NOT pinned (can use all %d system CPUs).\n", systemCPUs)
	}

	// Current CPU from /proc/<pid>/stat.
	currentCPU, err := getCurrentCPU(pid)
	if err != nil {
		log.Fatalf("Error getting current CPU: %v", err)
	}
	fmt.Printf("Currently running on CPU: %d\n", currentCPU)

	// NUMA topology from sysfs.
	numaMap, err := buildNUMAMap()
	if err != nil {
		log.Fatalf("Error reading NUMA topology: %v", err)
	}

	cpuNUMANode, ok := numaMap[currentCPU]
	if !ok {
		log.Fatalf("CPU %d not found in NUMA topology", currentCPU)
	}
	fmt.Printf("Current CPU NUMA node: %d\n", cpuNUMANode)

	// Optional numastat output.
	if printNumastat {
		numastatOut, err := exec.Command("numastat", "-p", fmt.Sprintf("%d", pid)).Output()
		if err != nil {
			fmt.Printf("Error running numastat: %v\n", err)
		} else {
			fmt.Printf("\nNUMA Stats:\n%s\n", string(numastatOut))
		}
	}

	// Physical core and package analysis from sysfs topology.
	uniqueCores := make(map[string]bool)
	uniquePackages := make(map[int]bool)
	for _, cpu := range affinityList {
		info, err := getCPUTopology(cpu)
		if err != nil {
			fmt.Printf("Error reading topology for CPU %d: %v\n", cpu, err)
			continue
		}
		key := fmt.Sprintf("%d-%d", info.PhysicalID, info.CoreID)
		uniqueCores[key] = true
		uniquePackages[info.PhysicalID] = true
	}
	fmt.Printf("Unique physical cores: %d, unique packages (sockets): %d\n",
		len(uniqueCores), len(uniquePackages))

	// NUMA distribution of allowed CPUs.
	numaNodes := make(map[int]bool)
	for _, cpu := range affinityList {
		if node, ok := numaMap[cpu]; ok {
			numaNodes[node] = true
		}
	}
	var nodes []int
	for node := range numaNodes {
		nodes = append(nodes, node)
	}
	sort.Ints(nodes)
	fmt.Printf("Allowed CPUs span %d NUMA node(s): %v\n", len(nodes), nodes)

	// GPU analysis (optional).
	if gpuCheck {
		fmt.Println("\nGPU NUMA Analysis:")
		runGPUAnalysis(pid, cpuNUMANode)
	}
}

// getCPUAffinity returns the sorted list of CPUs the process is allowed to run on.
func getCPUAffinity(pid int) ([]int, error) {
	var set unix.CPUSet
	if err := unix.SchedGetaffinity(pid, &set); err != nil {
		return nil, err
	}
	var cpus []int
	for i := 0; i < 1024; i++ {
		if set.IsSet(i) {
			cpus = append(cpus, i)
		}
	}
	return cpus, nil
}

// getCurrentCPU reads /proc/<pid>/stat to determine which CPU the process is currently on.
func getCurrentCPU(pid int) (int, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, err
	}
	// /proc/<pid>/stat format: pid (comm) state fields...
	// The comm field can contain spaces and parens, so find the last ')'.
	s := string(data)
	idx := strings.LastIndex(s, ")")
	if idx < 0 {
		return 0, fmt.Errorf("unexpected format in /proc/%d/stat", pid)
	}
	fields := strings.Fields(s[idx+1:])
	// After comm: fields[0]=state, fields[1]=ppid, ... fields[36]=processor (0-indexed 36, field 39 in stat)
	if len(fields) < 37 {
		return 0, fmt.Errorf("not enough fields in /proc/%d/stat", pid)
	}
	return strconv.Atoi(fields[36])
}

// getSystemCPUCount returns the total number of CPUs on the system.
func getSystemCPUCount() (int, error) {
	cpus, err := expandCPUList(readSysfsString("/sys/devices/system/cpu/possible"))
	if err != nil {
		return 0, err
	}
	return len(cpus), nil
}

// buildNUMAMap reads sysfs to build a mapping from CPU ID to NUMA node ID.
func buildNUMAMap() (map[int]int, error) {
	matches, err := filepath.Glob("/sys/devices/system/node/node[0-9]*")
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no NUMA nodes found in /sys/devices/system/node/")
	}

	cpuToNode := make(map[int]int)
	for _, nodePath := range matches {
		nodeName := filepath.Base(nodePath)
		nodeID, err := strconv.Atoi(strings.TrimPrefix(nodeName, "node"))
		if err != nil {
			continue
		}
		cpulistPath := filepath.Join(nodePath, "cpulist")
		cpus, err := expandCPUList(readSysfsString(cpulistPath))
		if err != nil {
			continue
		}
		for _, cpu := range cpus {
			cpuToNode[cpu] = nodeID
		}
	}
	return cpuToNode, nil
}

// getCPUTopology reads sysfs topology files for a given CPU.
func getCPUTopology(cpu int) (CoreInfo, error) {
	base := fmt.Sprintf("/sys/devices/system/cpu/cpu%d/topology", cpu)
	physID, err := readIntFile(filepath.Join(base, "physical_package_id"))
	if err != nil {
		return CoreInfo{}, err
	}
	coreID, err := readIntFile(filepath.Join(base, "core_id"))
	if err != nil {
		return CoreInfo{}, err
	}
	return CoreInfo{PhysicalID: physID, CoreID: coreID}, nil
}

// readIntFile reads a sysfs file containing a single integer.
func readIntFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// readSysfsString reads a sysfs file and returns its trimmed contents.
func readSysfsString(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// expandCPUList parses a CPU list string like "0-3,8-11" into individual CPU IDs.
func expandCPUList(s string) ([]int, error) {
	if s == "" {
		return nil, fmt.Errorf("empty CPU list")
	}
	var cpus []int
	for _, token := range strings.Split(strings.ReplaceAll(s, " ", ","), ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if strings.Contains(token, "-") {
			parts := strings.SplitN(token, "-", 2)
			start, err := strconv.Atoi(parts[0])
			if err != nil {
				return nil, fmt.Errorf("invalid CPU range %q: %v", token, err)
			}
			end, err := strconv.Atoi(parts[1])
			if err != nil {
				return nil, fmt.Errorf("invalid CPU range %q: %v", token, err)
			}
			for i := start; i <= end; i++ {
				cpus = append(cpus, i)
			}
		} else {
			n, err := strconv.Atoi(token)
			if err != nil {
				return nil, fmt.Errorf("invalid CPU number %q: %v", token, err)
			}
			cpus = append(cpus, n)
		}
	}
	return cpus, nil
}

// GPU analysis functions.

func runGPUAnalysis(pid int, cpuNUMANode int) {
	allowedGPUs, err := getAllowedGPUs(pid)
	if err != nil {
		fmt.Printf("Error reading process environment: %v\n", err)
		return
	}
	if len(allowedGPUs) == 0 {
		fmt.Println("NVIDIA_VISIBLE_DEVICES not set; process can access all GPUs.")
	} else {
		fmt.Printf("Process allowed GPUs (by UUID): %v\n", allowedGPUs)
	}

	gpuMap, err := getGPUInfo()
	if err != nil {
		fmt.Printf("Error retrieving GPU info via nvidia-smi: %v\n", err)
		return
	}

	if len(allowedGPUs) == 0 {
		for uuid := range gpuMap {
			allowedGPUs = append(allowedGPUs, uuid)
		}
	}

	for _, gpuUUID := range allowedGPUs {
		pciID, found := gpuMap[gpuUUID]
		if !found {
			fmt.Printf("GPU %s not found in nvidia-smi output.\n", gpuUUID)
			continue
		}
		gpuNUMANode, err := readIntFile(filepath.Join("/sys/bus/pci/devices", pciID, "numa_node"))
		if err != nil {
			fmt.Printf("Error reading NUMA node for GPU %s (PCI: %s): %v\n", gpuUUID, pciID, err)
			continue
		}
		if gpuNUMANode == cpuNUMANode {
			fmt.Printf("GPU %s (PCI: %s): NUMA node %d (same as CPU)\n", gpuUUID, pciID, gpuNUMANode)
		} else {
			fmt.Printf("GPU %s (PCI: %s): NUMA node %d (DIFFERENT from CPU node %d)\n", gpuUUID, pciID, gpuNUMANode, cpuNUMANode)
		}
	}
}

func getAllowedGPUs(pid int) ([]string, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		return nil, err
	}
	for _, env := range strings.Split(string(data), "\x00") {
		if strings.HasPrefix(env, "NVIDIA_VISIBLE_DEVICES=") {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("unexpected format in NVIDIA_VISIBLE_DEVICES")
			}
			return strings.Split(parts[1], ","), nil
		}
	}
	return nil, nil
}

func getGPUInfo() (map[string]string, error) {
	out, err := exec.Command("nvidia-smi", "--query-gpu=uuid,pci.bus_id", "--format=csv,noheader").Output()
	if err != nil {
		return nil, err
	}
	gpuMap := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, ",", 2)
		if len(parts) < 2 {
			continue
		}
		uuid := strings.TrimSpace(parts[0])
		pciID := normalizePCI(strings.TrimSpace(parts[1]))
		gpuMap[uuid] = pciID
	}
	return gpuMap, nil
}

func normalizePCI(pciID string) string {
	normalized := strings.ToLower(strings.TrimSpace(pciID))
	if strings.HasPrefix(normalized, "00000000:") {
		normalized = "0000:" + normalized[len("00000000:"):]
	}
	return normalized
}

// getPIDFromContainer uses crictl to find a container by pod and container name,
// then returns its host PID.
func getPIDFromContainer(podName, containerName string) (int, error) {
	out, err := exec.Command("crictl", "ps", "-o", "json").Output()
	if err != nil {
		return 0, fmt.Errorf("failed to run crictl ps: %v", err)
	}

	var psOut CrictlPSOutput
	if err := json.Unmarshal(out, &psOut); err != nil {
		return 0, fmt.Errorf("failed to parse crictl ps output: %v", err)
	}

	var targetContainerID string
	for _, ctr := range psOut.Containers {
		if ctr.Metadata.Name == containerName {
			if podLabel, ok := ctr.Labels["io.kubernetes.pod.name"]; ok && podLabel == podName {
				targetContainerID = ctr.ID
				break
			}
		}
	}
	if targetContainerID == "" {
		return 0, fmt.Errorf("container %q in pod %q not found", containerName, podName)
	}

	inspectOut, err := exec.Command("crictl", "inspect", targetContainerID, "-o", "json").Output()
	if err != nil {
		return 0, fmt.Errorf("crictl inspect failed: %v", err)
	}

	var inspectData map[string]interface{}
	if err := json.Unmarshal(inspectOut, &inspectData); err != nil {
		return 0, fmt.Errorf("failed to parse crictl inspect output: %v", err)
	}

	info, ok := inspectData["info"].(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("unexpected structure in crictl inspect output")
	}
	pidFloat, ok := info["pid"].(float64)
	if !ok {
		return 0, fmt.Errorf("PID not found in crictl inspect output")
	}

	return int(pidFloat), nil
}
