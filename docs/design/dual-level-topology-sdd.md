# Software Design Document: Dual-Level Accelerated Domain Support

## Status

In Progress

## Summary

Expand Topograph's accelerator topology model from a single locality level to
two nested levels, enabling schedulers to make placement decisions at both the
accelerator-domain level and the sub-domain level. The feature introduces a
`SubDomain` field in `topology.HostInfo` that identifies which sub-domain (e.g.
a rack) a host belongs to within its accelerator domain, and a flat two-level
tree-building algorithm that converts this membership into contiguous,
consistently-padded block groups for Slurm and Kubernetes schedulers.

This addresses [NVIDIA/topograph#415](https://github.com/NVIDIA/topograph/issues/415).

## Background

### Existing limitation

The canonical `topology.Graph` currently supports only one accelerator locality
dimension. Each host belongs to exactly one accelerator domain (e.g. an NVLink
switch or a DRA partition), but there is no model for sub-domains within that
accelerator domain — the individual racks or partitions that share the domain's
NVLink/InfiniBand fabric.

As a result:

- Slurm cannot differentiate placement within a sub-domain (one rack) from
  placement within the full accelerator domain (all racks in the domain).
- Missing or offline sub-domains shift base-block positions, breaking Slurm's
  position-based aggregate inference.
- Clusters with a natural two-level hierarchy (e.g. multiple racks grouped into
  a single NVLink Scalable Unit) require manual `topology.conf` authoring to
  express the sub-domain boundary.
- Kubernetes schedulers cannot expose both levels as independent topology labels.

## Goals

**Slurm integration:**
- Generate block-size strings following the pattern `BlockSizes=N,R×N` where `N`
  is the sub-domain node count and `R` is the sub-domain count per accelerator
  domain.
- Emit consecutive base blocks for each accelerator domain with no interleaving
  across accelerator domains.
- Reserve all positions when an accelerator domain is discovered, even if some
  sub-domains within it are not yet online.
- Use placeholder `BlockName` entries for unavailable or undiscovered sub-domains.
- Apply deterministic, position-stable naming across all Slurm topology outputs.

**Kubernetes integration:**
- Expose both hierarchy levels as configurable topology labels so that pods and
  node-feature rules can target either the sub-domain or the accelerator domain.

**Compatibility:**
- Preserve full backward compatibility: accelerator domains that carry no
  `SubDomain` must produce output identical to the pre-change single-level behavior.

## Non-Goals

- Changes to any provider (`aws`, `gcp`, `oci`, `netq`, etc.) are out of scope;
  those providers must opt in separately by populating `SubDomain`.
- Custom block-naming schemes are a provider responsibility and out of scope for
  the translate layer.
- Hierarchies deeper than two levels (accelerator domain → sub-domain → nodes)
  are not addressed by this change.

## Data Model Changes

### `topology.HostInfo`

`pkg/topology/domain.go` — one new optional string field that identifies the
sub-domain a host belongs to within its accelerator domain:

```go
type HostInfo struct {
    Domain     string
    InstanceID string
    HostName   string
    SubDomain  string // optional: sub-domain name within the accelerator domain
}
```

When `SubDomain` is empty the host is placed directly in its accelerator-domain
leaf node (single-level, original behavior). When `SubDomain` is set the host is
placed in a named sub-domain node under the accelerator domain node (dual-level).

Partially-configured deployments — where some hosts in a domain carry a
`SubDomain` and others do not — are detected and warned about: hosts with an
empty `SubDomain` in a grouped domain are skipped with a `klog.Warningf`, rather
than being silently bucketed under an empty-string key that would sort before all
real sub-domains and shift block numbering.

### `DomainTree` and `Vertex`

The tree is represented using the existing `topology.Vertex` type for node
structure, paired with an unexported companion type `vertexMeta` that holds the
domain-tree-specific metadata. This avoids modifying the general-purpose `Vertex`
struct.

```go
// vertexMeta holds domain-tree-specific metadata alongside a Vertex.
type vertexMeta struct {
    actualNodeCount  int
    desiredNodeCount int
    hosts            map[string]*HostInfo // non-nil only for leaf vertices
}

// DomainTree pairs a Vertex tree with per-vertex metadata.
type DomainTree struct {
    Root *Vertex                   // root of the Vertex tree
    meta map[*Vertex]*vertexMeta   // unexported; accessed via Hosts() and DesiredNodeCount()
}
```

`DomainTree` exposes two accessor methods used by `convert` in
`pkg/translate/block_tree.go`:

```go
func (dt *DomainTree) Hosts(v *Vertex) map[string]*HostInfo
func (dt *DomainTree) DesiredNodeCount(v *Vertex) int
```

Leaf vertices (sub-domain or single-level domain) have a non-nil `hosts` map in
their `vertexMeta`; interior vertices (accelerator domain in dual-level mode) have
a nil `hosts` map and carry sub-domain children via `Vertex.Vertices`. When no
`SubDomain` is set, the accelerator domain vertex itself is the leaf.

### Simulation model YAML

The model loader (`pkg/models/model.go`) reads two labels from each capacity
block:

- `network.topology.nvidia.com/accelerator` → `HostInfo.Domain` (accelerator domain)
- `network.topology.nvidia.com/sub-domain` → `HostInfo.SubDomain` (sub-domain)

No structural changes to the YAML schema are required.

## Algorithm: `buildBlockTree`

### Step 1 – `GetDomainTree`: build flat two-level tree

`DomainMap.GetDomainTree(blockSizes []int)` (`pkg/topology/domain.go`) returns a
`*DomainTree` whose `Root` is a `Vertex` tree with at most two levels below root:

**Single-level (no `SubDomain`):** The accelerator domain node is a leaf that
holds its hosts directly. This preserves the original behavior.

```
root
└── domain-A  (leaf, Hosts = {h1, h2, ...})
└── domain-B  (leaf, Hosts = {h3, h4, ...})
```

**Dual-level (with `SubDomain`):** One child node is created per distinct
`SubDomain` under each accelerator domain; each sub-domain node holds the hosts
that belong to it.

```
root
└── domain-01  (accelerator domain, Vertices = {sub-domain-01, sub-domain-02, ...})
│   └── sub-domain-01  (leaf, meta.hosts = {node-01 .. node-09})
│   └── sub-domain-02  (leaf, meta.hosts = {node-10 .. node-18})
│   └── ...
└── domain-02  (accelerator domain, Vertices = {sub-domain-01, sub-domain-02, ...})
    └── sub-domain-01  (leaf, meta.hosts = {node-145 .. node-153})
    └── ...
```

### Step 2 – `setDesiredCountByLevel`: assign slot capacities

`DomainTree.setDesiredCountByLevel` runs a BFS over `Root` and assigns
`desiredNodeCount` in `vertexMeta` for every vertex. All vertices at the same
tree depth receive the same value: the smallest `blockSize` that is
`>= maxActualNodeCount` across all vertices at that depth.

| Depth | Vertex type | `DesiredNodeCount` |
|---|---|---|
| 0 | root (`actualNodeCount == 0`) | `blockSizes[last]` |
| 1 | accelerator domain | smallest blockSize ≥ max domain host count |
| 2 | sub-domain | smallest blockSize ≥ max sub-domain host count |

This uniform-per-depth rule ensures all accelerator domains use the same slot
width, which is the precondition for non-interleaved block output.

### Step 3 – `convert`: translate to internal aggregate tree

`convert(src *topology.Vertex, dt *topology.DomainTree, baseBlockSize int)`
(`pkg/translate/block_tree.go`) recursively maps the `DomainTree` into the
internal `aggregateBlockNode`/`baseBlockNode` tree. Per-vertex metadata is
accessed through `dt.Hosts(src)` and `dt.DesiredNodeCount(src)`.

**Leaf vertex** (`dt.Hosts(src)` is non-empty):
```
groupSize = dt.DesiredNodeCount(src) / baseBlockSize
blocks    = splitIntoBaseBlocks(src.ID, sortedHosts, baseBlockSize)
pad with newEmptyBaseBlock until len(blocks) == groupSize
```

**Interior vertex** (has `src.Vertices`):
```
for each child ID in sorted(src.Vertices.keys()):
    append convert(src.Vertices[id], dt, baseBlockSize)
    accumulate nodeCount
while nodeCount < dt.DesiredNodeCount(src):
    append newEmptyChildAggregate(childCapacity, baseBlockSize)
```

Children are always visited in ascending alphabetical order, making block
assignments deterministic and reproducible across topology regenerations.

### Step 4 – `collectBaseBlockSlots` + numbering

`collectBaseBlockSlots` performs a left-to-right DFS over the aggregate tree,
collecting every `baseBlockNode` leaf in traversal order. Each slot is then
numbered `block001`, `block002`, … by position in that flat slice.

### Empty placeholder handling

Sub-domains absent from the live `DomainMap` appear as trailing empty slots
within their accelerator domain's aggregate: after exhausting real children,
`convert` pads with `newEmptyChildAggregate` until `DesiredNodeCount` is reached.
`baseBlockToBlockInfo` converts zero-host base blocks to `blockInfo` entries with
no name and no nodes. `toBlockTopology` writes them as bare `BlockName=blockNNN`
lines in `topology.conf` — the placeholder semantics required by Slurm.

## Example

**Topology:** two accelerator domains, each containing up to 16 sub-domains of 9
nodes. Accelerator domain 1 has 14 active sub-domains; accelerator domain 2 has
15 active sub-domains. `BlockSizes=[9, 144]`.

**`setDesiredCountByLevel` results:**
- Sub-domain nodes (depth 2): `DesiredNodeCount = 9` (max sub-domain size = 9)
- Accelerator domain nodes (depth 1): `DesiredNodeCount = 144` (smallest
  blockSize ≥ max domain host count)
- Root (depth 0): `DesiredNodeCount = 144`

**`convert` for domain-01:** 14 real sub-domain children + 2 empty padding slots
→ 16 base blocks, `nodeCount = 144`.

**`convert` for domain-02:** 15 real sub-domain children + 1 empty padding slot
→ 16 base blocks, `nodeCount = 144`.

**`convert` for root:** 2 accelerator domain children totalling 288 >
`DesiredNodeCount = 144` → no root-level padding.

**Output** (`BlockSizes=9,144`, 32 blocks total):
```
BlockName=block001 Nodes=...   ← domain-01 / sub-domain-01
...
BlockName=block014 Nodes=...   ← domain-01 / sub-domain-16
BlockName=block015             ← placeholder (sub-domain-03 absent)
BlockName=block016             ← placeholder (sub-domain-13 absent)
BlockName=block017 Nodes=...   ← domain-02 / sub-domain-01
...
BlockName=block031 Nodes=...   ← domain-02 / sub-domain-16
BlockName=block032             ← placeholder (sub-domain-11 absent)
BlockSizes=9,144
```

## Known Limitations

### Placeholder ordering

Empty placeholder blocks are appended at the end of each accelerator domain's
sub-domain list rather than inserted at the position of the missing sub-domain
in the alphabetically-sorted sequence.

**Operational impact:** Slurm's position-based aggregate inference will be
incorrect for slots occupied by placeholder blocks. For example, if sub-domains
at alphabetical positions 3 and 13 are absent, their placeholders appear as the
last two entries in the 16-slot group (positions 15 and 16), not at positions 3
and 13. Operators must not rely on placeholder slot position for scheduling
decisions until positional ordering is implemented.

Positional ordering would require providers to supply an explicit slot index and
a corresponding design update to the tree builder.

### Block names shift on membership changes

Block names (`block001`, …) are assigned by position in a left-to-right DFS
traversal sorted alphabetically. Adding or removing an accelerator domain or
sub-domain name changes the position of all subsequent entries. Stable naming
across topology changes requires a universe-based reserved-slot approach and is
tracked as a follow-up.

## Backward Compatibility

When no host carries a `SubDomain`, `GetDomainTree` produces a single-level tree
and `setDesiredCountByLevel` sizes slots based solely on accelerator domain host
counts. `convert` packs hosts into base blocks with uniform per-domain padding.
The output is identical to the pre-change single-level behavior.

## Test Plan

- `TestComplementDualLevel` in `pkg/translate/block_complement_test.go`: uses a
  two-level simulation model (`tests/models/dual-level.yaml`) with `BlockSizes=[9,144]`
  to assert 32 blocks — 16 per accelerator domain — with correct placeholder
  entries for absent sub-domains.
- All existing complement tests (`TestComplementMissingBaseBlock`,
  `TestComplementMissingLeafSegment`, `TestComplementKeepsSeparateAccelerators`,
  etc.) continue to pass, verifying backward compatibility for the no-`SubDomain`
  path.
- `pkg/topology` and `pkg/models` unit tests cover `GetDomainTree`,
  `setDesiredCountByLevel`, and `SubDomain` propagation from YAML labels.
