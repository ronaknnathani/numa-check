# numa-check

A quick diagnostic tool that tells you whether a Linux process (or Kubernetes container) has its CPUs and GPUs properly placed on the same NUMA node.

## Why

On multi-socket servers, memory access is non-uniform — a CPU accessing memory attached to a remote NUMA node pays a latency penalty. In Kubernetes, containers can silently end up with CPUs scattered across NUMA nodes, or with GPUs on a different node than their CPUs. This kills performance for latency-sensitive and GPU workloads, and it's invisible without checking.

`numa-check` answers these questions in one command:
- Is my process pinned to specific CPUs, or floating across the whole machine?
- Are those CPUs all on the same NUMA node, or scattered?
- How many physical cores and sockets am I actually using?
- Are my GPUs on the same NUMA node as my CPUs?

## Example

Process analysis on a 2-socket, 256-CPU machine where a container uses 16 CPUs on NUMA Node 1:

```
numa-check — PID 4521

  Process — PID 4521
  ──────────────────
  Allowed CPUs ......... 16 / 256 (pinned)
  Currently on ......... CPU 163 → NUMA Node 1

  ■ = allowed  ★ = current  □ = not allowed

  NUMA Node 0 — Socket 0         NUMA Node 1 — Socket 1
  □ □ □ □ □ □ □ □ □ □ □ □ □ □ □ □    ■ ■ ■ ★ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■
  □ □ □ □ □ □ □ □ □ □ □ □ □ □ □ □    □ □ □ □ □ □ □ □ □ □ □ □ □ □ □ □
  ...                                 ...
  0 of 128 CPUs                       16 of 128 CPUs
```

Machine topology mode (no PID needed):

```
$ numa-check -topo

numa-check — Machine Topology

  Topology
  ────────
  256 CPUs (128 physical cores), 2 NUMA nodes, 2 sockets, 4 GPUs

  NUMA Node 0 — Socket 0         NUMA Node 1 — Socket 1
  ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■    ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■
  ...                                    ...
  128 CPUs (0–127)                       128 CPUs (128–255)

  ▀▀ GPU 0    ▀▀ GPU 1               ▀▀ GPU 2    ▀▀ GPU 3
  2 GPUs                              2 GPUs
```

## Install

```
GOOS=linux go build -o numa-check .
```

Or use `make build`.

## Usage

```
# Machine topology (no PID required)
numa-check -topo

# Process analysis by PID
numa-check -pid <PID>

# By Kubernetes pod/container (requires crictl)
numa-check -pod <pod-name> -container <container-name>

# With numastat memory stats
numa-check -pid <PID> -numastat

# Show version
numa-check -version

# Enable debug logging
numa-check -pid <PID> -debug
```

## Flags

| Flag | Description |
|---|---|
| `-pid` | Process ID to analyze |
| `-pod` | Pod name (for container lookup via crictl) |
| `-container` | Container name (in the pod) |
| `-topo` | Show machine topology only (no process analysis) |
| `-numastat` | Print numastat memory stats (requires `-pid` or `-pod`/`-container`) |
| `-debug` | Enable debug logging to stderr |
| `-version` | Print version and exit |

## GPU Detection

GPU detection is automatic and requires no flags. The tool uses a two-phase approach:

1. **PCI scan**: Checks `/sys/bus/pci/devices/*/vendor` for NVIDIA devices (vendor `0x10de`). If no NVIDIA PCI devices are found, GPU analysis is skipped entirely.
2. **nvidia-smi**: If NVIDIA PCI devices are detected, calls `nvidia-smi` for UUID/PCI mappings. If `nvidia-smi` is unavailable, GPUs are shown with PCI IDs only.

## Color Output

Output uses ANSI colors when stdout is a TTY. To disable colors, set the `NO_COLOR` environment variable:

```
NO_COLOR=1 numa-check -topo
```

## Requirements

- Linux with `/proc` and `/sys`
- Optional: `nvidia-smi` (GPU UUID mapping), `crictl` (container lookup), `numastat` (memory stats)
- No external dependencies for core CPU/NUMA analysis — reads sysfs directly
