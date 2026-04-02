package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
)

func readCPUManagerState(fs FileSystem, path string) (*CPUManagerState, error) {
	data, err := fs.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %v", path, err)
	}
	var state CPUManagerState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing %s: %v", path, err)
	}
	return &state, nil
}

func parseCPUManagerEntries(state *CPUManagerState) []CPUManagerEntry {
	var entries []CPUManagerEntry
	for podUID, containers := range state.Entries {
		for containerName, cpuSet := range containers {
			cpus, err := expandCPUList(cpuSet)
			if err != nil {
				slog.Debug("skipping cpu_manager entry: invalid CPU set", "pod", podUID, "container", containerName, "cpuset", cpuSet, "error", err)
				continue
			}
			entries = append(entries, CPUManagerEntry{
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

func toJSONCPUManager(state *CPUManagerState, entries []CPUManagerEntry) *jsonCPUManager {
	jcm := &jsonCPUManager{
		PolicyName: state.PolicyName,
	}
	if state.DefaultCPUSet != "" {
		if cpus, err := expandCPUList(state.DefaultCPUSet); err == nil {
			jcm.DefaultCPUs = cpus
		}
	}
	for _, e := range entries {
		jcm.Entries = append(jcm.Entries, jsonCPUManagerEntry{
			PodUID:        e.PodUID,
			ContainerName: e.ContainerName,
			CPUs:          e.CPUs,
		})
	}
	return jcm
}
