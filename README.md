# numa-check

Analyze NUMA topology for a Linux process. Reports CPU affinity, cpuset pinning,
physical core/package mapping, NUMA node distribution, and optionally GPU locality.

Designed for Kubernetes nodes to diagnose container CPU and GPU placement.

## Requirements

- Linux with `/proc` and `/sys` filesystems
- Optional: `nvidia-smi` (for `-gpu`), `crictl` (for `-pod`/`-container`), `numastat` (for `-numastat`)

## Build

    GOOS=linux go build -o numa-check .

## Usage

    # Analyze by PID
    numa-check -pid 12345

    # Analyze by Kubernetes pod and container (requires crictl)
    numa-check -pod my-pod -container my-container

    # Include GPU NUMA locality analysis
    numa-check -pid 12345 -gpu

    # Include numastat memory stats
    numa-check -pid 12345 -numastat
