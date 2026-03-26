package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

var version = "dev"

type config struct {
	pid       int
	pod       string
	container string
	numastat  bool
	topoOnly  bool
	debug     bool
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "numa-check: "+format+"\n", args...)
	os.Exit(1)
}

func warnf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "numa-check: warning: "+format+"\n", args...)
}

func main() {
	var cfg config
	var showVersion bool

	flag.IntVar(&cfg.pid, "pid", 0, "Process ID to analyze")
	flag.StringVar(&cfg.pod, "pod", "", "Pod name (for container lookup via crictl)")
	flag.StringVar(&cfg.container, "container", "", "Container name (in the pod)")
	flag.BoolVar(&cfg.numastat, "numastat", false, "Print numastat memory stats")
	flag.BoolVar(&cfg.topoOnly, "topo", false, "Show machine topology only (no process analysis)")
	flag.BoolVar(&cfg.debug, "debug", false, "Enable debug logging")
	flag.BoolVar(&showVersion, "version", false, "Print version and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: numa-check [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Analyze NUMA topology for a Linux process.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if showVersion {
		fmt.Printf("numa-check %s\n", version)
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
		runTopoOnly(fs, cmd)
		return
	}

	pid := cfg.pid
	switch {
	case pid != 0:
		// PID provided directly via -pid flag.
	case cfg.pod != "" || cfg.container != "":
		if cfg.pod == "" {
			fatalf("-pod is required when -container is set")
		}
		if cfg.container == "" {
			fatalf("-container is required when -pod is set")
		}
		var err error
		pid, err = getPIDFromContainer(cmd, cfg.pod, cfg.container)
		if err != nil {
			fatalf("container lookup failed: %v", err)
		}
	default:
		if cfg.numastat {
			fatalf("-numastat requires -pid or -pod/-container")
		}
		flag.Usage()
		os.Exit(1)
	}

	runAnalysis(fs, cmd, pid, cfg.numastat)
}

func runTopoOnly(fs FileSystem, cmd CommandRunner) {
	numaMap, err := buildNUMAMap(fs)
	if err != nil {
		fatalf("reading NUMA topology: %v", err)
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

	fmt.Printf("\n%s\n\n", col(ansiBold, "numa-check — Machine Topology"))
	printSection("Topology")

	summary := fmt.Sprintf("  %d CPUs (%d physical cores), %d NUMA nodes, %d sockets",
		len(numaMap), len(allCores), len(nodes), len(totalSockets))
	if len(gpus) > 0 {
		summary += fmt.Sprintf(", %d GPUs", len(gpus))
	}
	fmt.Printf("%s\n\n", summary)

	printNodesGrid(nodes, ModeMachine, nil, -1, nil, nil)
	fmt.Println()
}

func runAnalysis(fs FileSystem, cmd CommandRunner, pid int, showNumastat bool) {
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

	fmt.Printf("\n%s\n\n", col(ansiBold, fmt.Sprintf("numa-check — PID %d", pid)))

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

	if len(gpus) > 0 && !gpuEnvErr {
		if allowedGPUs == nil {
			fmt.Printf("  Allowed GPUs ......... all %d GPUs\n", len(gpus))
		} else {
			fmt.Printf("  Allowed GPUs ......... %d / %d\n", len(allowedGPUs), len(gpus))
		}
	}
	fmt.Println()

	fmt.Printf("  %s = allowed  %s = current  %s = not allowed\n\n",
		col(ansiGreen, "■"), col(ansiBrightYellow, "★"), col(ansiDim, "□"))

	processGridNodes := nodes
	if gpuEnvErr {
		processGridNodes = make([]NUMANodeInfo, len(nodes))
		for i, n := range nodes {
			processGridNodes[i] = NUMANodeInfo{ID: n.ID, SocketID: n.SocketID, CPUs: n.CPUs}
		}
	}
	printNodesGrid(processGridNodes, ModeProcess, allowedSet, currentCPU, processNodes, allowedGPUs)

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
