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

```
$ numa-check -pid 4521 -gpu

Analyzing PID: 4521

Allowed CPUs: [32 33 34 35 36 37 38 39 40 41 42 43 44 45 46 47]
Process is pinned to 16 of 128 system CPUs.
Currently running on CPU: 35
Current CPU NUMA node: 1
Unique physical cores: 8, unique packages (sockets): 1
Allowed CPUs span 1 NUMA node(s): [1]

GPU NUMA Analysis:
Process allowed GPUs (by UUID): [GPU-a1b2c3d4-5678-90ab-cdef-111111111111]
GPU GPU-a1b2c3d4-5678-90ab-cdef-111111111111 (PCI: 0000:3b:00.0): NUMA node 1 (within allowed CPU NUMA nodes)
```

A badly placed container might look like:

```
Allowed CPUs span 2 NUMA node(s): [0 1]
GPU ...: NUMA node 0 (OUTSIDE allowed CPU NUMA nodes)
```

## Install

```
GOOS=linux go build -o numa-check .
```

Or use `make build`.

## Usage

```
# By PID
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
