# AGENTS.md

This file provides guidance to coding agents when working with code in this repository.

## What This Is

A Linux-only CLI tool that analyzes NUMA topology for a given process. Reports CPU affinity, cpuset pinning, physical core/package mapping, NUMA node distribution, and GPU-to-NUMA-node locality. Designed to run on Kubernetes nodes to diagnose container CPU/GPU placement.

## Build

```bash
GOOS=linux go build -o numacheck .
# or on a Linux host:
go build -o numacheck .
```

Must cross-compile with `GOOS=linux` on macOS since the code uses `unix.SchedGetaffinity` (Linux-only syscall). Vet/build checks:

```bash
GOOS=linux GOARCH=amd64 go vet ./...
GOOS=linux GOARCH=amd64 go build -o /dev/null .
```

Tests run natively on macOS (Linux-only code is behind build tags):

```bash
go test ./...
go test -cover ./...
```

## Usage

```bash
numacheck -topo                                    # machine topology only (no PID)
numacheck -pid <PID>                               # process NUMA analysis
numacheck -pod <pod-name> -container <container-name>
numacheck -pid <PID> -debug                        # with debug logging
numacheck -topo -cpumanager /var/lib/kubelet/cpu_manager_state  # CPU manager assignments
numacheck -pid <PID> -cpumanager /var/lib/kubelet/cpu_manager_state
```

## Architecture

Single package (`package main`), split across files by concern:

| File | Responsibility |
|---|---|
| `main.go` | CLI entry, flag parsing, orchestration |
| `types.go` | Types, interfaces (`FileSystem`, `CommandRunner`) |
| `sysfs.go` | Sysfs/procfs readers (NUMA map, CPU topology, process info) |
| `sysfs_linux.go` | Linux-only: `getCPUAffinity` via `unix.SchedGetaffinity` |
| `sysfs_stub.go` | Non-Linux stub for `getCPUAffinity` |
| `cpumanager.go` | Kubelet CPU manager state file reader |
| `commands.go` | External command wrappers (nvidia-smi, crictl) |
| `topology.go` | Data assembly, GPU discovery (2-phase: PCI + nvidia-smi) |
| `display.go` | Grid rendering, ANSI colors, formatting |
| `parse.go` | CPU list parsing, PCI ID normalization |

### Testability

Functions accept `FileSystem` and `CommandRunner` interfaces instead of calling `os.ReadFile`/`exec.Command` directly. Tests provide mock implementations (`mockFS`, `mockCmd`).

### Data sources

| Data | Source |
|---|---|
| CPU affinity | `unix.SchedGetaffinity()` syscall |
| Current CPU | `/proc/<pid>/stat` field 39 |
| NUMA node mapping | `/sys/devices/system/node/node*/cpulist` |
| Physical core/socket | `/sys/devices/system/cpu/cpu<N>/topology/{physical_package_id,core_id}` |
| System CPU count | `/sys/devices/system/cpu/possible` |
| GPU PCI detection | `/sys/bus/pci/devices/*/vendor` + `/sys/bus/pci/devices/*/class` |
| GPU UUID/PCI mapping | `nvidia-smi` (only when PCI detection finds NVIDIA devices) |
| GPU NUMA node | `/sys/bus/pci/devices/<pciID>/numa_node` |
| Container PID | `crictl` (only with `-pod`/`-container` flags) |
| CPU manager state | `{kubelet-root}/cpu_manager_state` (only with `-cpumanager` flag) |

### Output modes

- **`-topo`** — machine topology: CPU grid per NUMA node + GPU placement
- **`-pid`/`-pod`** — process analysis: CPU placement overlay

Output uses ANSI colors when stdout is a TTY. Respects `NO_COLOR` env var.

### Key helpers

- `readIntFile(fs, path)` — reads a single-integer sysfs file
- `expandCPUList(s)` — parses `0-3,8-11` format from sysfs cpulist files
- `buildNUMAMap(fs)` — builds cpu→NUMA node mapping from sysfs
- `buildNUMANodes(fs, numaMap, gpus)` — groups CPUs/GPUs by NUMA node
- `getCPUTopology(fs, cpu)` — reads physical_package_id and core_id
- `detectNVIDIAGPUsPCI(fs)` — scans PCI bus for NVIDIA GPUs
- `discoverGPUs(fs, cmd)` — two-phase GPU detection (PCI + nvidia-smi)
- `printNodesGrid(...)` — renders NUMA node CPU grids side-by-side
- `readCPUManagerState(fs, path)` — reads and parses kubelet cpu_manager_state JSON
- `parseCPUManagerEntries(state)` — converts raw state entries into sorted parsed entries

## gh CLI authentication

This repo lives under the personal `ronaknnathani` GitHub account. Before running any `gh` command in this repo, switch to that account:

```bash
gh auth switch -u ronaknnathani
```

`git push` is unaffected — it uses SSH keys, not the `gh` token.

## Commit author email

Commits in this repo must use `7279934+ronaknnathani@users.noreply.github.com` (set in this repo's local git config).
