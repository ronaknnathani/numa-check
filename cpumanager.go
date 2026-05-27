package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"

	"numacheck/apiv1"
)

func readCPUManagerState(fs FileSystem, path string) (*KubeletCPUManagerState, error) {
	data, err := fs.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %v", path, err)
	}
	var state KubeletCPUManagerState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing %s: %v", path, err)
	}
	return &state, nil
}

func parseCPUManagerEntries(state *KubeletCPUManagerState) []KubeletCPUManagerEntry {
	var entries []KubeletCPUManagerEntry
	for podUID, containers := range state.Entries {
		for containerName, cpuSet := range containers {
			cpus, err := expandCPUList(cpuSet)
			if err != nil {
				slog.Debug("skipping cpu_manager entry: invalid CPU set", "pod", podUID, "container", containerName, "cpuset", cpuSet, "error", err)
				continue
			}
			entries = append(entries, KubeletCPUManagerEntry{
				PodUID:        podUID,
				ContainerName: containerName,
				CPUs:          cpus,
				CPUSetRaw:     cpuSet,
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].PodUID != entries[j].PodUID {
			return entries[i].PodUID < entries[j].PodUID
		}
		return entries[i].ContainerName < entries[j].ContainerName
	})
	return entries
}

func toAPICPUManager(state *KubeletCPUManagerState, entries []KubeletCPUManagerEntry, nodes []NUMANodeInfo) *apiv1.CPUManager {
	jcm := &apiv1.CPUManager{
		PolicyName: state.PolicyName,
	}
	if state.DefaultCPUSet != "" {
		if cpus, err := expandCPUList(state.DefaultCPUSet); err == nil {
			jcm.DefaultCPUs = cpus
		}
	}
	for _, e := range entries {
		jcm.Entries = append(jcm.Entries, apiv1.CPUManagerEntry{
			PodUID:        e.PodUID,
			ContainerName: e.ContainerName,
			CPUs:          e.CPUs,
		})
	}

	jcm.PerNUMANode = perNUMANodeStats(entries, nodes)

	return jcm
}

// perNUMANodeStats computes per-NUMA-node exclusive/remaining CPU counts.
func perNUMANodeStats(entries []KubeletCPUManagerEntry, nodes []NUMANodeInfo) []apiv1.CPUManagerNUMANode {
	if len(nodes) == 0 {
		return nil
	}

	exclusiveSet := make(map[int]bool)
	for _, e := range entries {
		for _, cpu := range e.CPUs {
			exclusiveSet[cpu] = true
		}
	}

	stats := make([]apiv1.CPUManagerNUMANode, len(nodes))
	for i, n := range nodes {
		exclusive := 0
		for _, cpu := range n.CPUs {
			if exclusiveSet[cpu] {
				exclusive++
			}
		}
		stats[i] = apiv1.CPUManagerNUMANode{
			NodeID:        n.ID,
			ExclusiveCPUs: exclusive,
			RemainingCPUs: len(n.CPUs) - exclusive,
			TotalCPUs:     len(n.CPUs),
		}
	}
	return stats
}
