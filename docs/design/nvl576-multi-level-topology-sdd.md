# Software Design Document: NVL576 Multi-Level Topology Support

## Status

In Progress

## Summary

Extend Topograph's block topology engine to represent multi-level NVLink domain
hierarchies — specifically NVL576 Scalable Units (SUs) — so that Slurm and
Kubernetes schedulers can distinguish placement within a rack from placement
within a full NVL576 SU. The feature introduces up to three named hierarchy
levels (Level1, Level2, Level3) carried in `topology.HostInfo`, and a new
multi-phase tree-building algorithm in `pkg/translate` that converts those levels
into contiguous, consistently-padded block groups.

## Background

### NVL576 hardware topology

An NVL576 Scalable Unit (SU) contains 16 NVL36 racks. Each rack holds 9 compute
nodes (36 GPUs via NVLink). The logical grouping is therefore:

```
SU (NVL576)
└── rack-01 … rack-16    (16 racks per SU)
    └── node-01 … node-09 (9 nodes per rack, 36 GPUs)
```

A cluster may have multiple SUs under a common network domain (building, data
center zone, etc.), making the full hierarchy up to four levels deep:
Level1 (datacenter) → Level2 (building/SU group) → Level3 (SU/room) →
Level4 (rack/accelerator domain) → nodes.

### Existing limitation

Before this change Topograph treated each accelerator domain (rack) independently.
`BlockSizes` in `topology.conf` could express two tiers (base block and top
level), but the tree builder had no awareness of intermediate groupings. As a
result:

- Slurm could not differentiate intra-rack placement from intra-SU placement.
- Incremental deployment or maintenance (missing racks) could shift base-block
  positions, breaking Slurm's position-based aggregate inference.
- An NVL576 cluster required manual `topology.conf` authoring to express the
  16-rack SU boundary.

## Goals

- Emit `BlockSizes=9,144` for NVL576 deployments from topology provider data
  alone, without hand-editing `topology.conf`.
- Emit exactly 16 consecutive base-block entries per SU, inserting empty
  `BlockName` placeholder entries for missing or offline racks.
- Carry Level1/Level2/Level3 hierarchy data from providers through the canonical
  `topology.Graph` and into the block topology engine.
- Preserve full backward compatibility: clusters that do not use Level fields
  must produce identical output to the pre-change behavior.

## Non-Goals

- Custom rack naming schemes (e.g., `nvl576_d1_r05`) are a provider
  responsibility and are out of scope for the translate layer.
- Kubernetes label changes for NVL576 topology (separate from the existing
  `network.topology.nvidia.com/accelerator` and `tier-N` labels) are not part
  of this change.
- Changes to any provider (`aws`, `gcp`, `oci`, `netq`, etc.); those providers
  must opt in separately by populating the Level fields.

## Data Model Changes

### `topology.HostInfo`

`pkg/topology/domain.go` — three new string fields:

```go
type HostInfo struct {
    Domain     string // accelerator/NVLink domain name (rack)
    InstanceID string
    HostName   string
    Level1     string // optional: top-level grouping (e.g. datacenter)
    Level2     string // optional: mid-level grouping (e.g. building or SU group)
    Level3     string // optional: lowest named level above the domain (e.g. SU / room)
}
```

Level fields are optional and populated only when the provider or simulation
model has corresponding topology data. Their absence leaves behavior unchanged.

### `DomainMap.GetLevelInfo`

A new method on `DomainMap` (`pkg/topology/domain.go`) returns, for a given
level number (1–4), whether any hosts carry that level's value and a map from
each group name to its sorted list of child names at the level below:

```go
func (m DomainMap) GetLevelInfo(level int) (present bool, members map[string][]string)
```

`level=3` groups hosts by `Level3` value and returns child domain IDs.
`level=2` groups by `Level2` and returns child `Level3` values.
`level=1` groups by `Level1` and returns child `Level2` values.


### Simulation model YAML

