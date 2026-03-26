# numa-check

## Why

On multi-socket servers, a CPU accessing memory on a remote NUMA node pays a steep latency penalty. Kubernetes can silently scatter a container's CPUs across NUMA nodes, or place GPUs on a different node than the CPUs they serve. This destroys performance for latency-sensitive and GPU workloads, and nothing in `kubectl` will tell you it's happening.

## What

`numa-check` is a single-binary Linux CLI that reads sysfs and procfs to show you exactly where a process (or container) is placed in the machine's NUMA topology. It reports CPU affinity, pinning status, physical core layout, NUMA node distribution, and GPU-to-NUMA locality -- all rendered as a visual grid so you can spot misplacement at a glance.

## Install

```
GOOS=linux go build -o numa-check .
```

Copy the binary to your target node. No external dependencies for core analysis -- it reads `/sys` and `/proc` directly.

## Usage

**See the machine topology** (no PID needed):

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

**Check a process by PID** -- the grid shows which CPUs are allowed (`■`), which CPU is currently running (`★`), and which are unavailable (`□`):

```
$ numa-check -pid 4521

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

All 16 CPUs on Node 1, none on Node 0 -- clean placement.

**Check a Kubernetes container** (requires `crictl` on the node):

```
$ numa-check -pod my-pod -container my-container
```

**Include numastat memory stats:**

```
$ numa-check -pid 4521 -numastat
```

## Requirements

- Linux with `/proc` and `/sys`
- Optional: `nvidia-smi` (GPU detection), `crictl` (container PID lookup), `numastat` (memory stats)
