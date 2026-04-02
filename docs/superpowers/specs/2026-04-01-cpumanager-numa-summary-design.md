# CPU Manager: Per-NUMA-Node Allocation Summary

## Goal

Show how many cores are exclusively allocated per NUMA node and how many remain, in both text and JSON output modes.

## Current State

The `-cpumanager` flag reads `/var/lib/kubelet/cpu_manager_state` and displays per-container exclusive CPU assignments. It does not break down allocations by NUMA node.

## Design

Add a "Per NUMA node" summary after the existing container list in `printCPUManagerSection`. For each NUMA node, cross-reference exclusively assigned CPUs against the node's CPU list to compute exclusive count and remaining count.

### Text Output

Appended after the "Exclusively assigned" container list:

```
  Per NUMA node:
    Node 0:  4 exclusive / 8 remaining  (of 12)
    Node 1:  4 exclusive / 12 remaining (of 16)
```

### JSON Output

New `per_numa_node` array on `jsonCPUManager`:

```json
{
  "cpu_manager": {
    "policy_name": "static",
    "default_cpus": [0, 1, 2, 3],
    "entries": [...],
    "per_numa_node": [
      {"node_id": 0, "exclusive_cpus": 4, "remaining_cpus": 8, "total_cpus": 12},
      {"node_id": 1, "exclusive_cpus": 4, "remaining_cpus": 12, "total_cpus": 16}
    ]
  }
}
```

### Files Changed

- `display.go` — `printCPUManagerSection` gains `[]NUMANodeInfo` param, appends NUMA summary
- `types.go` — new `jsonCPUManagerNUMANode` struct, new `PerNUMANode` field on `jsonCPUManager`
- `cpumanager.go` — `toJSONCPUManager` gains `[]NUMANodeInfo` param, populates `PerNUMANode`
- `main.go` — pass `nodes` to both `printCPUManagerSection` and `toJSONCPUManager` at both call sites
- `cpumanager_test.go` — test per-NUMA aggregation, update `toJSONCPUManager` test
