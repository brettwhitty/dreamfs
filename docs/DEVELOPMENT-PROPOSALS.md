---
type: OBSOLETE
authority: SUPERSEDED BY .kiro/specs/dreamfs-vision/
review_status: N/A — OBSOLETE
version: 0.1.0
generated_on: 2026-03-12
obsoleted_on: 2026-03-13
origin_persona: Kiro (Code Review)
intent: OBSOLETE — These proposals predate the greenfield vision sessions. The vision spec (.kiro/specs/dreamfs-vision/) supersedes all content here. Do not use this document for planning or implementation.
primary_sources: [pkg/network/network.go, pkg/storage/storage.go, pkg/api/api.go, pkg/metrics/metrics.go, pkg/fileprocessor/fileprocessor.go, cmd/indexer/main.go, go.mod]
release_path: docs/DEVELOPMENT-PROPOSALS.md
related_issues: []
related_sops: []
tags: [OBSOLETE, proposals, engineering, refactoring, correctness, testing]
---

# ⚠️ OBSOLETE — DO NOT USE FOR PLANNING

This document predates the greenfield vision sessions and is superseded by:
- `.kiro/specs/dreamfs-vision/requirements.md`
- `.kiro/specs/dreamfs-vision/design.md`

The proposals below were written against the old codebase before the project vision was clarified. Some ideas were incorporated into the design document; others are no longer relevant. Retained for historical reference only.

---

# DreamFS v2: Development Proposals (OBSOLETE)

Eight engineering proposals to address shortcomings identified during a full repository code review.
Ordered by suggested implementation sequence (dependencies and risk).

---

## Proposal 1: Fix SwarmDelegate Assignment Race in `StartSwarm`

### Severity: Critical (Correctness)

### Problem

In `pkg/network/network.go`, `StartSwarm` calls `memberlist.Create(cfg)` before setting
`cfg.Delegate = d`. The memberlist library starts its internal goroutines immediately upon
creation. Any gossip messages arriving in the window between `Create` and delegate assignment
are handled with a nil delegate and silently dropped. On a busy network with existing peers,
this means the joining node can miss initial state synchronization messages.

### Current Code (Problematic)

```go
func StartSwarm(ps *storage.PersistentStore) (*memberlist.Memberlist, *SwarmDelegate, error) {
    cfg := memberlist.DefaultLocalConfig()
    cfg.Name = utils.HostID
    cfg.BindPort = viper.GetInt("swarmPort")

    // BUG: memberlist starts goroutines here, but delegate is nil
    ml, err := memberlist.Create(cfg)
    if err != nil {
        return nil, nil, fmt.Errorf("failed to create memberlist: %w", err)
    }
    // Delegate assigned AFTER Create — too late for early messages
    d := NewSwarmDelegate(ps, ml)
    cfg.Delegate = d
    // ...
}
```

### Proposed Fix

Set the delegate on the config before calling `memberlist.Create`. Since `NewSwarmDelegate`
currently requires the `*memberlist.Memberlist` reference (for the `NumNodes` callback in
`TransmitLimitedQueue`), split construction into two phases:

```go
func StartSwarm(ps *storage.PersistentStore) (*memberlist.Memberlist, *SwarmDelegate, error) {
    cfg := memberlist.DefaultLocalConfig()
    cfg.Name = utils.HostID
    cfg.BindPort = viper.GetInt("swarmPort")

    // Phase 1: Create delegate shell with storage reference.
    d := &SwarmDelegate{ps: ps}

    // Set delegate BEFORE Create so no early messages are dropped.
    cfg.Delegate = d

    ml, err := memberlist.Create(cfg)
    if err != nil {
        return nil, nil, fmt.Errorf("failed to create memberlist: %w", err)
    }

    // Phase 2: Wire up the broadcast queue now that memberlist is live.
    d.Broadcasts = &memberlist.TransmitLimitedQueue{
        NumNodes:       func() int { return len(ml.Members()) },
        RetransmitMult: 3,
    }

    // ... rest of peer discovery unchanged ...
}
```

Additionally, guard `GetBroadcasts` against the brief window where `Broadcasts` may be nil:

```go
func (d *SwarmDelegate) GetBroadcasts(overhead, limit int) [][]byte {
    if d.Broadcasts == nil {
        return nil
    }
    return d.Broadcasts.GetBroadcasts(overhead, limit)
}
```

### Risk Assessment

Low. The only behavioral change is the ordering of initialization. The nil guard on
`GetBroadcasts` is defensive and handles the brief window between `Create` and broadcast
queue initialization.

