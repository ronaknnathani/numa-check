# CPU Manager Per-NUMA-Node Allocation Summary — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show how many cores are exclusively allocated per NUMA node and how many remain, in both text and JSON output.

**Architecture:** Cross-reference `CPUManagerEntry` CPU lists with `NUMANodeInfo` CPU lists to compute per-node exclusive/remaining counts. Add a `PerNUMANode` field to JSON output and a "Per NUMA node" text section after the existing container list.

**Tech Stack:** Go, standard library only.

---

### Task 1: Add JSON type and per-NUMA aggregation logic

**Files:**
- Modify: `types.go:170-174` — add `PerNUMANode` field and new struct
- Modify: `cpumanager.go:48-65` — add `nodes` param to `toJSONCPUManager`, populate per-NUMA data

- [ ] **Step 1: Add `jsonCPUManagerNUMANode` struct and `PerNUMANode` field to `types.go`**

In `types.go`, after the `jsonCPUManagerEntry` struct (line 180), add:

```go
type jsonCPUManagerNUMANode struct {
	NodeID        int `json:"node_id"`
	ExclusiveCPUs int `json:"exclusive_cpus"`
	RemainingCPUs int `json:"remaining_cpus"`
	TotalCPUs     int `json:"total_cpus"`
}
```

In the `jsonCPUManager` struct (line 170-174), add a new field after `Entries`:

```go
type jsonCPUManager struct {
	PolicyName  string                   `json:"policy_name"`
	DefaultCPUs []int                    `json:"default_cpus,omitempty"`
	Entries     []jsonCPUManagerEntry    `json:"entries,omitempty"`
	PerNUMANode []jsonCPUManagerNUMANode `json:"per_numa_node,omitempty"`
}
```

- [ ] **Step 2: Update `toJSONCPUManager` in `cpumanager.go` to accept `[]NUMANodeInfo` and compute per-NUMA stats**

Change the signature from:
```go
func toJSONCPUManager(state *CPUManagerState, entries []CPUManagerEntry) *jsonCPUManager {
```
to:
```go
func toJSONCPUManager(state *CPUManagerState, entries []CPUManagerEntry, nodes []NUMANodeInfo) *jsonCPUManager {
```

After the existing entries loop (before the `return`), add:

```go
	// Build set of all exclusively assigned CPUs.
	exclusiveSet := make(map[int]bool)
	for _, e := range entries {
		for _, cpu := range e.CPUs {
			exclusiveSet[cpu] = true
		}
	}

	// Per-NUMA-node breakdown.
	for _, n := range nodes {
		exclusive := 0
		for _, cpu := range n.CPUs {
			if exclusiveSet[cpu] {
				exclusive++
			}
		}
		jcm.PerNUMANode = append(jcm.PerNUMANode, jsonCPUManagerNUMANode{
			NodeID:        n.ID,
			ExclusiveCPUs: exclusive,
			RemainingCPUs: len(n.CPUs) - exclusive,
			TotalCPUs:     len(n.CPUs),
		})
	}
```

- [ ] **Step 3: Update all 4 call sites in `main.go` to pass `nodes`**

In `runTopoOnly` (line 176):
```go
out.CPUManager = toJSONCPUManager(cpuMgrState, cpuMgrEntries, nodes)
```

In `runAnalysis` (line 298):
```go
out.CPUManager = toJSONCPUManager(cpuMgrState, cpuMgrEntries, nodes)
```

- [ ] **Step 4: Run vet to confirm it compiles**

Run: `GOOS=linux GOARCH=amd64 go vet ./...`
Expected: clean (no output)

- [ ] **Step 5: Update `TestToJSONCPUManager` in `cpumanager_test.go` to pass nodes and verify per-NUMA output**

Replace the existing `TestToJSONCPUManager` function (line 152-174) with:

