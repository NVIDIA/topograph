# Software Design Document: Dual-Level Accelerated Domain Support

## Status

In Progress

## Summary

Expand Topograph's accelerator topology model from a single locality level to
two nested levels, enabling schedulers to make placement decisions at both the
base-domain level and the parent-domain level. The feature introduces a
`SubDomain` field in `topology.HostInfo` that identifies which parent domain a
host belongs to within its accelerator domain, and a flat two-level tree-building
algorithm that converts this membership into contiguous, consistently-padded block
groups for Slurm and Kubernetes schedulers.

This addresses [NVIDIA/topograph#415](https://github.com/NVIDIA/topograph/issues/415).

## Background

### Existing limitation

The canonical `topology.Graph` currently supports only one accelerator locality
dimension. Each host belongs to exactly one accelerator domain (e.g. an NVLink
switch or a DRA partition), but there is no model for the next level up — the
parent domain that groups multiple accelerator domains together.

As a result:

- Slurm cannot differentiate placement within a base domain from placement within
  a parent domain that spans several base domains.
- Missing or offline base domains shift base-block positions, breaking Slurm's
  position-based aggregate inference.
- Clusters with a natural two-level hierarchy (e.g. racks grouped into Scalable
  Units) require manual `topology.conf` authoring to express the parent boundary.
- Kubernetes schedulers cannot expose both levels as independent topology labels.

## Goals

**Slurm integration:**
- Generate block-size strings following the pattern `BlockSizes=N,R×N` where `N`
  is the base-domain slot count and `R` is the parent-domain fanout.
- Emit consecutive base blocks for each parent domain with no interleaving across
  parent domains.
- Reserve all positions when a parent domain is discovered, even if some base
  domains within it are not yet online.
- Use placeholder `BlockName` entries for unavailable or undiscovered base domains.
- Apply deterministic, position-stable naming across all Slurm topology outputs.

**Kubernetes integration:**
- Expose both hierarchy levels as configurable topology labels so that pods and
  node-feature rules can target either the base domain or the parent domain.

**Compatibility:**
- Preserve full backward compatibility: accelerator domains that carry no
  `SubDomain` must produce output identical to the pre-change single-level behavior.

## Non-Goals

- Changes to any provider (`aws`, `gcp`, `oci`, `netq`, etc.) are out of scope;
  those providers must opt in separately by populating `SubDomain`.
- Custom block-naming schemes are a provider responsibility and out of scope for
  the translate layer.
- Hierarchies deeper than two levels (parent → base domain → nodes) are not
  addressed by this change.

## Data Model Changes

### `topology.HostInfo`

`pkg/topology/domain.go` — one new optional string field that identifies the
parent domain a host belongs to within its accelerator domain:

```go
type HostInfo struct {
    Domain     string
    InstanceID string
    HostName   string
    SubDomain  string // optional: parent-domain name within the accelerator domain
}
```

When `SubDomain` is empty the host is placed directly in its accelerator-domain
leaf node (single-level, original behavior). When `SubDomain` is set the host is
placed in a named group node under the domain node (dual-level).

Partially-configured deployments — where some hosts in a domain carry a
`SubDomain` and others do not — are detected and warned about: hosts with an
empty `SubDomain` in a grouped domain are skipped with a `klog.Warningf`, rather
than being silently bucketed under an empty-string key that would sort before all
real groups and shift block numbering.

### `DomainTreeNode`

`pkg/topology/domain.go` — intermediate tree node used during block topology
generation:

```go
type DomainTreeNode struct {
    Name             string
    ActualNodeCount  int // live host count at this node
    DesiredNodeCount int // slot capacity derived from BlockSizes
    Children         map[string]*DomainTreeNode
    Hosts            map[string]*HostInfo
}
```

Leaf nodes (accelerator domain or group) carry `Hosts`; interior nodes carry
`Children`.

### Simulation model YAML

The model loader (`pkg/models/model.go`) reads two labels from each capacity
block:

- `network.topology.nvidia.com/accelerator` → `HostInfo.Domain` (base accelerator
  domain)
- `network.topology.nvidia.com/group` → `HostInfo.SubDomain` (parent domain)

No structural changes to the YAML schema are required.

## Algorithm: `buildBlockTree`

### Step 1 – `GetDomainTree`: build flat two-level tree

`DomainMap.GetDomainTree(blockSizes []int)` (`pkg/topology/domain.go`) builds a
`DomainTreeNode` tree with at most two levels below root:

**Single-level (no `SubDomain`):** The domain node is a leaf that holds its hosts
directly. This preserves the original behavior.

```
root
└── domain-A  (leaf, Hosts = {h1, h2, ...})
└── domain-B  (leaf, Hosts = {h3, h4, ...})
```

**Dual-level (with `SubDomain`):** One child node is created per distinct
`SubDomain` under each domain; each group node holds the hosts that belong to it.

```
root
└── parent-domain-1  (interior, Children = {base-01, base-02, ...})
│   └── base-01  (leaf, Hosts = {node-01 .. node-09})
│   └── base-02  (leaf, Hosts = {node-10 .. node-18})
│   └── ...
└── parent-domain-2  (interior, Children = {base-01, base-02, ...})
    └── base-01  (leaf, Hosts = {node-145 .. node-153})
    └── ...
```

### Step 2 – `setDesiredCountByLevel`: assign slot capacities

A BFS pass assigns `DesiredNodeCount` to every node. All nodes at the same tree
depth receive the same value: the smallest `blockSize` that is
`>= maxActualNodeCount` across all nodes at that depth.

| Depth | Node type | `DesiredNodeCount` |
|---|---|---|
| 0 | root (`ActualNodeCount == 0`) | `blockSizes[last]` |
| 1 | domain / parent-domain | smallest blockSize ≥ max domain host count |
| 2 | group / base-domain | smallest blockSize ≥ max group host count |

This uniform-per-depth rule ensures all domains use the same slot width, which is
the precondition for non-interleaved block output.

### Step 3 – `convert`: translate to internal aggregate tree

`convert(src *DomainTreeNode, baseBlockSize int)` (`pkg/translate/block_tree.go`)
recursively maps the `DomainTreeNode` tree to the internal
`aggregateBlockNode`/`baseBlockNode` tree:

**Leaf node** (has `Hosts`):
```
groupSize = DesiredNodeCount / baseBlockSize
blocks    = splitIntoBaseBlocks(name, sortedHosts, baseBlockSize)
pad with newEmptyBaseBlock until len(blocks) == groupSize
```

**Interior node** (has `Children`):
```
for each child name in sorted(Children.keys()):
    append convert(child)
    accumulate nodeCount
while nodeCount < DesiredNodeCount:
    append newEmptyChildAggregate(childCapacity, baseBlockSize)
```

Children are always visited in ascending alphabetical order, making block
assignments deterministic and reproducible across topology regenerations.

### Step 4 – `collectBaseBlockSlots` + numbering

`collectBaseBlockSlots` performs a left-to-right DFS over the aggregate tree,
collecting every `baseBlockNode` leaf in traversal order. Each slot is then
numbered `block001`, `block002`, … by position in that flat slice.

### Empty placeholder handling

Base domains absent from the live `DomainMap` appear as trailing empty slots
within their parent-domain aggregate: after exhausting real children, `convert`
pads with `newEmptyChildAggregate` until `DesiredNodeCount` is reached.
`baseBlockToBlockInfo` converts zero-host base blocks to `blockInfo` entries with
no name and no nodes. `toBlockTopology` writes them as bare `BlockName=blockNNN`
lines in `topology.conf` — the placeholder semantics required by Slurm.

## Example

**Topology:** two parent domains, each containing up to 16 base domains of 9
nodes. Parent domain 1 has 14 active base domains; parent domain 2 has 15 active
base domains. `BlockSizes=[9, 144]`.

**`setDesiredCountByLevel` results:**
- Group nodes (depth 2): `DesiredNodeCount = 9` (max group size = 9)
- Domain nodes (depth 1): `DesiredNodeCount = 144` (smallest blockSize ≥ max
  domain host count)
- Root (depth 0): `DesiredNodeCount = 144`

**`convert` for parent domain 1:** 14 real group children + 2 empty padding slots
→ 16 base blocks, `nodeCount = 144`.

**`convert` for parent domain 2:** 15 real group children + 1 empty padding slot
→ 16 base blocks, `nodeCount = 144`.

**`convert` for root:** 2 domain children totalling 288 > `DesiredNodeCount = 144`
→ no root-level padding.

**Output** (`BlockSizes=9,144`, 32 blocks total):
```
BlockName=block001 Nodes=...   ← parent-domain-1 / base-01
...
BlockName=block014 Nodes=...   ← parent-domain-1 / base-16
BlockName=block015             ← placeholder (base-15 absent)
BlockName=block016             ← placeholder (base-03 absent)
BlockName=block017 Nodes=...   ← parent-domain-2 / base-01
...
BlockName=block031 Nodes=...   ← parent-domain-2 / base-16
BlockName=block032             ← placeholder (base-11 absent)
BlockSizes=9,144
```

## Known Limitations

### Placeholder ordering

Empty placeholder blocks are appended at the end of each parent-domain's group
list rather than inserted at the position of the missing base domain in the
alphabetically-sorted sequence. Positional ordering would require providers to
supply an explicit slot index, which is a follow-up design change.

### Block names shift on membership changes

Block names (`block001`, …) are assigned by position in a left-to-right DFS
traversal sorted alphabetically. Adding or removing a domain or group name changes
the position of all subsequent entries. Stable naming across topology changes
requires a universe-based reserved-slot approach and is tracked as a follow-up.

## Backward Compatibility

When no host carries a `SubDomain`, `GetDomainTree` produces a single-level tree
and `setDesiredCountByLevel` sizes slots based solely on domain host counts.
`convert` packs hosts into base blocks with uniform per-domain padding. The output
is identical to the pre-change single-level behavior.

## Test Plan

- `TestComplementDualLevel` in `pkg/translate/block_complement_test.go`: uses a
  two-level simulation model (`tests/models/dual-level.yaml`) with `BlockSizes=[9,144]`
  to assert 32 blocks — 16 per parent domain — with correct placeholder entries
  for absent base domains.
- All existing complement tests (`TestComplementMissingBaseBlock`,
  `TestComplementMissingLeafSegment`, `TestComplementKeepsSeparateAccelerators`,
  etc.) continue to pass, verifying backward compatibility for the no-`SubDomain`
  path.
- `pkg/topology` and `pkg/models` unit tests cover `GetDomainTree`,
  `setDesiredCountByLevel`, and `SubDomain` propagation from YAML labels.
