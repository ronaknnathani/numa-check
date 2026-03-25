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
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

// Types for crictl JSON output.

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

type GPUDevice struct {
	UUID     string
	PCIID    string
	NUMANode int
}

type NUMANodeInfo struct {
	ID       int
	SocketID int
	CPUs     []int // sorted
	GPUs     []GPUDevice
}

// ANSI color codes.
const (
	ansiReset        = "\033[0m"
	ansiBold         = "\033[1m"
	ansiDim          = "\033[2m"
	ansiRed          = "\033[31m"
	ansiGreen        = "\033[32m"
	ansiCyan         = "\033[36m"
	ansiBrightYellow = "\033[93m"
)

var (
	gpuCheck      bool
	printNumastat bool
	topoOnly      bool
	useColor      bool
)

func init() {
	if fi, err := os.Stdout.Stat(); err == nil {
		useColor = fi.Mode()&os.ModeCharDevice != 0
	}
}

func col(code, text string) string {
	if !useColor {
		return text
	}
	return code + text + ansiReset
}

func main() {
	var pidFlag int
	var pod, container string

	flag.IntVar(&pidFlag, "pid", 0, "Process ID to analyze")
	flag.StringVar(&pod, "pod", "", "Pod name (for container lookup via crictl)")
	flag.StringVar(&container, "container", "", "Container name (in the pod)")
	flag.BoolVar(&gpuCheck, "gpu", false, "Perform GPU NUMA analysis")
	flag.BoolVar(&printNumastat, "numastat", false, "Print numastat output")
	flag.BoolVar(&topoOnly, "topo", false, "Show machine topology only (no process analysis)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: numa-check [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Analyze NUMA topology for a Linux process.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if topoOnly {
		runTopoOnly()
		return
	}

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

func runTopoOnly() {
	numaMap, err := buildNUMAMap()
	if err != nil {
		log.Fatalf("Error reading NUMA topology: %v", err)
	}

	gpus := discoverGPUs()
	nodes := buildNUMANodes(numaMap, gpus)

	totalSockets := make(map[int]bool)
	for _, n := range nodes {
		if n.SocketID >= 0 {
			totalSockets[n.SocketID] = true
		}
	}

	fmt.Printf("\n%s\n\n", col(ansiBold, "numa-check — Machine Topology"))
	printSection("Topology")

	summary := fmt.Sprintf("  %d CPUs, %d NUMA nodes, %d sockets", len(numaMap), len(nodes), len(totalSockets))
	if len(gpus) > 0 {
		summary += fmt.Sprintf(", %d GPUs", len(gpus))
	}
	fmt.Printf("%s\n\n", summary)

	printNodesGrid(nodes, "machine", nil, -1, nil)
	fmt.Println()
}

func runAnalysis(pid int) {
	// Collect data.
	affinityList, err := getCPUAffinity(pid)
	if err != nil {
		log.Fatalf("Error getting CPU affinity: %v", err)
	}

	systemCPUs, systemCPUErr := getSystemCPUCount()

	currentCPU, err := getCurrentCPU(pid)
	if err != nil {
		log.Fatalf("Error getting current CPU: %v", err)
	}

	numaMap, err := buildNUMAMap()
	if err != nil {
		log.Fatalf("Error reading NUMA topology: %v", err)
	}

	cpuNUMANode, ok := numaMap[currentCPU]
	if !ok {
		log.Fatalf("CPU %d not found in NUMA topology", currentCPU)
	}

	var gpus []GPUDevice
	if gpuCheck {
		gpus = discoverGPUs()
	}
	nodes := buildNUMANodes(numaMap, gpus)

	allowedSet := make(map[int]bool, len(affinityList))
	for _, cpu := range affinityList {
		allowedSet[cpu] = true
	}

	uniqueCores := make(map[CoreInfo]bool)
	uniquePackages := make(map[int]bool)
	processNodes := make(map[int]bool)
	for _, cpu := range affinityList {
		info, err := getCPUTopology(cpu)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: cannot read topology for CPU %d: %v\n", cpu, err)
			continue
		}
		uniqueCores[info] = true
		uniquePackages[info.PhysicalID] = true
		if node, ok := numaMap[cpu]; ok {
			processNodes[node] = true
		}
	}

	// Render output.
	fmt.Printf("\n%s\n\n", col(ansiBold, fmt.Sprintf("numa-check — PID %d", pid)))

	// Machine Topology section.
	totalSockets := make(map[int]bool)
	for _, n := range nodes {
		if n.SocketID >= 0 {
			totalSockets[n.SocketID] = true
		}
	}

	printSection("Machine Topology")
	summary := fmt.Sprintf("  %d CPUs, %d NUMA nodes, %d sockets", len(numaMap), len(nodes), len(totalSockets))
	if len(gpus) > 0 {
		summary += fmt.Sprintf(", %d GPUs", len(gpus))
	}
	fmt.Printf("%s\n\n", summary)
	printNodesGrid(nodes, "machine", nil, -1, nil)

	// Process section.
	fmt.Println()
	printSection(fmt.Sprintf("Process — PID %d", pid))

	if systemCPUErr != nil {
		fmt.Fprintf(os.Stderr, "  Warning: could not determine system CPU count: %v\n", systemCPUErr)
	} else {
		pinLabel := col(ansiGreen, "pinned")
		if len(affinityList) >= systemCPUs {
			pinLabel = col(ansiBrightYellow, "not pinned")
		}
		fmt.Printf("  Allowed CPUs ......... %d / %d (%s)\n", len(affinityList), systemCPUs, pinLabel)
	}
	fmt.Printf("  Currently on ......... CPU %d → NUMA Node %d\n", currentCPU, cpuNUMANode)
	fmt.Printf("  Physical cores ....... %d cores, %d sockets\n", len(uniqueCores), len(uniquePackages))

	sortedNodes := sortedKeys(processNodes)
	fmt.Printf("  NUMA span ............ %d node%s %v\n\n", len(sortedNodes), plural(len(sortedNodes)), sortedNodes)

	fmt.Printf("  %s = allowed  %s = current  %s = not allowed\n\n",
		col(ansiGreen, "■"), col(ansiBrightYellow, "★"), col(ansiDim, "□"))
	printNodesGrid(nodes, "process", allowedSet, currentCPU, processNodes)

	// Optional numastat.
	if printNumastat {
		fmt.Println()
		numastatOut, err := exec.Command("numastat", "-p", fmt.Sprintf("%d", pid)).Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error running numastat: %v\n", execStderr(err))
		} else {
			printSection("NUMA Memory Stats")
			fmt.Printf("%s\n", string(numastatOut))
		}
	}

	fmt.Println()
}

