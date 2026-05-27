package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

var version = "dev"

// Test seams for buildMetadata. Tests can swap these and restore via t.Cleanup.
var (
	hostnameFn = os.Hostname
	nowFn      = time.Now
)

type config struct {
	pid        int
	pod        string
	container  string
	numastat   bool
	topoOnly   bool
	debug      bool
	jsonOut    bool
	cpuManager string
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "numacheck: "+format+"\n", args...)
	os.Exit(1)
}

func warnf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "numacheck: warning: "+format+"\n", args...)
}

func main() {
	var cfg config
	var showVersion bool

	flag.IntVar(&cfg.pid, "pid", 0, "process ID to analyze")
	flag.StringVar(&cfg.pod, "pod", "", "pod name (requires -container)")
	flag.StringVar(&cfg.container, "container", "", "container name (requires -pod)")
	flag.BoolVar(&cfg.numastat, "numastat", false, "include numastat memory stats")
	flag.BoolVar(&cfg.topoOnly, "topo", false, "machine topology only (no PID required)")
	flag.BoolVar(&cfg.debug, "debug", false, "enable debug logging")
	flag.BoolVar(&cfg.jsonOut, "json", false, "output in JSON format")
	flag.StringVar(&cfg.cpuManager, "cpumanager", "", "path to kubelet cpu_manager_state file")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: numacheck [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Analyze NUMA topology for a Linux process.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  numacheck -topo                              machine topology only
  numacheck -pid 12345                         analyze a process
  numacheck -pod mypod -container mycontainer  analyze a container
  numacheck -pid 12345 -numastat               include NUMA memory stats
  numacheck -pid 12345 -json                   JSON output
  numacheck -topo -cpumanager /var/lib/kubelet/cpu_manager_state
`)
	}
	flag.Parse()

	if showVersion {
		fmt.Printf("numacheck %s\n", version)
		return
	}

	if cfg.debug {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
	} else {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1})))
	}

	if flag.NArg() > 0 {
		fatalf("unexpected arguments: %s", strings.Join(flag.Args(), " "))
	}

	if cfg.topoOnly && cfg.numastat {
		fatalf("-numastat cannot be used with -topo")
	}
	if cfg.topoOnly && cfg.pid != 0 {
		fatalf("-pid cannot be used with -topo")
	}
	if cfg.topoOnly && (cfg.pod != "" || cfg.container != "") {
		fatalf("-pod/-container cannot be used with -topo")
	}

	fs := osFS{}
	cmd := execRunner{}

	if cfg.topoOnly {
		if err := runTopoOnly(fs, cmd, cfg.jsonOut, cfg.cpuManager); err != nil {
			fatalf("%v", err)
		}
		return
	}

	var pid int
	var containerRes *crictlResources
	switch {
	case cfg.pid != 0:
		pid = cfg.pid
	case cfg.pod != "" || cfg.container != "":
		if cfg.pod == "" {
			fatalf("-pod is required when -container is set")
		}
		if cfg.container == "" {
			fatalf("-container is required when -pod is set")
		}
		info, err := getContainerInfo(cmd, cfg.pod, cfg.container)
		if err != nil {
			fatalf("container lookup failed: %v", err)
		}
		pid = info.PID
		containerRes = &info.Resources
	default:
		if cfg.numastat {
			fatalf("-numastat requires -pid or -pod/-container")
		}
		flag.Usage()
		os.Exit(1)
	}

	runAnalysis(fs, cmd, pid, cfg.numastat, cfg.jsonOut, containerRes, cfg.cpuManager, cfg.pod, cfg.container)
}

func runTopoOnly(fs FileSystem, cmd CommandRunner, jsonOut bool, cpuManagerPath string) error {
	if jsonOut {
		out, err := buildTopoJSON(fs, cmd, cpuManagerPath)
		if err != nil {
			return err
		}
		printJSON(out)
		return nil
	}

	numaMap, err := buildNUMAMap(fs)
	if err != nil {
		return fmt.Errorf("reading NUMA topology: %v", err)
	}

	gpus, gpuErr := discoverGPUs(fs, cmd)
	if gpuErr != nil {
		warnf("GPU detection: %v", gpuErr)
	}
	nodes := buildNUMANodes(fs, numaMap, gpus)

	totalSockets := make(map[int]bool)
	for _, n := range nodes {
		if n.SocketID >= 0 {
			totalSockets[n.SocketID] = true
		}
	}

	allCores := make(map[CoreInfo]bool)
	for cpu := range numaMap {
		if info, err := getCPUTopology(fs, cpu); err == nil {
			allCores[info] = true
		}
	}

	var cpuMgrState *CPUManagerState
	var cpuMgrEntries []CPUManagerEntry
	if cpuManagerPath != "" {
		state, err := readCPUManagerState(fs, cpuManagerPath)
		if err != nil {
			warnf("cpu manager: %v", err)
		} else {
			cpuMgrState = state
			if state.PolicyName == "static" {
				cpuMgrEntries = parseCPUManagerEntries(state)
			}
		}
	}

	totalMem := sumNodeMemTotals(nodes)

	fmt.Printf("\n%s\n\n", col(ansiBold, "numacheck — Machine Topology"))
	printSection("Topology")

	summary := fmt.Sprintf("  %d CPUs (%d physical cores), %d NUMA nodes, %d sockets",
		len(numaMap), len(allCores), len(nodes), len(totalSockets))
	if totalMem > 0 {
		summary += fmt.Sprintf(", %s memory", formatBytes(totalMem))
	}
	if len(gpus) > 0 {
		summary += fmt.Sprintf(", %d GPUs", len(gpus))
	}
	fmt.Printf("%s\n\n", summary)

	printNodesGrid(nodes, ModeMachine, nil, -1, nil, nil, nil)

	if cpuMgrState != nil {
		fmt.Println()
		if cpuMgrState.PolicyName != "static" {
			fmt.Printf("  CPU Manager policy is %q — CPUs are not exclusively assigned to containers\n", cpuMgrState.PolicyName)
		} else {
			printCPUManagerSection(cpuMgrState, cpuMgrEntries, nodes)
		}
	}

	fmt.Println()
	return nil
}

// buildTopoJSON gathers topology data and assembles the MachineTopology JSON envelope.
// Returns an error only on unrecoverable failures (e.g., NUMA map unreadable). GPU and
// CPU-manager errors are surfaced via warnf, matching the text path.
func buildTopoJSON(fs FileSystem, cmd CommandRunner, cpuManagerPath string) (jsonMachineTopology, error) {
	numaMap, err := buildNUMAMap(fs)
	if err != nil {
		return jsonMachineTopology{}, fmt.Errorf("reading NUMA topology: %v", err)
	}

	gpus, gpuErr := discoverGPUs(fs, cmd)
	if gpuErr != nil {
		warnf("GPU detection: %v", gpuErr)
	}
	nodes := buildNUMANodes(fs, numaMap, gpus)

	var cpuMgrState *CPUManagerState
	var cpuMgrEntries []CPUManagerEntry
	if cpuManagerPath != "" {
		state, err := readCPUManagerState(fs, cpuManagerPath)
		if err != nil {
			warnf("cpu manager: %v", err)
		} else {
			cpuMgrState = state
			if state.PolicyName == "static" {
				cpuMgrEntries = parseCPUManagerEntries(state)
			}
		}
	}

	return jsonMachineTopology{
		APIVersion: "numacheck/v1",
		Kind:       "MachineTopology",
		Metadata:   buildMetadata(),
		Machine:    buildMachine(fs, numaMap, nodes, gpus, cpuMgrState, cpuMgrEntries),
	}, nil
}

func runAnalysis(fs FileSystem, cmd CommandRunner, pid int, showNumastat, jsonOut bool, containerRes *crictlResources, cpuManagerPath string, pod, container string) {
	affinityList, err := getCPUAffinity(pid)
	if err != nil {
		fatalf("getting CPU affinity: %v", err)
	}

	systemCPUs, systemCPUErr := getSystemCPUCount(fs)

	currentCPU, err := getCurrentCPU(fs, pid)
	if err != nil {
		fatalf("getting current CPU: %v", err)
	}

	numaMap, err := buildNUMAMap(fs)
	if err != nil {
		fatalf("reading NUMA topology: %v", err)
	}

	cpuNUMANode, ok := numaMap[currentCPU]
	if !ok {
		fatalf("CPU %d not found in NUMA topology", currentCPU)
	}

	gpus, gpuErr := discoverGPUs(fs, cmd)
	if gpuErr != nil {
		warnf("GPU detection: %v", gpuErr)
	}
	nodes := buildNUMANodes(fs, numaMap, gpus)

	allowedSet := make(map[int]bool, len(affinityList))
	processNodes := make(map[int]bool)
	for _, cpu := range affinityList {
		allowedSet[cpu] = true
		if node, ok := numaMap[cpu]; ok {
			processNodes[node] = true
		}
	}

	var allowedGPUs map[string]bool
	var gpuEnvErr bool
	if len(gpus) > 0 {
		procGPUs, err := getAllowedGPUs(fs, pid, gpus)
		if err != nil {
			warnf("could not read process GPU environment: %v", err)
			gpuEnvErr = true
		} else if procGPUs != nil {
			allowedGPUs = make(map[string]bool, len(procGPUs))
			for _, uuid := range procGPUs {
				allowedGPUs[uuid] = true
			}
		}
	}

	processMem, memErr := readProcessNUMAMemory(fs, pid)
	if memErr != nil {
		warnf("could not read process NUMA memory: %v", memErr)
		processMem = nil
	}

	var cpuMgrState *CPUManagerState
	var cpuMgrEntries []CPUManagerEntry
	if cpuManagerPath != "" {
		state, err := readCPUManagerState(fs, cpuManagerPath)
		if err != nil {
			warnf("cpu manager: %v", err)
		} else {
			cpuMgrState = state
			if state.PolicyName == "static" {
				cpuMgrEntries = parseCPUManagerEntries(state)
			}
		}
	}

	if jsonOut {
		pinned := systemCPUErr == nil && len(affinityList) < systemCPUs
		md := buildMetadata()
		md.PID = pid
		md.Pod = pod
		md.Container = container

		if affinityList == nil {
			affinityList = []int{}
		}

		out := jsonProcessReport{
			APIVersion: "numacheck/v1",
			Kind:       "ProcessReport",
			Metadata:   md,
			Machine:    buildMachine(fs, numaMap, nodes, gpus, cpuMgrState, cpuMgrEntries),
			Process: &jsonProcess{
				CurrentCPU:        currentCPU,
				CurrentNUMANode:   cpuNUMANode,
				AllowedCPUs:       affinityList,
				AllowedCPUCount:   len(affinityList),
				Pinned:            pinned,
				MemoryBytes:       sumProcessMem(processMem),
				AllowedGPUs:       mapKeys(allowedGPUs),
				MemoryPerNUMANode: toJSONProcessMemPerNode(nodes, processMem),
			},
		}
		if systemCPUErr == nil {
			out.Process.SystemCPUCount = systemCPUs
		}
		if containerRes != nil {
			out.Process.ContainerResources = toJSONResources(*containerRes, gpuCount(allowedGPUs, gpus, gpuEnvErr))
		}
		if showNumastat {
			raw, err := cmd.Run("numastat", "-p", fmt.Sprintf("%d", pid))
			if err == nil {
				out.Process.Numastat = strings.TrimSpace(string(raw))
			}
		}
		printJSON(out)
		return
	}

	fmt.Printf("\n%s\n\n", col(ansiBold, fmt.Sprintf("numacheck — PID %d", pid)))

	printSection(fmt.Sprintf("Process — PID %d", pid))

	if systemCPUErr != nil {
		warnf("could not determine system CPU count: %v", systemCPUErr)
		fmt.Printf("  Allowed CPUs ......... %d\n", len(affinityList))
	} else {
		pinLabel := col(ansiGreen, "pinned")
		if len(affinityList) >= systemCPUs {
			pinLabel = col(ansiBrightYellow, "not pinned")
		}
		fmt.Printf("  Allowed CPUs ......... %d / %d (%s)\n", len(affinityList), systemCPUs, pinLabel)
	}
	fmt.Printf("  Currently on ......... CPU %d → NUMA Node %d\n", currentCPU, cpuNUMANode)

	if processMem != nil {
		totalProc := sumProcessMem(processMem)
		totalNode := sumNodeMemTotals(nodes)
		if totalNode > 0 {
			fmt.Printf("  Memory ............... %s used / %s total\n", formatBytes(totalProc), formatBytes(totalNode))
		} else {
			fmt.Printf("  Memory ............... %s used\n", formatBytes(totalProc))
		}
	}

	if len(gpus) > 0 && !gpuEnvErr {
		if allowedGPUs == nil {
			fmt.Printf("  Allowed GPUs ......... all %d GPUs\n", len(gpus))
		} else {
			fmt.Printf("  Allowed GPUs ......... %d / %d\n", len(allowedGPUs), len(gpus))
		}
	}
	fmt.Println()

	if containerRes != nil {
		resText := formatResources(*containerRes, gpuCount(allowedGPUs, gpus, gpuEnvErr))
		if resText != "" {
			printSection("Container Resources")
			fmt.Print(resText)
			fmt.Println()
		}
	}

	fmt.Printf("  %s = allowed  %s = current  %s = not allowed\n\n",
		col(ansiGreen, "■"), col(ansiBrightYellow, "★"), col(ansiDim, "□"))

	processGridNodes := nodes
	if gpuEnvErr {
		processGridNodes = make([]NUMANodeInfo, len(nodes))
		for i, n := range nodes {
			processGridNodes[i] = NUMANodeInfo{ID: n.ID, SocketID: n.SocketID, CPUs: n.CPUs}
		}
	}
	printNodesGrid(processGridNodes, ModeProcess, allowedSet, currentCPU, processNodes, allowedGPUs, processMem)

	if cpuMgrState != nil {
		fmt.Println()
		if cpuMgrState.PolicyName != "static" {
			fmt.Printf("  CPU Manager policy is %q — CPUs are not exclusively assigned to containers\n", cpuMgrState.PolicyName)
		} else {
			printCPUManagerSection(cpuMgrState, cpuMgrEntries, nodes)
		}
	}

	if showNumastat {
		fmt.Println()
		out, err := runNumastat(cmd, pid)
		if err != nil {
			warnf("numastat: %v", err)
		} else {
			printSection("NUMA Memory Stats")
			fmt.Print(out)
		}
	}

	fmt.Println()
}

// sumNodeMemTotals returns the sum of MemTotalBytes across nodes.
func sumNodeMemTotals(nodes []NUMANodeInfo) int64 {
	var total int64
	for _, n := range nodes {
		total += n.MemTotalBytes
	}
	return total
}

// sumProcessMem returns the sum of bytes across all NUMA nodes in processMem.
// A nil map sums to 0.
func sumProcessMem(processMem map[int]int64) int64 {
	var total int64
	for _, b := range processMem {
		total += b
	}
	return total
}

func toJSONNodes(nodes []NUMANodeInfo) []jsonNUMANode {
	out := make([]jsonNUMANode, len(nodes))
	for i, n := range nodes {
		jn := jsonNUMANode{
			ID:       n.ID,
			SocketID: n.SocketID,
			CPUs:     n.CPUs,
		}
		if n.MemTotalBytes != 0 || n.MemFreeBytes != 0 {
			jn.Memory = &jsonMemory{TotalBytes: n.MemTotalBytes, FreeBytes: n.MemFreeBytes}
		}
		out[i] = jn
	}
	return out
}

func toJSONProcessMemPerNode(nodes []NUMANodeInfo, processMem map[int]int64) []jsonProcessMemPerNode {
	if processMem == nil {
		return nil
	}
	out := make([]jsonProcessMemPerNode, 0, len(nodes))
	for _, n := range nodes {
		if b, ok := processMem[n.ID]; ok {
			out = append(out, jsonProcessMemPerNode{ID: n.ID, Bytes: b})
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func toJSONGPUs(gpus []GPUDevice) []jsonGPU {
	if len(gpus) == 0 {
		return nil
	}
	out := make([]jsonGPU, len(gpus))
	for i, g := range gpus {
		out[i] = jsonGPU{Index: g.Index, UUID: g.UUID, PCIID: g.PCIID, NUMANode: g.NUMANode}
	}
	return out
}

func toJSONResources(res crictlResources, gc int) *jsonResources {
	jr := &jsonResources{}
	hasContent := false
	if res.CPUShares > 2 {
		v := float64(res.CPUShares) / 1024
		jr.CPURequestCores = &v
		hasContent = true
	}
	if res.CPUQuota > 0 && res.CPUPeriod > 0 {
		v := float64(res.CPUQuota) / float64(res.CPUPeriod)
		jr.CPULimitCores = &v
		hasContent = true
	}
	if res.MemoryLimitInBytes > 0 {
		jr.MemoryLimitBytes = &res.MemoryLimitInBytes
		hasContent = true
	}
	if gc > 0 {
		jr.GPUCount = gc
		hasContent = true
	}
	if !hasContent {
		return nil
	}
	return jr
}

func buildMachine(fs FileSystem, numaMap map[int]int, nodes []NUMANodeInfo, gpus []GPUDevice, cpuMgrState *CPUManagerState, cpuMgrEntries []CPUManagerEntry) jsonMachine {
	allCores := make(map[CoreInfo]bool)
	for cpu := range numaMap {
		if info, err := getCPUTopology(fs, cpu); err == nil {
			allCores[info] = true
		}
	}
	totalSockets := make(map[int]bool)
	for _, n := range nodes {
		if n.SocketID >= 0 {
			totalSockets[n.SocketID] = true
		}
	}

	m := jsonMachine{
		CPU: jsonCPUSummary{
			Total:         len(numaMap),
			PhysicalCores: len(allCores),
			Sockets:       len(totalSockets),
		},
		NUMANodes: toJSONNodes(nodes),
		GPUs:      toJSONGPUs(gpus),
	}
	if total := sumNodeMemTotals(nodes); total != 0 {
		m.Memory = &jsonMemory{TotalBytes: total}
	}
	if cpuMgrState != nil {
		m.CPUManager = toJSONCPUManager(cpuMgrState, cpuMgrEntries, nodes)
	}
	return m
}

func buildMetadata() jsonMetadata {
	hostname, err := hostnameFn()
	if err != nil {
		slog.Debug("os.Hostname failed", "error", err)
		hostname = ""
	}
	return jsonMetadata{
		Timestamp:        nowFn().UTC().Format(time.RFC3339),
		Host:             hostname,
		NumacheckVersion: version,
	}
}

func gpuCount(allowedGPUs map[string]bool, gpus []GPUDevice, envErr bool) int {
	if allowedGPUs != nil {
		return len(allowedGPUs)
	}
	if len(gpus) > 0 && !envErr {
		return len(gpus)
	}
	return 0
}

func mapKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fatalf("encoding JSON: %v", err)
	}
}
