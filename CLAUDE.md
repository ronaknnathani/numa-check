# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

A Linux-only CLI tool that analyzes NUMA topology for a given process. Reports CPU affinity, cpuset pinning, physical core/package mapping, NUMA node distribution, and optionally GPU-to-NUMA-node locality. Designed to run on Kubernetes nodes to diagnose container CPU/GPU placement.

## Build

```bash
GOOS=linux go build -o numa-check .
# or on a Linux host:
go build -o numa-check .
```

Must cross-compile with `GOOS=linux` on macOS since the code uses `unix.SchedGetaffinity` (Linux-only syscall). Vet/build checks:

```bash
GOOS=linux GOARCH=amd64 go vet ./...
GOOS=linux GOARCH=amd64 go build -o /dev/null .
```

## Usage

```bash
numa-check -topo                                    # machine topology only (no PID)
numa-check -pid <PID>                               # process NUMA analysis
numa-check -pod <pod-name> -container <container-name>
numa-check -pid <PID> -gpu                          # GPU NUMA analysis
numa-check -pid <PID> -numastat                     # numastat memory stats
```

## Architecture

Single-file tool (`main.go`). All logic in `package main`. Only external dependency: `golang.org/x/sys/unix` for `SchedGetaffinity`.

### Data sources

The tool reads sysfs/procfs directly instead of shelling out to CLI tools:

| Data | Source |
|---|---|
| CPU affinity | `unix.SchedGetaffinity()` syscall (reflects cgroup cpuset restrictions) |
| Current CPU | `/proc/<pid>/stat` field 39 |
| NUMA node mapping | `/sys/devices/system/node/node*/cpulist` |
| Physical core/socket | `/sys/devices/system/cpu/cpu<N>/topology/{physical_package_id,core_id}` |
| System CPU count | `/sys/devices/system/cpu/possible` |
| GPU PCI bus IDs | `nvidia-smi` (only with `-gpu` flag) |
| GPU NUMA node | `/sys/bus/pci/devices/<pciID>/numa_node` |
| Container PID | `crictl` (only with `-pod`/`-container` flags) |
| NUMA memory stats | `numastat` (only with `-numastat` flag) |

### Output modes

- **`-topo`** — machine topology only: CPU grid per NUMA node + GPU NUMA placement (no PID needed)
- **`-pid`/`-pod`** — full analysis: machine topology grid, then process CPU placement overlay on same grid, then optional GPU/numastat

Output uses ANSI colors when stdout is a TTY, plain text otherwise.

### Key helpers

- `readIntFile(path)` — reads a single-integer sysfs file
- `expandCPUList(s)` — parses `0-3,8-11` format from sysfs cpulist files
- `buildNUMAMap()` — builds cpu→NUMA node mapping from sysfs
- `buildNUMANodes(numaMap)` — groups CPUs by NUMA node with socket IDs for display
- `getCPUTopology(cpu)` — reads physical_package_id and core_id for a CPU
- `printNodesGrid(...)` — renders NUMA node CPU grids side-by-side (16 CPUs per row)