// Display helpers.

func printSection(title string) {
	fmt.Printf("  %s\n", col(ansiBold, title))
	fmt.Printf("  %s\n", col(ansiDim, strings.Repeat("─", len(title))))
}

const gridCols = 16 // CPUs per row in the topology grid

// processNodes is only used in "process" mode to color GPUs by NUMA locality.
func printNodesGrid(nodes []NUMANodeInfo, mode string, allowedSet map[int]bool, currentCPU int, processNodes map[int]bool) {
	for i := 0; i < len(nodes); i += 2 {
		left := &nodes[i]
		var right *NUMANodeInfo
		if i+1 < len(nodes) {
			right = &nodes[i+1]
		}
		printNodePair(left, right, mode, allowedSet, currentCPU, processNodes)
		if i+2 < len(nodes) {
			fmt.Println()
		}
	}
}

func printNodePair(left, right *NUMANodeInfo, mode string, allowedSet map[int]bool, currentCPU int, processNodes map[int]bool) {
	const colWidth = 34 // visual width reserved for left column (16 CPUs * 2 - 1 = 31, plus padding)
	const gap = "    "   // gap between side-by-side nodes

	// Headers.
	lh := nodeHeader(left)
	if right != nil {
		fmt.Printf("  %s%s%s\n", col(ansiBold, pad(lh, colWidth)), gap, col(ansiBold, nodeHeader(right)))
	} else {
		fmt.Printf("  %s\n", col(ansiBold, lh))
	}

	// CPU grid rows.
	leftRows := renderGrid(left.CPUs, mode, allowedSet, currentCPU)
	var rightRows []string
	if right != nil {
		rightRows = renderGrid(right.CPUs, mode, allowedSet, currentCPU)
	}

	maxRows := len(leftRows)
	if len(rightRows) > maxRows {
		maxRows = len(rightRows)
	}

	for r := 0; r < maxRows; r++ {
		lRow := ""
		lWidth := 0
		if r < len(leftRows) {
			lRow = leftRows[r]
			lWidth = gridRowVisualWidth(left.CPUs, r)
		}
		if right != nil {
			rRow := ""
			if r < len(rightRows) {
				rRow = rightRows[r]
			}
			fmt.Printf("  %s%s%s%s\n", lRow, strings.Repeat(" ", colWidth-lWidth), gap, rRow)
		} else {
			fmt.Printf("  %s\n", lRow)
		}
	}

	// CPU footer.
	lcf := cpuFooter(left, mode, allowedSet)
	if right != nil {
		rcf := cpuFooter(right, mode, allowedSet)
		fmt.Printf("  %s%s%s\n", col(ansiDim, pad(lcf, colWidth)), gap, col(ansiDim, rcf))
	} else {
		fmt.Printf("  %s\n", col(ansiDim, lcf))
	}

	// GPU rows below, with their own footer.
	if len(left.GPUs) > 0 || (right != nil && len(right.GPUs) > 0) {
		fmt.Println()
		leftGPURows, leftGPUWidths := renderGPURows(left.GPUs, mode, left.ID, processNodes)
		var rightGPURows []string
		if right != nil {
			rightGPURows, _ = renderGPURows(right.GPUs, mode, right.ID, processNodes)
		}

		maxGPURows := len(leftGPURows)
		if len(rightGPURows) > maxGPURows {
			maxGPURows = len(rightGPURows)
		}

		for r := 0; r < maxGPURows; r++ {
			lRow := ""
			lWidth := 0
			if r < len(leftGPURows) {
				lRow = leftGPURows[r]
				lWidth = leftGPUWidths[r]
			}
			if right != nil {
				rRow := ""
				if r < len(rightGPURows) {
					rRow = rightGPURows[r]
				}
				fmt.Printf("  %s%s%s%s\n", lRow, strings.Repeat(" ", colWidth-lWidth), gap, rRow)
			} else if lRow != "" {
				fmt.Printf("  %s\n", lRow)
			}
		}

		// GPU footer.
		lgf := gpuFooter(left)
		if right != nil {
			rgf := gpuFooter(right)
			fmt.Printf("  %s%s%s\n", col(ansiDim, pad(lgf, colWidth)), gap, col(ansiDim, rgf))
		} else if lgf != "" {
			fmt.Printf("  %s\n", col(ansiDim, lgf))
		}
	}
}

