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
- Expose both hierarchy levels as topology labels so that pods and
  node-feature rules can target either the sub-domain or the accelerator domain.

**Compatibility:**
- Preserve full backward compatibility: accelerator domains that carry no
  `SubDomain` must produce output identical to the pre-change single-level behavior.

## Non-Goals

- Provider-specific discovery of accelerator sub-domains beyond OCI is out of
  scope; those providers must opt in separately by populating
  `InstanceTopology.XclrSubDomainID`.
- Custom block-naming schemes are a provider responsibility and out of scope for
  the translate layer.
- Hierarchies deeper than two levels (accelerator domain → sub-domain → nodes)
  are not addressed by this change.

## Data Model Changes

### `topology.InstanceTopology`

`pkg/topology/graph.go` represents accelerator locality explicitly:

```go
type InstanceTopology struct {
    InstanceID      string
    FabricTiers     []FabricTier
    XclrDomainID    string // accelerator domain ID
    XclrSubDomainID string // optional accelerator sub-domain ID
    Instance        *Instance
}
```

`XclrDomainID` identifies the top-level accelerator domain. When
`XclrSubDomainID` is non-empty, graph conversion places the host in that
sub-domain beneath `XclrDomainID`; otherwise the host remains directly in the
domain for backward-compatible single-level output. These fields replace the
ambiguous `AcceleratorID` and `ParentAcceleratorID` pair.

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
empty `SubDomain` in a grouped domain are placed in a fallback sub-domain vertex
keyed by the accelerator domain name (with a `klog.Warningf`), so the host is
always emitted rather than silently dropped.