`pkg/models/model.go` already reads `network.topology.nvidia.com/level1`,
`network.topology.nvidia.com/level2`, and `network.topology.nvidia.com/level3`
labels from the capacity-block label map and writes them into `HostInfo` when
calling `DomainMap.AddHostInfo`. No structural changes to the YAML schema are
needed; providers assign the labels on their `Instance` objects, and the model
loader propagates them automatically.

## Algorithm: `buildBlockTree`

The tree builder in `pkg/translate/block_tree.go` is refactored into three
explicit phases.

### Phase 1 – Domain packing

`packDomainNodes` splits each accelerator domain's hosts into base blocks of
`baseBlockSize` nodes, pads each domain to a multiple of `groupSize` base
blocks, and wraps the result in one `aggregateBlockNode` per domain. `groupSize`
is the smallest power of two such that `groupSize × baseBlockSize ≥ maxDomainSize`
across all domains.

If every domain's padded capacity already meets `blockSizes[last]`, the flat list
of domain nodes is returned immediately as the tree root's children.

### Phase 2 – Level aggregation (Level3 → Level2 → Level1)

When `HostInfo` carries Level3/Level2/Level1 values, the builder iterates over
levels from closest to the node outward:

```
for level in [3, 2, 1]:
    remaining = getRemainingBlocks(blockSizes, currentCapacity)
    if len(remaining) == 0: break
    present, members = domains.GetLevelInfo(level)
    if not present: break
    desiredGroupSize = blockSizes[last] / currentCapacity
    if desiredGroupSize <= 0: break

    nodesMap  = { node.levelIdentifier() → node  for node in currentNodes }
    levelMap  = { levelName → [nodesMap[child] for child in members[levelName]] }
    packed          = packAggregateNodes(levelMap, completed, desiredGroupSize)
    currentNodes    = packed
    currentCapacity = desiredGroupSize * currentCapacity  // slot capacity, not live-node count
    completed.append(currentCapacity)
```

Key design decisions:

**Fanout from `blockSizes`, not from observed maximum.** `desiredGroupSize` is
`blockSizes[last] / currentCapacity`, not `maxNodes / currentCapacity`. This
ensures every group is padded to the declared block-size boundary. In the NVL576
example, 15 active racks in a room produce `desiredGroupSize = 144/9 = 16`; the
room is padded to 16 rack slots regardless of how many racks are currently
populated.

**Capacity from slot arithmetic.** After packing, `currentCapacity` is updated
to `desiredGroupSize × prevCapacity` — the slot capacity of one packed group. 
Using slot arithmetic ensures the next tier's fanout is computed against the declared block-size boundary.

**Membership from `GetLevelInfo`.** Level names (e.g. `"room-1"`, `"room-2"`)
and their children (sorted rack IDs) come from `GetLevelInfo`. The builder looks
up each child in `nodesMap` by `levelIdentifier()`. Domain nodes carry their
domain ID as their identifier; packed groups carry the level-name key set by
`packAggregateNodes` when it creates the outer aggregate per map key.

**Early exit.** The loop exits as soon as `remaining` becomes empty (blockSizes
satisfied) or a level is absent from the DomainMap (no further hierarchy to
climb).

### Phase 3 – Fallback "root" aggregation

When no Level fields are present in the DomainMap, Phase 2 exits on the first
iteration and all domain nodes are packed under a single `"root"` key, preserving
the pre-change behavior exactly. The same path handles clusters where level
processing exhausts the available levels before satisfying `blockSizes`: the
remaining nodes are packed under `"root"` using `blockSizes[last] / currentCapacity`.

### Empty placeholder handling

`newAggregateBlock` (called by `packAggregateNodes`) pads incomplete groups up
to `desiredGroupSize` using `newEmptyAggregateBlock`. Each empty slot becomes an
empty `baseBlockNode` (no ID, no leaves), which `baseBlockToBlockInfo` converts
to a `blockInfo` with an empty name and no nodes. `toBlockTopology` writes these
as bare `BlockName=blockNNN` entries in `topology.conf` — exactly the placeholder
semantics required by Slurm.

