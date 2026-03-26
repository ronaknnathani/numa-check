package main

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
)

func readIntFile(fs FileSystem, path string) (int, error) {
	data, err := fs.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func buildNUMAMap(fs FileSystem) (map[int]int, error) {
	matches, err := fs.Glob("/sys/devices/system/node/node[0-9]*")
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
			slog.Debug("skipping NUMA node: cannot parse ID", "path", nodeName, "error", err)
			continue
		}
		cpulistData, err := fs.ReadFile(filepath.Join(nodePath, "cpulist"))
		if err != nil {
			slog.Debug("skipping NUMA node: cannot read cpulist", "node", nodeID, "error", err)
			continue
		}
		cpus, err := expandCPUList(strings.TrimSpace(string(cpulistData)))
		if err != nil {
			slog.Debug("skipping NUMA node: cannot parse cpulist", "node", nodeID, "error", err)
			continue
		}
		slog.Debug("parsed NUMA node", "node", nodeID, "cpus", len(cpus))
		for _, cpu := range cpus {
			cpuToNode[cpu] = nodeID
		}
	}
	if len(cpuToNode) == 0 {
		return nil, fmt.Errorf("NUMA nodes found in sysfs but none could be parsed")
	}
	return cpuToNode, nil
}

func getCPUTopology(fs FileSystem, cpu int) (CoreInfo, error) {
	base := fmt.Sprintf("/sys/devices/system/cpu/cpu%d/topology", cpu)
	physID, err := readIntFile(fs, filepath.Join(base, "physical_package_id"))
	if err != nil {
		return CoreInfo{}, err
	}
	coreID, err := readIntFile(fs, filepath.Join(base, "core_id"))
	if err != nil {
		return CoreInfo{}, err
	}
	return CoreInfo{PhysicalID: physID, CoreID: coreID}, nil
}

func getSystemCPUCount(fs FileSystem) (int, error) {
	data, err := fs.ReadFile("/sys/devices/system/cpu/possible")
	if err != nil {
		return 0, err
	}
	cpus, err := expandCPUList(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, err
	}
	return len(cpus), nil
}

func getCurrentCPU(fs FileSystem, pid int) (int, error) {
	data, err := fs.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, err
	}
	s := string(data)
	idx := strings.LastIndex(s, ")")
	if idx < 0 {
		return 0, fmt.Errorf("unexpected format in /proc/%d/stat", pid)
	}
	fields := strings.Fields(s[idx+1:])
	if len(fields) < 37 {
		return 0, fmt.Errorf("not enough fields in /proc/%d/stat", pid)
	}
	return strconv.Atoi(fields[36])
}

func getParentPID(fs FileSystem, pid int) (int, error) {
	data, err := fs.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if val, ok := strings.CutPrefix(line, "PPid:"); ok {
			return strconv.Atoi(strings.TrimSpace(val))
		}
	}
	return 0, fmt.Errorf("PPid not found in /proc/%d/status", pid)
}

// getAllowedGPUs walks up the process tree to find NVIDIA_VISIBLE_DEVICES.
// Returns UUIDs of allowed GPUs, or nil if all GPUs are visible.
// When the env var contains numeric indices, they are resolved to UUIDs using the provided GPU list.
func getAllowedGPUs(fs FileSystem, pid int, gpus []GPUDevice) ([]string, error) {
	for p := pid; p > 1; {
		data, err := fs.ReadFile(fmt.Sprintf("/proc/%d/environ", p))
		if err != nil {
			if p == pid {
				return nil, err
			}
			return nil, nil
		}
		for _, env := range strings.Split(string(data), "\x00") {
			if val, ok := strings.CutPrefix(env, "NVIDIA_VISIBLE_DEVICES="); ok {
				val = strings.TrimSpace(val)
				if val == "" || val == "none" || val == "void" {
					return nil, nil
				}
				return resolveGPUIDs(strings.Split(val, ","), gpus), nil
			}
		}
		p, err = getParentPID(fs, p)
		if err != nil {
			return nil, nil
		}
	}
	return nil, nil
}

// resolveGPUIDs converts NVIDIA_VISIBLE_DEVICES values to UUIDs.
// The values can be either UUIDs (GPU-xxxx) or numeric indices (0,1,2).
func resolveGPUIDs(ids []string, gpus []GPUDevice) []string {
	if len(ids) == 0 {
		return nil
	}
	// Check if the first value looks like a numeric index.
	if _, err := strconv.Atoi(strings.TrimSpace(ids[0])); err == nil {
		// Numeric indices — resolve to UUIDs.
		var uuids []string
		for _, idStr := range ids {
			idx, err := strconv.Atoi(strings.TrimSpace(idStr))
			if err != nil {
				continue
			}
			if idx >= 0 && idx < len(gpus) {
				uuids = append(uuids, gpus[idx].UUID)
			}
		}
		return uuids
	}
	// Already UUIDs — return as-is.
	return ids
}