### Scope

- `pkg/network/network.go` — `StartSwarm`, `NewSwarmDelegate`, `GetBroadcasts`
- No API changes, no caller changes required.

---

## Proposal 2: Eliminate Default Mux Conflict Between `network.StartHTTPServer` and Gin

### Severity: Medium (Architectural Debt / Latent Bug)

### Problem

`network.StartHTTPServer` registers routes (`/_changes`, `/peerlist`) on Go's
`http.DefaultServeMux`. The Gin-based API server in `pkg/api` uses its own `*gin.Engine`
router. If both servers were started on the same process, the default mux routes would be
orphaned (served on a different listener) or conflict.

Analysis of call sites reveals that `StartHTTPServer` is **never called** from
`cmd/indexer/main.go`. Both the `serve` and `index` commands use `api.NewServer` exclusively.
`StartHTTPServer` is dead code left over from the pre-refactor monolith.

However, the `/_changes` and `/peerlist` endpoints it provided are important for swarm
replication and peer discovery. These need to be properly integrated into the Gin server.

### Proposed Fix

#### Part A: Migrate Legacy Endpoints into Gin API Server

Add the replication and peer list endpoints to `pkg/api/api.go`:

```go
// pkg/api/api.go — add to registerRoutes()

func (s *Server) registerRoutes() {
    // ... existing static file and redirect routes ...

    v1 := s.router.Group("/api/v1")
    {
        // ... existing v1 routes ...
        v1.GET("/changes", s.handleChanges)
        v1.GET("/peerlist", s.handlePeerList)
    }

    // Backward-compatibility aliases at the legacy paths.
    // These ensure existing peers using the old URLs continue to work.
    s.router.GET("/_changes", s.handleChanges)
    s.router.GET("/peerlist", s.handlePeerList)
}

func (s *Server) handleChanges(c *gin.Context) {
    metas, err := s.ps.GetAll()
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, metas)
}

func (s *Server) handlePeerList(c *gin.Context) {
    host := c.ClientIP()
    peerAddr := fmt.Sprintf("%s:%d", host, viper.GetInt("swarmPort"))
    peers := network.AddAndGetPeers(peerAddr)
    c.JSON(http.StatusOK, peers)
}
```

#### Part B: Extract Pure Peer List Logic from HTTP Handler

Refactor `network.HandlePeerList` to separate the peer management logic from the HTTP
transport concern:

```go
// pkg/network/network.go

// AddAndGetPeers adds a peer address if not already present and returns the
// current peer list. Thread-safe.
func AddAndGetPeers(peerAddr string) []string {
    peerListMutex.Lock()
    defer peerListMutex.Unlock()
    found := false
    for _, p := range peerList {
        if p == peerAddr {
            found = true
            break
        }
    }
    if !found {
        peerList = append(peerList, peerAddr)
        log.Printf("Added new peer via HTTP: %s", peerAddr)
    }
    result := make([]string, len(peerList))
    copy(result, peerList)
    return result
}
```

#### Part C: Remove or Deprecate `StartHTTPServer`

Mark `StartHTTPServer` and `HandlePeerList` as deprecated. They can be removed entirely
once the Gin migration is confirmed working.

### Dependency

This proposal benefits from Proposal 7 (Extract Peer List State from Package Globals),
which replaces the package-level `peerList`/`peerListMutex` with a proper `PeerRegistry`
struct. If Proposal 7 lands first, Part B becomes cleaner.

### Risk Assessment

Low-Medium. The backward-compat aliases ensure existing peers using `/_changes` and
`/peerlist` continue to work. The `api.Server` struct may need a reference to swarm
state (or a `PeerRegistry` from Proposal 7).

### Scope

- `pkg/network/network.go` — new `AddAndGetPeers`, deprecate `StartHTTPServer`/`HandlePeerList`
- `pkg/api/api.go` — new handlers, route registration
- `cmd/indexer/main.go` — may need to pass peer registry to `api.NewServer`

---

## Proposal 3: Route Swarm Gossip Writes Through CacheWriter

### Severity: Medium (Performance / Consistency)

### Problem

`SwarmDelegate.NotifyMsg` calls `ps.Put()` directly, creating one BoltDB write transaction
per gossiped message. This has two consequences:

1. **Performance:** Under heavy gossip traffic (e.g., a peer joining a large cluster and
   receiving thousands of metadata records), each individual `Put` acquires BoltDB's
   exclusive write lock, serializing all writes and creating I/O pressure.