func nodeHeader(n *NUMANodeInfo) string {
	h := fmt.Sprintf("NUMA Node %d", n.ID)
	if n.SocketID >= 0 {
		h += fmt.Sprintf(" — Socket %d", n.SocketID)
	}
	return h
}

func cpuFooter(n *NUMANodeInfo, mode string, allowedSet map[int]bool) string {
	if mode == "process" {
		count := 0
		for _, cpu := range n.CPUs {
			if allowedSet[cpu] {
				count++
			}
		}
		return fmt.Sprintf("%d of %d CPUs", count, len(n.CPUs))
	}
	return fmt.Sprintf("%d CPUs (%d–%d)", len(n.CPUs), n.CPUs[0], n.CPUs[len(n.CPUs)-1])
}

func gpuFooter(n *NUMANodeInfo) string {
	if len(n.GPUs) == 0 {
		return ""
	}
	return fmt.Sprintf("%d GPUs", len(n.GPUs))
}

// renderGPURows renders GPU labels in rows of 2, fitting under the CPU grid.
// Returns the rendered strings and their visual widths.
func renderGPURows(gpus []GPUDevice, mode string, nodeID int, processNodes map[int]bool) ([]string, []int) {
	if len(gpus) == 0 {
		return nil, nil
	}
	var rows []string
	var widths []int
	for i := 0; i < len(gpus); i += 2 {
		var sb strings.Builder
		width := 0
		for j := 0; j < 2 && i+j < len(gpus); j++ {
			gpu := gpus[i+j]
			if j > 0 {
				sb.WriteString("    ")
				width += 4
			}
			suffix := ""
			if gpu.NUMANode < 0 {
				suffix = " ?"
			}
			block := fmt.Sprintf("▀▀ GPU %d%s", i+j, suffix)
			switch mode {
			case "machine":
				sb.WriteString(col(ansiGreen, block))
			case "process":
				if processNodes[nodeID] {
					sb.WriteString(col(ansiGreen, block))
				} else {
					sb.WriteString(col(ansiRed, block))
				}
			}
			width += utf8.RuneCountInString(block)
		}
		rows = append(rows, sb.String())
		widths = append(widths, width)
	}
	return rows, widths
}

func renderGrid(cpus []int, mode string, allowedSet map[int]bool, currentCPU int) []string {
	var rows []string
	for i := 0; i < len(cpus); i += gridCols {
		end := i + gridCols
		if end > len(cpus) {
			end = len(cpus)
		}
		var sb strings.Builder
		for j, cpu := range cpus[i:end] {
			if j > 0 {
				sb.WriteString(" ")
			}
			switch mode {
			case "machine":
				sb.WriteString(col(ansiCyan, "■"))
			case "process":
				if cpu == currentCPU {
					sb.WriteString(col(ansiBrightYellow, "★"))
				} else if allowedSet[cpu] {
					sb.WriteString(col(ansiGreen, "■"))
				} else {
					sb.WriteString(col(ansiDim, "□"))
				}
			}
		}
		rows = append(rows, sb.String())
	}
	return rows
}

