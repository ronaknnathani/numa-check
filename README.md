# numacheck

## Why

On multi-socket servers, a CPU accessing memory on a remote NUMA node pays a steep latency penalty. Kubernetes can silently scatter a container's CPUs across NUMA nodes, or place GPUs on a different node than the CPUs they serve. This destroys performance for latency-sensitive and GPU workloads, and nothing in `kubectl` will tell you it's happening.

## What

`numacheck` is a single-binary Linux CLI that reads sysfs and procfs to show you exactly where a process (or container) is placed in the machine's NUMA topology. It reports CPU affinity, pinning status, physical core layout, NUMA node distribution, and GPU-to-NUMA locality -- all rendered as a visual grid so you can spot misplacement at a glance.

## Install

```
GOOS=linux go build -o numacheck .
```

Copy the binary to your target node. No external dependencies for core analysis -- it reads `/sys` and `/proc` directly.

## Usage

**See the machine topology** (no PID needed):

```
numacheck -topo
```

![Machine topology](images/topo.png)

**Check a process by PID** -- the grid shows which CPUs are allowed, which CPU is currently running, and which are unavailable:

```
numacheck -pid <PID>
```

![Process analysis](images/pid.png)

**Check a Kubernetes container** (requires `crictl` on the node) -- also shows container resource requests/limits:

```
numacheck -pod <pod> -container <container>
```

![Container analysis](images/pod.png)

**JSON output** for scripting and automation:

```
$ numacheck -topo -json
$ numacheck -pid 4521 -json
$ numacheck -pod my-pod -container my-container -json
```

The `-json` flag emits a versioned, k8s-style envelope (`apiVersion: numacheck/v1`) instead of the visual grid. Two kinds: `MachineTopology` for `-topo`, and `ProcessReport` for `-pid` / `-pod` + `-container`. Both carry an envelope (`apiVersion`, `kind`, `metadata`) and a `data` body keyed by kind.

`metadata` carries `timestamp`, `host`, `numacheckVersion`, plus `pid` / `pod` / `container` when applicable.

`MachineTopology.data` captures:
- `resources.cpu` — `logicalCores`, `physicalCores`, `sockets`
- `resources.memory` — `totalBytes`, `freeBytes`
- `resources.gpus[]` — per-GPU `index`, `uuid`, `pciId`, `numaNode`
- `numaNodes[]` — per-node `id`, `socketId`, `cpus`, `gpus` (GPU indexes co-located on this node), `memory.{totalBytes, freeBytes}`
- `cpuManager` (with `-cpumanager`) — kubelet policy, default CPU set, per-pod entries, and per-NUMA-node exclusive/remaining/total CPU counts

`ProcessReport.data.process` captures:
- `placement` — `currentCpu`, `currentNumaNode`, `pinned`
- `affinity.cpus`, `affinity.gpus` — CPUs (cpuset) and GPU UUIDs the process can access
- `affinity.memory.bytes`, `affinity.memory.perNumaNode[]` — process RSS in total and per NUMA node
- `container.resources` (with `-pod`/`-container`) — k8s-shaped `requests`/`limits` map of `cpu`, `memory`, `nvidia.com/gpu` to `resource.Quantity` strings

Note: `ProcessReport` does not include a `machine` block — run `numacheck -topo -json` for host topology. A metrics agent typically polls `-topo` once per host and `-pod`/`-pid` per workload.

### Using `numacheck` as a metrics source

The schema is designed to plug into a host-level metrics agent that runs `numacheck -topo -json` periodically and `numacheck -pod ... -container ... -json` per pod/container, then emits dimensional metrics. Everything under `metadata` (host, pod, container, pid) maps directly to metric labels; everything else is a measurement. Useful derivable metrics:

- **Machine topology** — CPUs/cores/sockets per host; memory total/free per NUMA node; GPU count per NUMA node.
- **Container NUMA alignment** — whether `affinity.cpus` is confined to a single NUMA node, whether `affinity.gpus` are co-located with those CPUs, and what fraction of `affinity.memory.bytes` lives on the same NUMA node as the allowed CPUs (derived from `affinity.memory.perNumaNode[]`).
- **CPU pinning** — `len(affinity.cpus) / resources.cpu.logicalCores` per container (using host topology from `-topo`); per-node `exclusiveCpus / totalCpus` from `cpuManager.perNumaNode[]` for capacity planning.
- **Resource fit** — `affinity.memory.bytes` parsed against `container.resources.limits.memory`.

`apiVersion` is the migration contract: a consumer that pins `numacheck/v1` is guaranteed the field shape above and can fail loudly on a future `numacheck/v2`.

Example `MachineTopology`:

```json
{
  "apiVersion": "numacheck/v1",
  "kind": "MachineTopology",
  "metadata": {
    "timestamp": "2026-05-25T19:30:00Z",
    "host": "node-foo",
    "numacheckVersion": "dev"
  },
  "data": {
    "resources": {
      "cpu":    { "logicalCores": 96, "physicalCores": 48, "sockets": 2 },
      "memory": { "totalBytes": 805306368000, "freeBytes": 200000000000 },
      "gpus":   [ { "index": 0, "uuid": "GPU-...", "pciId": "0000:17:00.0", "numaNode": 0 } ]
    },
    "numaNodes": [
      {
        "id": 0,
        "socketId": 0,
        "cpus":   [0,1,2,3],
        "gpus":   [0],
        "memory": { "totalBytes": 402653184000, "freeBytes": 100000000000 }
      }
    ]
  }
}
```

Example `ProcessReport`:

```json
{
  "apiVersion": "numacheck/v1",
  "kind": "ProcessReport",
  "metadata": {
    "timestamp": "2026-05-25T19:30:00Z",
    "host": "node-foo",
    "numacheckVersion": "dev",
    "pid": 12345,
    "pod": "my-pod",
    "container": "my-container"
  },
  "data": {
    "process": {
      "placement": { "currentCpu": 2, "currentNumaNode": 0, "pinned": true },
      "affinity": {
        "cpus": [0,1,2,3],
        "gpus": ["GPU-..."],
        "memory": {
          "bytes": 2147483648,
          "perNumaNode": [ { "id": 0, "bytes": 2147483648 } ]
        }
      },
      "container": {
        "resources": {
          "requests": { "cpu": "4" },
          "limits":   { "cpu": "8", "memory": "16Gi", "nvidia.com/gpu": "1" }
        }
      }
    }
  }
}
```

## Requirements

- Linux with `/proc` and `/sys`
- Optional: `nvidia-smi` (GPU detection), `crictl` (container PID lookup)