`DomainMap` also carries a new method `InferBlockSizes() []int` that derives a
`blockSizes` slice from the map's content without requiring explicit operator
configuration. See [BlockSizes Configuration — Automatic inference](#automatic-inference).

### `BlockVertex`

`BlockVertex` (`pkg/topology/domain.go`) augments the existing `topology.Vertex`
with domain-tree-specific metadata, keeping `Vertex` itself unmodified:

```go
type BlockVertex struct {
    Vertex
    ActualNodeCount   int
    MaxChildNodeCount int
    Hosts             map[string]*HostInfo    // non-nil only for leaf vertices
    Children          map[string]*BlockVertex // type-safe children (interior vertices only)
}
```

Children are stored in the `Children` map, which is typed `map[string]*BlockVertex` so
the compiler enforces that only `*BlockVertex` values are inserted. The inherited
`Vertex.Vertices` field is not used for `BlockVertex` child storage.

`ChildAt` is the accessor for callers that need to look up a child by name:

```go
func (bv *BlockVertex) ChildAt(name string) *BlockVertex
```

Leaf vertices (sub-domain or single-level domain) have a non-nil `Hosts` map;
interior vertices (accelerator domain in dual-level mode) have a nil `Hosts` map
and carry sub-domain children via `Children`. When no `SubDomain` is set,
the accelerator domain vertex itself is the leaf.

### Simulation model YAML

The model loader (`pkg/models/model.go`) reads two annotations from each capacity
block:

- `accelerator.topology.test/domain` → `HostInfo.Domain` (accelerator domain)
- `accelerator.topology.test/sub-domain` → `HostInfo.SubDomain` (sub-domain)

No structural changes to the YAML schema are required.

## Algorithm: `buildBlockTree`

### Step 1 – `GetDomainTree`: build flat two-level tree

`DomainMap.GetDomainTree()` (`pkg/topology/domain.go`) returns a
`*BlockVertex` root of a tree with at most two levels below root:

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
root (BlockVertex)
└── domain-01  (BlockVertex, Children = {sub-domain-01, sub-domain-02, ...})
│   └── sub-domain-01  (BlockVertex leaf, Hosts = {node-01 .. node-09})
│   └── sub-domain-02  (BlockVertex leaf, Hosts = {node-10 .. node-18})
│   └── ...
└── domain-02  (BlockVertex, Children = {sub-domain-01, sub-domain-02, ...})
    └── sub-domain-01  (BlockVertex leaf, Hosts = {node-145 .. node-153})
    └── ...
```

`GetDomainTree` also sets `MaxChildNodeCount` on every interior vertex — the
largest `ActualNodeCount` among its direct children. The translate layer reads
this to size base-block slots without a separate BFS pass.

### Step 2 – `toRootAggregate` + `toDomainAggregate`: translate to internal aggregate tree

`buildBlockTree` calls `toRootAggregate(root, blockSizes)` which iterates the
root's domain children in sorted name order and converts each one by calling
`toDomainAggregate(domain, root.MaxChildNodeCount, blockSizes)`.

#### Slot capacity

Before using the dual-level conversion strategies, `toRootAggregate` detects an
entirely single-level tree (every accelerator domain stores hosts directly). That
shape uses the pre-dual-level allocation rules:

| Configuration | Single-level domain allocation |
|---|---|
| One `BlockSizes` entry | `ceil(ActualNodeCount / blockSizes[0])` base blocks per domain |
| Multiple `BlockSizes` entries | Every domain receives the same `aggregateSlotCapacity(maxSiblingNodes, blockSizes[0])` slot |

This compatibility path is selected only when every root domain is single-level.
A mixed tree containing any dual-level domain uses the new conversion path for
all domains.

For dual-level and mixed trees, `toDomainAggregate` computes `numBaseBlocks`
differently for leaf and interior nodes:

**Leaf nodes** (`src.Hosts != nil`, single-level domain):

| Condition | Formula |
|---|---|
| `blockSizes[last] ≥ maxSiblingNodes` (normal) | `aggregateSlotCapacity(maxSiblingNodes, blockSizes[0]) / blockSizes[0]` |
| `blockSizes[last] < maxSiblingNodes` (oversized) | `⌈ActualNodeCount / blockSizes[0]⌉` |

The oversized case uses ceiling division rather than power-of-2 rounding. Power-of-2
rounding is only needed for interior nodes (to keep sub-domain slots uniform); applying
it to a leaf in the dual-level path produces spurious empty placeholder blocks.

**Interior nodes** (`src.Hosts == nil`, two-level domain):

| Condition | `nodeCount` |
|---|---|
| `blockSizes[last] ≥ maxSiblingNodes` (normal) | `aggregateSlotCapacity(maxSiblingNodes, blockSizes[0])` |
| `blockSizes[last] < maxSiblingNodes` (oversized) | `aggregateSlotCapacity(src.ActualNodeCount, blockSizes[0])` |

`numBaseBlocks = nodeCount / blockSizes[0]`.

`maxSiblingNodes` is the `MaxChildNodeCount` of the caller's vertex (the largest
`ActualNodeCount` among the siblings being processed), ensuring uniform slot widths.

#### Three conversion strategies

**Strategy 1 — leaf vertex** (`src.Hosts != nil`):
```
numBaseBlocks = (see slot capacity table above)
blocks = splitIntoBaseBlocks(src.ID, sortedHosts, blockSizes[0])
pad with newEmptyBaseBlock until len(blocks) == numBaseBlocks
```

**Strategy 2 — small sub-domains** (`src.MaxChildNodeCount < blockSizes[0]`):
Sub-domain hosts are accumulated in sorted order using a greedy lookahead flush: before
appending each child's hosts, the pending set is flushed into a new base block if adding
the child would exceed `blockSizes[0]`. Each flushed block is named with the `"+"`
delimited sub-domains that contributed to it. Any remaining hosts form a partial base
block. Result is padded to `numBaseBlocks`.

This strategy fills each block as densely as possible. When a `blockName` formatter is
configured, Strategy 2 is automatically skipped — combining hosts from different
sub-domains into one block makes per-sub-domain name derivation ambiguous. Strategy 3
is used instead. To keep each sub-domain in its own block without a formatter, set
`BlockSizes[0]` equal to the sub-domain node count so that Strategy 3 fires.

**Strategy 3 — larger sub-domains** (`src.MaxChildNodeCount ≥ blockSizes[0]`):
`toDomainAggregate` is called recursively on each sub-domain child using
`src.MaxChildNodeCount` as `maxSiblingNodes`. The resulting base blocks are
flattened into the current aggregate, and the list is padded to `numBaseBlocks`
with empty base blocks.

Children are always visited in ascending alphabetical order for determinism.

#### Root-level padding

After all domain aggregates are collected, `toRootAggregate` pads with empty domain
aggregates until the total `nodeCount` is a positive multiple of `blockSizes[last]`.
The step uses `lcm(childCapacity, blockSizes[last])` to avoid overshooting when the
two values are not aligned.

### Step 3 – `collectBaseBlockSlots` + numbering

`collectBaseBlockSlots` performs a left-to-right DFS over the aggregate tree,
collecting every `baseBlockNode` leaf in traversal order. Each slot is then
numbered `block001`, `block002`, … by position in that flat slice.

### Empty placeholder handling

Sub-domains absent from the live `DomainMap` appear as trailing empty base blocks
within their accelerator domain's aggregate: after processing all real sub-domain
children, `toDomainAggregate` pads to `numBaseBlocks` with `newEmptyBaseBlock`
entries. Similarly, `toRootAggregate` pads missing domain slots with
`newEmptyChildAggregate` until the root `nodeCount` reaches a multiple of
`blockSizes[last]`. `baseBlockToBlockInfo` converts zero-host base blocks to
`blockInfo` entries with no name and no nodes. `toBlockTopology` writes them as
bare `BlockName=blockNNN` lines in `topology.conf` — the placeholder semantics
required by Slurm.

## BlockSizes Configuration

### Automatic inference

When `BlockSizes` is not configured, `complementBlocks` calls `DomainMap.InferBlockSizes`
on the partition-local domain map to derive sizes automatically:

| Domain map shape | Inferred `BlockSizes` |
|---|---|
| Single-level (no host carries `SubDomain`) | `[D, 2D, 4D, ..., 2^k*D]`, where `D` is the smallest domain size and `k=floor(log2(N))` for `N` domains |
| Two-level (any host carries `SubDomain`) | `[maxSubDomainSize, aggregateSize]` — one base block per sub-domain; `aggregateSize` is the smallest power-of-2 multiple of `maxSubDomainSize` that is `≥ maxDomainSize` |
| Empty map | `nil` — complement is skipped, blocks returned unchanged |

The power-of-2 rounding for `aggregateSize` matches `aggregateSlotCapacity` inside
`buildBlockTree`, so the inferred sizes produce the same slot layout that explicit
configuration with those values would.

**Example (OCI, one fabric, two racks of 8 nodes each):** `maxSubDomainSize=8`,
`maxDomainSize=16`; inferred `BlockSizes=[8,16]`; output is two blocks of 8 nodes
each (one per rack) rather than one coarse block of 16.

Explicit `BlockSizes` overrides inference entirely — set it when you need a specific
packing strategy (e.g. Strategy 2 to combine small sub-domains across rack boundaries).

### Strategy selection

`BlockSizes[0]` (the base block size) determines how sub-domain hosts are packed into
base blocks. Only `BlockSizes[0]` requires careful selection; intermediate entries
(`BlockSizes[1]`, etc.) are used only for root-level padding and do not need to match
any particular topology level.

The tree builder selects one of two strategies based on how the sub-domain actual host
count (`MaxChildNodeCount`) compares to `BlockSizes[0]`:

**Strategy 2 — combine sub-domain hosts** (when `MaxChildNodeCount < BlockSizes[0]`):
Hosts from multiple sub-domains are accumulated using a greedy lookahead flush: a new base
block is flushed before appending the next sub-domain whenever doing so would exceed
`BlockSizes[0]`. This fills each block as densely as possible. When a `blockName` formatter
is configured, Strategy 2 is automatically skipped (Strategy 3 is used instead) to avoid
combining hosts from different sub-domains into one block, which makes per-sub-domain name
derivation ambiguous. To keep each sub-domain in its own block without a formatter, set
`BlockSizes[0]` equal to the sub-domain node count so that Strategy 3 fires.

**Strategy 3 — one slot per sub-domain** (when `MaxChildNodeCount ≥ BlockSizes[0]`):
Each sub-domain is assigned its own base block slot; slots are padded with empty host
placeholders when the sub-domain does not fill the full block. This preserves sub-domain
boundary granularity in the output — callers that rely on per-sub-domain block positions
for scheduling can use this strategy by setting `BlockSizes[0]` equal to or smaller than
`MaxChildNodeCount`.

**Example cluster:** 2 accelerator domains, each with 4 racks of 9 hosts = 36 hosts per
domain, 72 hosts total.

---

**`BlockSizes[0]=9` — rack exactly fills one base block:**

Every rack produces exactly 1 full base block. No host slots are wasted.
Domain slot capacity = `aggregateSlotCapacity(36, 9) = 36` → 4 base blocks per domain.
Root pads 2 domain slots to 288 with 6 empty domain aggregates.

```
block001 Nodes=node[a0-a8]    ← rack-0 of domain-a (full)
block002 Nodes=node[a9-a17]   ← rack-1 of domain-a (full)
block003 Nodes=node[a18-a26]  ← rack-2 of domain-a (full)
block004 Nodes=node[a27-a35]  ← rack-3 of domain-a (full)
... same for domain-b, then 24 empty root-padding blocks ...
BlockSizes=9,288
```

---

**`BlockSizes[0]=18`, rack has 9 hosts — Strategy 2 fires (`9 < 18`)**:

Sub-domain hosts are combined across rack boundaries using the greedy lookahead flush.
rack-0 (9): pending=0, no flush. pending=9. rack-1 (9): 9+9=18 ≤ 18, no flush. pending=18.
rack-2 (9): 18+9=27 > 18 → flush "rack-0+rack-1" (18 hosts). pending=9.
rack-3 (9): 9+9=18 ≤ 18, no flush. pending=18. End → flush "rack-2+rack-3" (18 hosts).
All 4 racks' 36 hosts pack into 2 full 18-node base blocks.

```
block001 Nodes=node[a0-a17]   ← combined racks 0+1 of domain-a (18 real, full)
block002 Nodes=node[a18-a35]  ← combined racks 2+3 of domain-a (18 real, full)
... same for domain-b, then 28 empty root-padding blocks ...
BlockSizes=18,288
```

---

**`BlockSizes[0]=18`, rack has 12 hosts — Strategy 2 fires (`12 < 18`)**:

Strategy 2 fires, but the greedy lookahead sees that each pair of racks would overflow
(12 + 12 = 24 > 18), so every rack still flushes into its own base block with 6 empty slots.
Sub-domain boundary is preserved in the output, same as if Strategy 3 had been used.

```
block001 Nodes=node[a0-a11]   ← rack-0 of domain-a (12 real + 6 empty host slots)
block002 Nodes=node[a12-a23]  ← rack-1 of domain-a (12 real + 6 empty host slots)
block003 Nodes=node[a24-a35]  ← rack-2 of domain-a (12 real + 6 empty host slots)
block004 Nodes=node[a36-a47]  ← rack-3 of domain-a (12 real + 6 empty host slots)
... same for domain-b, then 24 empty root-padding blocks ...
BlockSizes=18,288
```

To eliminate the per-slot waste, set `BlockSizes[0]=12` so each rack fills exactly one base
block and Strategy 3 fires (`12 ≥ 12`).

## Example

**Topology:** two accelerator domains, each containing up to 16 sub-domains of 9
nodes. Accelerator domain 1 has 14 active sub-domains; accelerator domain 2 has
15 active sub-domains. `BlockSizes=[9, 144]`.

**`GetDomainTree` populates:**
- `MaxChildNodeCount` on domain-01 = 9 (max sub-domain host count within domain-01)
- `MaxChildNodeCount` on domain-02 = 9
- `MaxChildNodeCount` on root = 131 (max domain `ActualNodeCount`; domain-02 has
  15 × 9 = 135? actually domain-02's `ActualNodeCount` = placed hosts)

**`toRootAggregate` for each domain:**
- `maxSiblingNodes = root.MaxChildNodeCount`
- `nodeCount = aggregateSlotCapacity(maxSiblingNodes, 9)` = 144 (9 × 16 ≥ max)
- `numBaseBlocks = 144 / 9 = 16`
- Strategy 3 (MaxChildNodeCount=9 ≥ blockSizes[0]=9): recurse into each sub-domain
  - Each sub-domain: `nodeCount = aggregateSlotCapacity(9, 9) = 9`, `numBaseBlocks = 1`
  - Strategy 1 (leaf): each sub-domain → 1 base block
- domain-01: 14 real sub-domain blocks + 2 empty = 16 base blocks
- domain-02: 15 real sub-domain blocks + 1 empty = 16 base blocks

**`toRootAggregate` root padding:**
- 2 domains × 144 = 288 nodes; `rootDesired = 144`; 288 % 144 = 0 → no padding.

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
where every domain vertex is a leaf with `Hosts` set directly. `toRootAggregate`
detects this shape and uses `toSingleLevelDomainAggregate`: a single `BlockSizes`
entry allocates only the minimum required base blocks, while multiple entries
reserve the same power-of-two slot for every domain based on the largest sibling.
The output is therefore identical to the pre-change single-level behavior.

When `BlockSizes` is not configured and no host carries a `SubDomain`, inference
preserves the existing single-level hierarchy. For `N` domains, `D` is the
smallest domain size and the result is `[D, 2D, 4D, ..., 2^k*D]`, where
`k=floor(log2(N))`. The single-level compatibility path is then taken.

## Test Plan

**Dual-level complement output (`pkg/translate/block_complement_test.go`):**
- `TestComplementDualLevel`: uses a two-level simulation model
  (`tests/models/dual-xclr-irregular.yaml`) with `BlockSizes=[9,144]` to assert
  32 blocks — 16 per accelerator domain — with correct placeholder entries for
  absent sub-domains.
- `TestComplementMultiGroupRootExpansion6x16`, `TestComplementMultiGroupRootExpansion3x72`:
  verify over-full root padding — when the total real-child capacity exceeds
  `blockSizes[last]` but is not yet a multiple of it, empty domain slots are
  appended to reach the next multiple boundary.
- `TestComplementSingleLevelOversizedDomain`: regression test for the power-of-2
  over-allocation fix — 1 domain, 5 hosts, `BlockSizes=[2]` (`lastBlockSize < maxSiblingNodes`)
  asserts exactly 3 blocks with no spurious empty placeholder.
- `TestComplementSingleLevelMixedOversizedDomainsPreservesUniformSlots`: verifies
  that differently sized single-level domains retain the legacy uniform slot
  reservation when the largest domain exceeds `blockSizes[last]`.
- `TestComplementMixedLevelDomainsUseDualLevelSizing`: verifies that a tree with
  any sub-domain bypasses the single-level compatibility path.
- `TestComplementSubDomainGranularityWithoutBlockSizes`: regression test for the
  OCI two-rack scenario — one fabric domain, 8 nodes per rack, `BlockSizes` unset;
  asserts two rack-level blocks rather than one coarse domain block, verifying that
  `InferBlockSizes` restores sub-domain granularity on the unconfigured path.
- All existing complement tests (`TestComplementMissingBaseBlock`,
  `TestComplementMissingLeafSegment`, `TestComplementKeepsSeparateAccelerators`,
  etc.) continue to pass, verifying backward compatibility for the no-`SubDomain`
  path.

**`DomainMap` unit tests (`pkg/topology/domain_test.go`):**
- `TestGetDomainTreeUnrackedHostFallback`: host with empty `SubDomain` in a grouped domain is placed in a fallback vertex keyed by the domain name rather than dropped.
- `TestInferBlockSizes`: covers nil (empty map), single-level domain counts that
  produce one or more inferred sizes, two-level equal sub-domains (returns
  `[maxSubDomainSize, aggregateSize]`), a single sub-domain per domain, and unequal
  sub-domain sizes (power-of-2 rounding of `aggregateSize`).

**Kubernetes label output:**
- `TestBuildNodeLabelsWithXclrSubDomain` in `pkg/engines/k8s/labeler_test.go`
  verifies that Kubernetes nodes receive both accelerator hierarchy labels when
  `XclrSubDomainID` is present.
