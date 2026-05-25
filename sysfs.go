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

// readNodeMemInfo reads per-NUMA-node memory from sysfs.
// Format: lines like "Node 0 MemTotal:      131072000 kB".
// Returns total and free in bytes. Returns 0 for fields not present in the file.
// If a field's value is malformed, it is logged via slog.Debug and treated as 0;
// successfully parsed fields are still returned so the caller can render partial data.
func readNodeMemInfo(fs FileSystem, nodeID int) (total, free int64, err error) {
	path := fmt.Sprintf("/sys/devices/system/node/node%d/meminfo", nodeID)
	data, err := fs.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		// Expect: "Node", "<id>", "<key>:", "<value>", "kB"
		if len(fields) < 4 || fields[0] != "Node" {
			continue
		}
		key := strings.TrimSuffix(fields[2], ":")
		switch key {
		case "MemTotal":
			kb, perr := strconv.ParseInt(fields[3], 10, 64)
			if perr != nil {
				slog.Debug("parsing MemTotal", "node", nodeID, "value", fields[3], "err", perr)
				continue
			}
			total = kb * 1024
		case "MemFree":
			kb, perr := strconv.ParseInt(fields[3], 10, 64)
			if perr != nil {
				slog.Debug("parsing MemFree", "node", nodeID, "value", fields[3], "err", perr)
				continue
			}
			free = kb * 1024
		}
	}
	return total, free, nil
}

// readProcessNUMAMemory aggregates per-NUMA-node memory for a process by parsing
// /proc/<pid>/numa_maps. Returns a map of NUMA node ID → bytes resident on that node.
// Lines without kernelpagesize_kB are skipped (cannot determine byte count).
func readProcessNUMAMemory(fs FileSystem, pid int) (map[int]int64, error) {
	data, err := fs.ReadFile(fmt.Sprintf("/proc/%d/numa_maps", pid))
	if err != nil {
		return nil, err
	}
	result := make(map[int]int64)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)

		// First find kernelpagesize_kB; without it we can't convert pages to bytes.
		var pageSizeKB int64
		for _, f := range fields {
			if val, ok := strings.CutPrefix(f, "kernelpagesize_kB="); ok {
				pageSizeKB, _ = strconv.ParseInt(val, 10, 64)
				break
			}
		}
		if pageSizeKB == 0 {
			continue
		}

		// Then accumulate per-node page counts from "N<id>=<pages>" entries.
		for _, f := range fields {
			if len(f) < 3 || f[0] != 'N' {
				continue
			}
			eq := strings.IndexByte(f, '=')
			if eq <= 1 {
				continue
			}
			nodeID, err1 := strconv.Atoi(f[1:eq])
			pages, err2 := strconv.ParseInt(f[eq+1:], 10, 64)
			if err1 != nil || err2 != nil {
				continue
			}
			result[nodeID] += pages * pageSizeKB * 1024
		}
	}
	return result, nil
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