// gridRowVisualWidth returns the visual width of a grid row (chars + spaces).
func gridRowVisualWidth(cpus []int, row int) int {
	start := row * gridCols
	n := len(cpus) - start
	if n > gridCols {
		n = gridCols
	}
	if n <= 0 {
		return 0
	}
	return n*2 - 1 // each char + space, minus trailing space
}

func pad(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func sortedKeys(m map[int]bool) []int {
	var keys []int
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

func buildNUMANodes(numaMap map[int]int, gpus []GPUDevice) []NUMANodeInfo {
	nodeCPUs := make(map[int][]int)
	for cpu, node := range numaMap {
		nodeCPUs[node] = append(nodeCPUs[node], cpu)
	}
	// Group GPUs by NUMA node. GPUs with unknown NUMA (-1) go to all nodes.
	nodeGPUs := make(map[int][]GPUDevice)
	for _, g := range gpus {
		if _, exists := nodeCPUs[g.NUMANode]; exists {
			nodeGPUs[g.NUMANode] = append(nodeGPUs[g.NUMANode], g)
		} else {
			// GPU has unknown NUMA affinity (-1) or doesn't match a node.
			// Attach to all nodes so it's still visible.
			for nodeID := range nodeCPUs {
				nodeGPUs[nodeID] = append(nodeGPUs[nodeID], g)
			}
		}
	}
	var nodes []NUMANodeInfo
	for id, cpus := range nodeCPUs {
		sort.Ints(cpus)
		socketID := -1
		if len(cpus) > 0 {
			if info, err := getCPUTopology(cpus[0]); err == nil {
				socketID = info.PhysicalID
			}
		}
		nodes = append(nodes, NUMANodeInfo{ID: id, SocketID: socketID, CPUs: cpus, GPUs: nodeGPUs[id]})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return nodes
}

func discoverGPUs() []GPUDevice {
	gpuMap, err := getGPUInfo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not query GPUs: %v\n", err)
		return nil
	}
	if len(gpuMap) == 0 {
		fmt.Fprintf(os.Stderr, "Warning: nvidia-smi returned no GPUs\n")
		return nil
	}
	var gpus []GPUDevice
	for uuid, pciID := range gpuMap {
		node, err := readIntFile(filepath.Join("/sys/bus/pci/devices", pciID, "numa_node"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: cannot read NUMA node for GPU %s (PCI: %s): %v\n", uuid, pciID, err)
			node = -1
		}
		if node == -1 {
			fmt.Fprintf(os.Stderr, "Warning: GPU %s reports NUMA node -1 (affinity unknown)\n", pciID)
		}
		gpus = append(gpus, GPUDevice{UUID: uuid, PCIID: pciID, NUMANode: node})
	}
	sort.Slice(gpus, func(i, j int) bool {
		if gpus[i].NUMANode != gpus[j].NUMANode {
			return gpus[i].NUMANode < gpus[j].NUMANode
		}
		return gpus[i].PCIID < gpus[j].PCIID
	})
	return gpus
}

// Data collection functions.

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

func getSystemCPUCount() (int, error) {
	data, err := os.ReadFile("/sys/devices/system/cpu/possible")
	if err != nil {
		return 0, err
	}
	cpus, err := expandCPUList(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, err
	}
	return len(cpus), nil
}

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
			fmt.Fprintf(os.Stderr, "Warning: skipping %s: cannot parse node ID: %v\n", nodeName, err)
			continue
		}
		cpulistData, err := os.ReadFile(filepath.Join(nodePath, "cpulist"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skipping NUMA node %d: cannot read cpulist: %v\n", nodeID, err)
			continue
		}
		cpus, err := expandCPUList(strings.TrimSpace(string(cpulistData)))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skipping NUMA node %d: cannot parse cpulist: %v\n", nodeID, err)
			continue
		}
		for _, cpu := range cpus {
			cpuToNode[cpu] = nodeID
		}
	}
	if len(cpuToNode) == 0 {
		return nil, fmt.Errorf("NUMA nodes found in sysfs but none could be parsed")
	}
	return cpuToNode, nil
}

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

func readIntFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func execStderr(err error) error {
	if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
	}
	return err
}

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

// GPU discovery.

func getGPUInfo() (map[string]string, error) {
	out, err := exec.Command("nvidia-smi", "--query-gpu=uuid,pci.bus_id", "--format=csv,noheader").Output()
	if err != nil {
		return nil, execStderr(err)
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

// Container PID lookup via crictl.

func getPIDFromContainer(podName, containerName string) (int, error) {
	out, err := exec.Command("crictl", "ps", "-o", "json").Output()
	if err != nil {
		return 0, fmt.Errorf("failed to run crictl ps: %v", execStderr(err))
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
		return 0, fmt.Errorf("crictl inspect failed: %v", execStderr(err))
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
