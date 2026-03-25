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

  Machine Topology
  ────────────────
  256 CPUs, 2 NUMA nodes, 2 sockets

  NUMA Node 0 — Socket 0         NUMA Node 1 — Socket 1
  ████████████████                ████████████████
  ████████████████                ████████████████
  ████████████████                ████████████████
  ████████████████                ████████████████
  ████████████████                ████████████████
  ████████████████                ████████████████
  ████████████████                ████████████████
  ████████████████                ████████████████
  128 CPUs (0–127)                128 CPUs (128–255)

  Process — PID 4521
  ──────────────────
  Allowed CPUs ......... 16 / 256 (pinned)
  Currently on ......... CPU 163 → NUMA Node 1
  Physical cores ....... 8 cores, 1 socket
  NUMA span ............ 1 node [1]

  ■ = allowed  ★ = current  · = not allowed

  NUMA Node 0                     NUMA Node 1
  ················                ················
  ················                ················
  ················                ■■■★■■■■■■■■■■■■
  ················                ················
  ················                ················
  ················                ················
  ················                ················
  ················                ················
  0 allowed                       16 allowed

  GPU Locality
  ────────────
  ✓ GPU-a1b2...c3d4 (0000:3b:00.0) → NUMA Node 1 same NUMA
```

Machine topology mode (no PID needed):

```
$ numa-check -topo

numa-check — Machine Topology

  CPU Topology
  ────────────
  256 CPUs, 2 NUMA nodes, 2 sockets

  NUMA Node 0 — Socket 0         NUMA Node 1 — Socket 1
  ████████████████                ████████████████
  ...                             ...

  GPU Topology
  ────────────
  4 GPUs

  ■ GPU-a1b2...c3d4 (0000:3b:00.0) → NUMA Node 0
  ■ GPU-e5f6...7890 (0000:3c:00.0) → NUMA Node 0
  ■ GPU-1234...5678 (0000:86:00.0) → NUMA Node 1
  ■ GPU-abcd...ef01 (0000:87:00.0) → NUMA Node 1
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

# With GPU NUMA analysis (requires nvidia-smi)
numa-check -pid <PID> -gpu

# With numastat memory stats
numa-check -pid <PID> -numastat
```

## Requirements

- Linux with `/proc` and `/sys`
- Optional: `nvidia-smi` (GPU analysis), `crictl` (container lookup), `numastat` (memory stats)
- No external dependencies for core CPU/NUMA analysis — reads sysfs directly
