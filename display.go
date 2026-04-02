package main

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

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

var useColor bool

func init() {
	if os.Getenv("NO_COLOR") != "" {
		useColor = false
		return
	}
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

func pad(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

func printSection(title string) {
	fmt.Printf("  %s\n", col(ansiBold, title))
	fmt.Printf("  %s\n", col(ansiDim, strings.Repeat("─", utf8.RuneCountInString(title))))
}

const gridCols = 16 // CPUs per row in the topology grid

func printNodesGrid(nodes []NUMANodeInfo, mode DisplayMode, allowedSet map[int]bool, currentCPU int, processNodes map[int]bool, allowedGPUs map[string]bool) {
	for i := 0; i < len(nodes); i += 2 {
		left := &nodes[i]
		var right *NUMANodeInfo
		if i+1 < len(nodes) {
			right = &nodes[i+1]
		}
		printNodePair(left, right, mode, allowedSet, currentCPU, processNodes, allowedGPUs)
		if i+2 < len(nodes) {
			fmt.Println()
		}
	}
}

func printNodePair(left, right *NUMANodeInfo, mode DisplayMode, allowedSet map[int]bool, currentCPU int, processNodes map[int]bool, allowedGPUs map[string]bool) {
	const colWidth = 34
	const gap = "    "

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

	// GPU rows.
	if len(left.GPUs) > 0 || (right != nil && len(right.GPUs) > 0) {
		fmt.Println()
		leftGPURows, leftGPUWidths := renderGPURows(left.GPUs, mode, left.ID, processNodes, allowedGPUs)
		var rightGPURows []string
		if right != nil {
			rightGPURows, _ = renderGPURows(right.GPUs, mode, right.ID, processNodes, allowedGPUs)
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

func cpuFooter(n *NUMANodeInfo, mode DisplayMode, allowedSet map[int]bool) string {
	if len(n.CPUs) == 0 {
		return "0 CPUs"
	}
	if mode == ModeProcess {
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

func renderGPURows(gpus []GPUDevice, mode DisplayMode, nodeID int, processNodes map[int]bool, allowedGPUs map[string]bool) ([]string, []int) {
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
			block := fmt.Sprintf("▀▀ GPU %d%s", gpu.Index, suffix)
			switch mode {
			case ModeMachine:
				sb.WriteString(col(ansiGreen, block))
			case ModeProcess:
				if allowedGPUs == nil || allowedGPUs[gpu.UUID] {
					if processNodes[nodeID] {
						sb.WriteString(col(ansiGreen, block))
					} else {
						sb.WriteString(col(ansiRed, block))
					}
				} else {
					sb.WriteString(col(ansiDim, block))
				}
			}
			width += utf8.RuneCountInString(block)
		}
		rows = append(rows, sb.String())
		widths = append(widths, width)
	}
	return rows, widths
}

func renderGrid(cpus []int, mode DisplayMode, allowedSet map[int]bool, currentCPU int) []string {
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
			case ModeMachine:
				sb.WriteString(col(ansiCyan, "■"))
			case ModeProcess:
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

func printCPUManagerSection(state *CPUManagerState, entries []CPUManagerEntry, nodes []NUMANodeInfo) {
	printSection("CPU Manager")
	fmt.Printf("  Policy .............. %s\n", state.PolicyName)
	if state.DefaultCPUSet != "" {
		if cpus, err := expandCPUList(state.DefaultCPUSet); err == nil {
			fmt.Printf("  Default CPUs ........ %s (%d CPUs available for shared containers)\n", state.DefaultCPUSet, len(cpus))
		} else {
			fmt.Printf("  Default CPUs ........ %s\n", state.DefaultCPUSet)
		}
	} else {
		fmt.Printf("  Default CPUs ........ none (all CPUs exclusively assigned)\n")
	}

	if len(entries) == 0 {
		fmt.Printf("\n  No containers with exclusive CPU assignments\n")
	} else {
		fmt.Printf("\n  Exclusively assigned:\n")
		for _, e := range entries {
			uid := e.PodUID
			if len(uid) > 12 {
				uid = uid[:12]
			}
			fmt.Printf("    %s / %s %s %s (%d CPUs)\n",
				uid, e.ContainerName,
				col(ansiDim, ".."),
				e.CPUSetRaw, len(e.CPUs))
		}
	}

	stats := perNUMANodeStats(entries, nodes)
	if len(stats) == 0 {
		return
	}

	fmt.Printf("\n  Per NUMA node:\n")
	for _, s := range stats {
		fmt.Printf("    Node %d:  %d exclusive / %d remaining  (of %d)\n",
			s.NodeID, s.ExclusiveCPUs, s.RemainingCPUs, s.TotalCPUs)
	}
}

func gridRowVisualWidth(cpus []int, row int) int {
	start := row * gridCols
	n := len(cpus) - start
	if n > gridCols {
		n = gridCols
	}
	if n <= 0 {
		return 0
	}
	return n*2 - 1
}