2. **Contention:** If the local indexer is also running (writing through `CacheWriter`),
   the direct `Put` calls from gossip compete with the CacheWriter's batched transactions
   for the single write lock.

The same issue applies to `MergeRemoteState`, which calls `ps.Put` in a loop during
full state sync on join.

### Proposed Fix

Give `SwarmDelegate` an optional `*storage.CacheWriter` reference. When present, gossip
writes go through the batch path:

```go
// pkg/network/network.go

type SwarmDelegate struct {
    ps         *storage.PersistentStore
    cw         *storage.CacheWriter              // optional: batched writes
    Broadcasts *memberlist.TransmitLimitedQueue
}

func NewSwarmDelegate(ps *storage.PersistentStore, ml *memberlist.Memberlist, cw *storage.CacheWriter) *SwarmDelegate {
    d := &SwarmDelegate{ps: ps, cw: cw}
    // ... broadcast queue setup ...
    return d
}

func (d *SwarmDelegate) NotifyMsg(msg []byte) {
    var meta metadata.FileMetadata
    if err := json.Unmarshal(msg, &meta); err != nil {
        log.Printf("Swarm: failed to unmarshal metadata: %v", err)
        return
    }
    if d.cw != nil {
        d.cw.Write(meta) // batched path
    } else {
        if err := d.ps.Put(meta); err != nil {
            log.Printf("Swarm: failed to store metadata for %s: %v", meta.FilePath, err)
            return
        }
    }
    log.Printf("Swarm: received metadata for %s", meta.FilePath)
}

func (d *SwarmDelegate) MergeRemoteState(buf []byte, join bool) {
    var metas []metadata.FileMetadata
    if err := json.Unmarshal(buf, &metas); err != nil {
        log.Printf("Swarm: failed to merge remote state: %v", err)
        return
    }
    for _, meta := range metas {
        if d.cw != nil {
            d.cw.Write(meta)
        } else {
            if err := d.ps.Put(meta); err != nil {
                log.Printf("Swarm: failed to merge metadata for %s: %v", meta.FilePath, err)
            }
        }
    }
}
```

### Caller Updates

`StartSwarm` signature gains an optional `cw *storage.CacheWriter` parameter:

```go
func StartSwarm(ps *storage.PersistentStore, cw *storage.CacheWriter) (*memberlist.Memberlist, *SwarmDelegate, error) {
```

In `cmd/indexer/main.go`:
- **index command:** Pass the CacheWriter (created before swarm start).
- **serve command:** Pass nil (or create a CacheWriter for serve mode too, which would
  be beneficial if the serve node receives heavy gossip).

### Risk Assessment

Medium. The CacheWriter must be created before `StartSwarm` is called in the index command.
This requires reordering initialization slightly. The `MergeRemoteState` batching means
a large state merge on join won't be immediately visible in the DB until the next flush,
but this is acceptable given the eventual-consistency model.

### Scope

- `pkg/network/network.go` — `SwarmDelegate`, `NewSwarmDelegate`, `NotifyMsg`, `MergeRemoteState`, `StartSwarm`
- `cmd/indexer/main.go` — initialization order in `indexCmd` and `serveCmd`

---

## Proposal 4: Remove Dead `schollz/progressbar` Dependency

### Severity: Low (Cleanup)

### Problem

`go.mod` lists `github.com/schollz/progressbar/v3 v3.19.0` as a direct dependency. The
refactored codebase uses charmbracelet's `bubbles/progress` exclusively (via `pkg/ui`).
The schollz progressbar was used in the archive-era monolithic `main.go` and
`main_new_full.go` files, which are no longer part of the build.

This adds unnecessary dependency weight and creates confusion about which progress bar
library is canonical.

### Proposed Fix

```bash
# 1. Verify no active source files import it
grep -r "schollz/progressbar" pkg/ cmd/

# 2. If clean, remove from go.mod requires and tidy
#    Edit go.mod to remove the line:
#      github.com/schollz/progressbar/v3 v3.19.0
#    Then run:
go mod tidy
```

If `go mod tidy` doesn't remove it (because an indirect dependency pulls it in), the
line should be moved from `require` to the indirect block, or the explicit `require`
entry should simply be deleted and tidy will handle the rest.

### Risk Assessment

None. Pure cleanup. No code changes.

### Scope

- `go.mod`
- `go.sum` (automatically updated by `go mod tidy`)

---

## Proposal 5: Fix Divide-by-Zero in `metrics.RenderPeerMetricsUI`