```go
func TestToJSONCPUManager(t *testing.T) {
	state := &CPUManagerState{
		PolicyName:    "static",
		DefaultCPUSet: "0-3",
	}
	entries := []CPUManagerEntry{
		{PodUID: "pod-uid-1", ContainerName: "main", CPUs: []int{4, 5, 6, 7}},
	}
	nodes := []NUMANodeInfo{
		{ID: 0, CPUs: []int{0, 1, 2, 3, 4, 5, 6, 7}},
		{ID: 1, CPUs: []int{8, 9, 10, 11, 12, 13, 14, 15}},
	}

	got := toJSONCPUManager(state, entries, nodes)
	if got.PolicyName != "static" {
		t.Errorf("PolicyName = %q, want %q", got.PolicyName, "static")
	}
	if !reflect.DeepEqual(got.DefaultCPUs, []int{0, 1, 2, 3}) {
		t.Errorf("DefaultCPUs = %v, want [0 1 2 3]", got.DefaultCPUs)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("len(Entries) = %d, want 1", len(got.Entries))
	}
	if got.Entries[0].PodUID != "pod-uid-1" || got.Entries[0].ContainerName != "main" {
		t.Errorf("Entry = %+v, want pod-uid-1/main", got.Entries[0])
	}

	// Per-NUMA-node: CPUs 4-7 are exclusive on node 0, node 1 has none.
	if len(got.PerNUMANode) != 2 {
		t.Fatalf("len(PerNUMANode) = %d, want 2", len(got.PerNUMANode))
	}
	n0 := got.PerNUMANode[0]
	if n0.NodeID != 0 || n0.ExclusiveCPUs != 4 || n0.RemainingCPUs != 4 || n0.TotalCPUs != 8 {
		t.Errorf("Node 0 = %+v, want {0, 4, 4, 8}", n0)
	}
	n1 := got.PerNUMANode[1]
	if n1.NodeID != 1 || n1.ExclusiveCPUs != 0 || n1.RemainingCPUs != 8 || n1.TotalCPUs != 8 {
		t.Errorf("Node 1 = %+v, want {1, 0, 8, 8}", n1)
	}
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./... -run TestToJSONCPUManager -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add types.go cpumanager.go cpumanager_test.go main.go
git commit -m "Add per-NUMA-node allocation stats to CPU manager JSON output"
```

---

### Task 2: Add per-NUMA-node summary to text display

**Files:**
- Modify: `display.go:266-295` — update `printCPUManagerSection` to accept `[]NUMANodeInfo` and append NUMA summary
- Modify: `main.go:199,355` — pass `nodes` to `printCPUManagerSection`

- [ ] **Step 1: Update `printCPUManagerSection` signature and add NUMA summary**

Change the signature from:
```go
func printCPUManagerSection(state *CPUManagerState, entries []CPUManagerEntry) {
```
to:
```go
func printCPUManagerSection(state *CPUManagerState, entries []CPUManagerEntry, nodes []NUMANodeInfo) {
```

At the end of the function (after the entries loop, before the closing `}`), add:

```go
	if len(nodes) == 0 {
		return
	}

	// Build set of all exclusively assigned CPUs.
	exclusiveSet := make(map[int]bool)
	for _, e := range entries {
		for _, cpu := range e.CPUs {
			exclusiveSet[cpu] = true
		}
	}

	fmt.Printf("\n  Per NUMA node:\n")
	for _, n := range nodes {
		exclusive := 0
		for _, cpu := range n.CPUs {
			if exclusiveSet[cpu] {
				exclusive++
			}
		}
		remaining := len(n.CPUs) - exclusive
		fmt.Printf("    Node %d:  %d exclusive / %d remaining  (of %d)\n",
			n.ID, exclusive, remaining, len(n.CPUs))
	}
```

- [ ] **Step 2: Update both call sites in `main.go`**

Line 199 in `runTopoOnly`:
```go
printCPUManagerSection(cpuMgrState, cpuMgrEntries, nodes)
```

Line 355 in `runAnalysis`:
```go
printCPUManagerSection(cpuMgrState, cpuMgrEntries, nodes)
```

- [ ] **Step 3: Run vet and tests**

Run: `GOOS=linux GOARCH=amd64 go vet ./... && go test ./...`
Expected: clean vet, all tests pass

- [ ] **Step 4: Commit**

```bash
git add display.go main.go
git commit -m "Add per-NUMA-node allocation summary to CPU manager text output"
```

---

### Validation

After both tasks, verify the full build and test suite:

```bash
GOOS=linux GOARCH=amd64 go vet ./...
GOOS=linux GOARCH=amd64 go build -o /dev/null .
go test ./... -v
go test -cover ./...
```

All should pass cleanly.
