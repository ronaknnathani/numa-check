package main

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
)

func buildNUMANodes(fs FileSystem, numaMap map[int]int, gpus []GPUDevice) []NUMANodeInfo {
	nodeCPUs := make(map[int][]int)
	for cpu, node := range numaMap {
		nodeCPUs[node] = append(nodeCPUs[node], cpu)
	}
	nodeGPUs := make(map[int][]GPUDevice)
	for _, g := range gpus {
		if _, exists := nodeCPUs[g.NUMANode]; exists {
			nodeGPUs[g.NUMANode] = append(nodeGPUs[g.NUMANode], g)
		} else {
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
			if info, err := getCPUTopology(fs, cpus[0]); err == nil {
				socketID = info.PhysicalID
			}
		}
		nodes = append(nodes, NUMANodeInfo{ID: id, SocketID: socketID, CPUs: cpus, GPUs: nodeGPUs[id]})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return nodes
}

const nvidiaVendorID = "0x10de"

// detectNVIDIAGPUsPCI checks sysfs for NVIDIA PCI devices (VGA or 3D controller class).
// Returns PCI device paths (e.g., "0000:3b:00.0") for NVIDIA GPUs found.
func detectNVIDIAGPUsPCI(fs FileSystem) []string {
	matches, err := fs.Glob("/sys/bus/pci/devices/*")
	if err != nil {
		slog.Debug("failed to glob PCI devices", "error", err)
		return nil
	}

	var gpuPCIIDs []string
	for _, devPath := range matches {
		vendorData, err := fs.ReadFile(filepath.Join(devPath, "vendor"))
		if err != nil {
			continue
		}
		vendor := strings.TrimSpace(string(vendorData))
		if vendor != nvidiaVendorID {
			continue
		}
		classData, err := fs.ReadFile(filepath.Join(devPath, "class"))
		if err != nil {
			continue
		}
		class := strings.TrimSpace(string(classData))
		// Prefix match: drivers may report extended class codes beyond the base 6 digits.
		if strings.HasPrefix(class, "0x0300") || strings.HasPrefix(class, "0x0302") {
			pciID := filepath.Base(devPath)
			slog.Debug("found NVIDIA GPU via PCI", "pci", pciID, "class", class)
			gpuPCIIDs = append(gpuPCIIDs, pciID)
		}
	}
	return gpuPCIIDs
}

// discoverGPUs performs two-phase GPU detection:
// Phase 1: Check sysfs PCI for NVIDIA devices (no external commands needed).
// Phase 2: If GPUs found, call nvidia-smi for UUID mapping. Falls back to PCI-only if nvidia-smi unavailable.
func discoverGPUs(fs FileSystem, cmd CommandRunner) ([]GPUDevice, error) {
	// Phase 1: PCI detection.
	pciIDs := detectNVIDIAGPUsPCI(fs)
	if len(pciIDs) == 0 {
		slog.Debug("no NVIDIA GPUs detected via PCI")
		return nil, nil
	}
	slog.Debug("detected NVIDIA GPUs via PCI", "count", len(pciIDs))

	// Phase 2: Try nvidia-smi for UUID mapping.
	gpuMap, err := getGPUInfo(cmd)
	if err != nil {
		slog.Debug("nvidia-smi unavailable, using PCI-only GPU info", "error", err)
		// Build GPU list from PCI info only (no UUIDs).
		return buildGPUsFromPCI(fs, pciIDs), fmt.Errorf("nvidia-smi unavailable: %w", err)
	}
	if len(gpuMap) == 0 {
		slog.Debug("nvidia-smi returned no GPUs, using PCI-only info")
		return buildGPUsFromPCI(fs, pciIDs), nil
	}

	var gpus []GPUDevice
	for uuid, pciID := range gpuMap {
		gpus = append(gpus, GPUDevice{UUID: uuid, PCIID: pciID, NUMANode: readGPUNUMANode(fs, pciID)})
	}
	sortAndIndexGPUs(gpus)
	return gpus, nil
}

func buildGPUsFromPCI(fs FileSystem, pciIDs []string) []GPUDevice {
	var gpus []GPUDevice
	for _, pciID := range pciIDs {
		gpus = append(gpus, GPUDevice{PCIID: pciID, NUMANode: readGPUNUMANode(fs, pciID)})
	}
	sortAndIndexGPUs(gpus)
	return gpus
}

func readGPUNUMANode(fs FileSystem, pciID string) int {
	node, err := readIntFile(fs, filepath.Join("/sys/bus/pci/devices", pciID, "numa_node"))
	if err != nil {
		slog.Debug("cannot read NUMA node for GPU", "pci", pciID, "error", err)
		return -1
	}
	if node == -1 {
		slog.Debug("GPU reports unknown NUMA affinity", "pci", pciID)
	}
	return node
}

func sortAndIndexGPUs(gpus []GPUDevice) {
	sort.Slice(gpus, func(i, j int) bool {
		if gpus[i].NUMANode != gpus[j].NUMANode {
			return gpus[i].NUMANode < gpus[j].NUMANode
		}
		return gpus[i].PCIID < gpus[j].PCIID
	})
	for i := range gpus {
		gpus[i].Index = i
	}
}