### Severity: Medium (Runtime Crash)

### Problem

In `pkg/metrics/metrics.go`, `RenderPeerMetricsUI` computes the cluster average CPU as:

```go
fmt.Sprintf("%.1f", totalCPU/float64(len(peerMetrics)))
```

If the `peerMetrics` map is empty (no peers have reported yet), this divides by zero
and panics.

### Proposed Fix

Add an early return for the empty case and guard the division:

```go
func RenderPeerMetricsUI() {
    peerMetricsMutex.Lock()
    defer peerMetricsMutex.Unlock()

    if len(peerMetrics) == 0 {
        fmt.Println(lipgloss.NewStyle().Bold(true).Render("\nPEER STATUS"))
        fmt.Println("No peers reporting metrics yet.")
        return
    }

    // ... existing column/row setup ...

    avgCPU := 0.0
    if len(peerMetrics) > 0 {
        avgCPU = totalCPU / float64(len(peerMetrics))
    }

    rows = append(rows, table.Row{
        "CLUSTER TOTAL", "",
        fmt.Sprintf("%.1f", avgCPU),
        fmt.Sprintf("%.1f", totalMemory),
        fmt.Sprintf("%.1fMB/s", totalIORead+totalIOWrite),
        fmt.Sprintf("%d", totalFiles),
    })

    // ... rest unchanged ...
}
```

### Risk Assessment

None. Defensive fix only.

### Scope

- `pkg/metrics/metrics.go` — `RenderPeerMetricsUI` only.

---

## Proposal 6: Enforce Single-Writer Discipline on BoltDB

### Severity: Medium (Data Integrity / Correctness)

### Problem

Both `PersistentStore.Put` (single-record, one transaction per call) and
`CacheWriter.flush` (batched, one transaction per batch) can write to BoltDB. BoltDB
allows only one write transaction at a time, so concurrent use causes serialization
delays. More importantly, there is no compile-time or runtime guard preventing accidental
direct writes when a CacheWriter is active.

This means:
- `SwarmDelegate.NotifyMsg` can call `ps.Put` while `CacheWriter.flush` is in a transaction
- Future code could accidentally bypass the CacheWriter
- No error or warning is produced; writes just silently serialize

### Proposed Fix (Option A: Runtime Guard)

Add a batch-mode flag to `PersistentStore` that prevents direct `Put` calls when a
CacheWriter is active:

```go
// pkg/storage/storage.go

type PersistentStore struct {
    db          *bolt.DB
    batchLocked bool       // true when a CacheWriter owns writes
    mu          sync.Mutex // guards batchLocked flag only
}

func (ps *PersistentStore) SetBatchMode(enabled bool) {
    ps.mu.Lock()
    defer ps.mu.Unlock()
    ps.batchLocked = enabled
}

func (ps *PersistentStore) Put(meta metadata.FileMetadata) error {
    ps.mu.Lock()
    if ps.batchLocked {
        ps.mu.Unlock()
        return fmt.Errorf("direct Put disallowed: CacheWriter is active, use CacheWriter.Write instead")
    }
    ps.mu.Unlock()
    // ... existing implementation ...
}
```

Wire it into CacheWriter lifecycle:

```go
func NewCacheWriter(ps *PersistentStore, batchSize int, flushInterval time.Duration) *CacheWriter {
    ps.SetBatchMode(true)
    // ... existing code ...
}

func (cw *CacheWriter) Close() {
    // ... existing shutdown logic ...
    cw.ps.SetBatchMode(false)
}
```

### Alternative (Option B: Stronger Enforcement)

Make `Put` unexported and only expose writes through the CacheWriter interface. This is
a larger refactor and would require Proposal 3 to land first (so gossip writes go through
CacheWriter). Recommended as a follow-up.

### Dependency

This proposal should land after Proposal 3, which routes gossip writes through CacheWriter.
Otherwise, enabling batch mode would break `SwarmDelegate.NotifyMsg` which still calls
`ps.Put` directly.

### Risk Assessment

Low for Option A. The guard is a runtime check (not compile-time), but it catches misuse
early with a clear error message. The mutex protects only the boolean flag, not the
BoltDB transactions themselves, so it adds negligible overhead.

### Scope

- `pkg/storage/storage.go` — `PersistentStore`, `NewCacheWriter`, `CacheWriter.Close`

---

## Proposal 7: Extract Peer List State from Package Globals

### Severity: Low (Code Quality / Testability)

### Problem