## NVL576 Example

**Topology:** building-1 → room-1 (16 rack switches, 14 with nodes), room-2
(15 rack switches, 15 with nodes). Each rack has 9 nodes. `BlockSizes=[9, 144]`.

**Phase 1:** 29 domain aggregate nodes, each with `nodeCount = 9`.
`domCapacity = 9`, `remaining = [144]`.

**Phase 2 Level3:**
- `GetLevelInfo(3)` → `members = { "room-1": [14 rack IDs], "room-2": [15 rack IDs] }`.
- `desiredGroupSize = 144/9 = 16`.
- room-1: 14 real racks packed into a group of 16 → 2 empty placeholder slots.
- room-2: 15 real racks packed into a group of 16 → 1 empty placeholder slot.
- `currentCapacity = 16 × 9 = 144 = blockSizes[last]`.

**Phase 2 Level2 check:** `remaining = getRemainingBlocks([9,144], 144) = nil` → break.

**Phase 3:** `remaining` is empty → early return; no "root" wrapping tier.

**Output** (32 blocks):

```
# block001=rack-1-01
BlockName=block001 Nodes=node[0001-0009]
...
# block014=rack-1-16
BlockName=block014 Nodes=node[0136-0144]
BlockName=block015          ← empty placeholder (rack-1-03 missing)
BlockName=block016          ← empty placeholder (rack-1-13 missing)
# block017=rack-2-01
BlockName=block017 Nodes=node[0145-0153]
...
# block031=rack-2-16
BlockName=block031 Nodes=node[0280-0288]
BlockName=block032          ← empty placeholder (rack-2-11 missing)
BlockSizes=9,144
```

## Known Limitations

### Non-contiguous level hierarchies

Phase 2 stops at the first absent intermediate level and does not apply any outer
levels above it. `GetLevelInfo(N)` returns `Level{N+1}` values as child names; if
an intermediate level is absent, those child lists are empty and `packAggregateNodes`
would produce all-placeholder groups, so the loop breaks instead of continuing.

As a result, a topology that sets `Level3` and `Level1` but omits `Level2`, or one
that only sets `Level2`, will fall through to Phase 3 root aggregation without the
outer level being applied. A `klog.Warningf` is emitted when data at an outer level
is dropped, so the condition is not silent.

Providers populating Level fields should use contiguous level assignments
(Level3 → Level2 → Level1 without gaps).

### Placeholder placement

Empty placeholder blocks are appended at the end of each level group, not inserted
at their logical position within a sorted name sequence. If a room has 16 racks
(rack-01 through rack-16) and rack-05 is absent, the placeholder appears after
rack-15 rather than in the fifth slot.

The current design has no per-element ordering information; all ordering within a
level is alphabetical by name, and missing entries fill trailing slots. Positional
ordering within groups would require providers to supply an explicit slot index and
a corresponding design update to the tree builder.

## Backward Compatibility

The Level fields in `HostInfo` are zero-valued strings by default. `GetLevelInfo`
returns `present=false` when no host carries a given level value. Phase 2's first
iteration immediately breaks on `!present`, and Phase 3 falls through to the
original single-tier `"root"` aggregation with `desiredGroupSize = blockSizes[last]
/ domCapacity`. All existing tests pass without modification.


## Test Plan

- `TestComplementNVL576` in `pkg/translate/block_complement_test.go`: loads the
  `nvl576.yaml` simulation model, generates block topology with `BlockSizes=[9, 144]`,
  and asserts 32 blocks — 16 per room, with correct node ranges and placeholder
  entries for the 3 missing racks.
- All pre-existing complement tests (`TestComplementMissingBaseBlock`,
  `TestComplementMissingLeafSegment`, `TestComplementKeepsSeparateAccelerators`,
  etc.) continue to pass, verifying backward compatibility.
- `pkg/topology` unit tests cover `GetLevelInfo` across all four level values
  including absent levels.