`peerList` and `peerListMutex` are package-level globals in `pkg/network/network.go`.
This has several consequences:

1. **Testing:** Unit tests cannot isolate peer list state. Tests that exercise peer
   discovery will pollute each other's state.
2. **Multiple instances:** Cannot run multiple swarm instances in the same process
   (relevant for integration tests).
3. **Coupling:** The Gin API server (Proposal 2) needs to access peer list state,
   creating a cross-package dependency on a global variable.

### Proposed Fix

Create a `PeerRegistry` struct that encapsulates peer list management:

```go
// pkg/network/peers.go (new file)

package network

import (
    "log"
    "sync"
)

// PeerRegistry manages the list of known swarm peers. Thread-safe.
type PeerRegistry struct {
    mu    sync.Mutex
    peers []string
}

// NewPeerRegistry creates an empty peer registry.
func NewPeerRegistry() *PeerRegistry {
    return &PeerRegistry{}
}

// Add adds a peer address if not already present. Returns true if the peer was new.
func (r *PeerRegistry) Add(addr string) bool {
    r.mu.Lock()
    defer r.mu.Unlock()
    for _, p := range r.peers {
        if p == addr {
            return false
        }
    }
    r.peers = append(r.peers, addr)
    log.Printf("Added new peer: %s", addr)
    return true
}

// AddAndList adds a peer address if not present and returns the current list.
func (r *PeerRegistry) AddAndList(addr string) []string {
    r.mu.Lock()
    defer r.mu.Unlock()
    found := false
    for _, p := range r.peers {
        if p == addr {
            found = true
            break
        }
    }
    if !found {
        r.peers = append(r.peers, addr)
        log.Printf("Added new peer: %s", addr)
    }
    result := make([]string, len(r.peers))
    copy(result, r.peers)
    return result
}

// List returns a copy of the current peer list.
func (r *PeerRegistry) List() []string {
    r.mu.Lock()
    defer r.mu.Unlock()
    result := make([]string, len(r.peers))
    copy(result, r.peers)
    return result
}

// Count returns the number of known peers.
func (r *PeerRegistry) Count() int {
    r.mu.Lock()
    defer r.mu.Unlock()
    return len(r.peers)
}
```

### Integration Points

- `SwarmDelegate` or `api.Server` holds a `*PeerRegistry` instance
- `StartSwarm` creates and returns a `*PeerRegistry`
- The Gin API server (Proposal 2) receives the registry and uses it in `handlePeerList`
- Remove the package-level `peerList` and `peerListMutex` variables

### Risk Assessment

Low. Internal refactor with no external API changes. The `PeerRegistry` is a simple,
well-encapsulated struct that's easy to test in isolation.

### Scope

- New file: `pkg/network/peers.go`
- `pkg/network/network.go` — remove globals, update `HandlePeerList` and `StartSwarm`
- `pkg/api/api.go` — `Server` struct gains `*PeerRegistry` field (if Proposal 2 lands)

---

## Proposal 8: Add Test Coverage for `storage`, `network`, `api`, and `metrics`

### Severity: Medium (Quality / Confidence)

### Problem

Only `pkg/fileprocessor` has tests (`fileprocessor_test.go` with 6 test functions).
The remaining packages have zero test coverage:

| Package          | Test Coverage | Risk Level |
|------------------|---------------|------------|
| `pkg/storage`    | None          | High — core data layer |
| `pkg/api`        | None          | Medium — HTTP handlers |
| `pkg/metrics`    | None          | Low — display only |
| `pkg/network`    | None          | Medium — swarm logic |
| `pkg/config`     | None          | Low — thin viper wrapper |
| `pkg/metadata`   | None          | Medium — custom JSON marshaling |
| `pkg/utils`      | None          | Low — utility functions |
| `pkg/ui`         | None          | Low — display only |

### Proposed Test Plan

#### Priority 1: `pkg/storage/storage_test.go` (Highest Value)

The storage layer is the foundation. Bugs here corrupt data silently.

```
TestPersistentStore_PutAndGetAll
    — Round-trip: Put a FileMetadata, GetAll, verify fields match.

TestPersistentStore_PrefixHas_Match
    — Insert a record, verify PrefixHas returns true for a valid prefix.

TestPersistentStore_PrefixHas_NoMatch
    — Verify PrefixHas returns false for a non-existent prefix.

TestCacheWriter_BatchFlush
    — Write exactly batchSize records, verify all are in DB after flush.

TestCacheWriter_TimerFlush
    — Write fewer than batchSize records, wait for flushInterval, verify in DB.

TestCacheWriter_FlushNow
    — Write records, call FlushNow, verify all are immediately in DB.

TestCacheWriter_CloseFlushesRemaining
    — Write records without triggering batch/timer, call Close, verify no data loss.

TestCacheWriter_ConcurrentWrites
    — Launch N goroutines each writing M records, Close, verify N*M records in DB.
```

#### Priority 2: `pkg/metadata/metadata_test.go`

Custom JSON marshaling is a common source of subtle bugs.

```
TestFileMetadata_MarshalJSON_RoundTrip
    — Marshal then Unmarshal, verify all fields including Extra.

TestFileMetadata_ExtraFields_Flattened
    — Verify Extra fields appear at top level in JSON output.

TestFileMetadata_UnmarshalJSON_UnknownFields
    — Unmarshal JSON with extra keys, verify they land in Extra map.

TestFileMetadata_UnmarshalJSON_MissingFields
    — Unmarshal partial JSON, verify zero values for missing fields.
```

#### Priority 3: `pkg/api/api_test.go`

Use `httptest.NewRecorder` with the Gin router for fast, isolated HTTP tests.

```
TestHandleStatus
    — GET /api/v1/status, verify JSON shape (host_id, uptime_sec, etc.).

TestHandleVolumes_EmptyDB
    — GET /api/v1/volumes with empty DB, verify {"volumes": [], "count": 0}.

TestHandleIndexerProgress_Idle
    — GET /api/v1/indexer/progress when idle, verify running=false.

TestHandleIndexerStop_NotRunning
    — POST /api/v1/indexer/stop when idle, verify 409 Conflict.

TestHandleIndexerStop_Running
    — Set indexer running, POST stop, verify 200 and cancellation.
```

#### Priority 4: `pkg/metrics/metrics_test.go`

```
TestCollectLocalMetrics
    — Verify returns non-zero CPU and memory values on any host.

TestRenderPeerMetricsUI_Empty
    — Call with empty peerMetrics map, verify no panic, verify "No peers" message.

TestRenderPeerMetricsUI_WithData
    — Populate peerMetrics, call render, verify table output contains expected hosts.
```

#### Priority 5: `pkg/network/network_test.go`

Lower priority due to integration-heavy nature. Focus on unit-testable components.

```
TestPeerRegistry_AddAndList
    — Add peers, verify list. Add duplicate, verify no duplicate. (After Proposal 7)

TestPeerRegistry_Concurrent
    — Concurrent Add from multiple goroutines, verify no race. (After Proposal 7)

TestDumpDB_JSON
    — Capture stdout, verify valid JSON array output.

TestDumpDB_TSV
    — Capture stdout, verify TSV header and row count.

TestGetLocalIP
    — Verify returns a non-loopback IPv4 address (or 127.0.0.1 as fallback).
```

### Risk Assessment

None. All tests are additive. No existing code is modified.

### Scope

New `_test.go` files in each package directory.

---

## Suggested Implementation Order

Based on dependency chains and risk:

```
 Step │ Proposal │ Description                              │ Risk │ Depends On
──────┼──────────┼──────────────────────────────────────────┼──────┼───────────
  1   │    5     │ Fix metrics divide-by-zero               │ None │ —
  2   │    4     │ Remove dead schollz/progressbar dep      │ None │ —
  3   │    1     │ Fix SwarmDelegate assignment race         │ Low  │ —
  4   │    7     │ Extract PeerRegistry from globals         │ Low  │ —
  5   │    2     │ Migrate legacy HTTP endpoints to Gin      │ Low  │ 7
  6   │    3     │ Route gossip writes through CacheWriter   │ Med  │ 1
  7   │    6     │ Enforce single-writer discipline          │ Low  │ 3
  8   │    8     │ Add test coverage                         │ None │ Parallel*
```

*Test coverage (Proposal 8) can begin in parallel with any of the above. However,
tests for `storage` and `network` are most valuable after Proposals 3, 6, and 7 land,
since those proposals change the interfaces being tested.

### Dependency Graph

```
Proposal 1 (delegate race)
    └──▶ Proposal 3 (gossip through CacheWriter)
              └──▶ Proposal 6 (single-writer discipline)

Proposal 7 (peer registry)
    └──▶ Proposal 2 (Gin migration)

Proposal 4 (dead dep)     — independent
Proposal 5 (div-by-zero)  — independent
Proposal 8 (tests)        — parallel, benefits from 3/6/7
```
