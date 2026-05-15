# ADR 0001 — Total Refactor for v2.0: Polymorphic Storage, Streaming RDB, Modular Replication, Redis 6.0+ Compatibility

- **Status:** Proposed
- **Date:** 2026-05-15
- **Deciders:** @raniellyferreira (project owner), engineering review pending
- **Tags:** breaking-change, replication, storage, rdb, server, observability
- **Target release:** v2.0.0
- **Supersedes:** sections of ROADMAP.md (performance roadmap remains valid as a parallel concern)

> Status legend: Proposed → Accepted → Implemented → Superseded.
> All claims labelled `[Inference]`, `[Speculation]` or `[Unverified]` are not
> directly verified against runtime behavior; they are based on static reading
> of the current codebase and the Redis source at `/tmp/redis-study/redis-unstable`.

---

## 1. Context

`redis-inmemory-replica` is a Go library that connects to a Redis master as a
passive replica, ingests the RDB snapshot, follows the replication stream, and
exposes the in-memory copy via:

1. A direct Go API (`Replica.Storage()`).
2. An optional embedded RESP server (`server/server.go`) that other Redis
   clients (e.g. `go-redis`, GUI tools) can connect to read-only.

After a deep code study (~15 800 LoC across 64 files) and a parallel reading of
the Redis `unstable` branch (`replication.c`, `rdb.c/rdb.h`, `resp_parser.c`),
we identified seven structural issues that motivate a coordinated, breaking
refactor instead of incremental patches.

### 1.1 Issues found

| # | Area | Symptom | Evidence |
|---|------|---------|----------|
| I-1 | Replication client | Single 1 446-line file mixes connection, AUTH, PSYNC, RDB intake, command streaming, heartbeat and the RDB handler. Hard to reason about state transitions or error recovery. | `replication/client.go` |
| I-2 | RDB streaming | `rdbStreamBuffer` materialises the **entire RDB** into RAM before parsing, defeating the streaming parser and roughly doubling peak memory during initial sync. | `replication/client.go:1235–1280`, `:709–774` |
| I-3 | Storage value model | `Storage.Set(key, value []byte, expiry *time.Time)` only models strings. RDB types `1–4, 9–21, 24–28` (List, Set, Hash, ZSet, Stream, Hash-with-field-TTL) are parsed but silently dropped by `rdbStorageHandler`. **[Inference]** Replication of any non-string master is divergent. | `storage/storage.go`, `replication/client.go:1109–1125` |
| I-4 | Command stream executor | Only `SET / DEL / SELECT / PING` are applied from the live replication stream; `INCR, LPUSH, RPUSH, SADD, ZADD, HSET, EXPIRE, …` are logged at debug level and dropped. | `replication/client.go:967–1005` |
| I-5 | Phantom interfaces | `TransactionalStorage`, `SnapshotStorage`, `MemoryLimitedStorage`, `CleanupConfigurableStorage`, `StorageObserver`, `Snapshot`, `Transaction` are declared without any concrete implementation. Classic YAGNI surface. | `storage/storage.go:46–127` |
| I-6 | Configuration sprawl | Six `CleanupConfig*` presets (`Default`, `SmallDataset`, `MediumDataset`, `LargeDataset`, `BestPerformance`, `LowLatency`) with no clear selection rule create paralysis. | `storage/storage.go:131–186` |
| I-7 | Server monolith | `server/server.go` is a 1 140-line `switch` over 30+ commands with no registry; adding or shadowing a command requires editing the dispatcher. | `server/server.go:250–313` |

### 1.2 Compatibility surface today

Reading `replication/rdb.go:15–19`, the parser advertises support for RDB
versions 9 – 12 (Redis 5.0 – 7.0). It does decode many type opcodes but
**hands string-only payloads** to the handler. Stream (`15, 19, 21, 26, 27`),
listpack-only types (`16, 17, 20`), Hash with field-TTL (`24, 25`) and the
Redis 8.0 Array (`28`) are either missing or partially handled.

The Redis source confirms the wire-level requirements:

- Mandatory `REPLCONF capa eof capa psync2`
  (`/tmp/redis-study/redis-unstable/src/replication.c:3198–3205`).
- `REPLCONF capa rdb-channel-repl` is **optional**; the master degrades
  gracefully to single-channel diskless when it is absent.
- `REPLCONF GETACK *` must be answered immediately — `[Inference]` the current
  client appears to do this from the streaming loop, but should be re-verified
  in tests.
- Partial resync (`PSYNC <replid> <offset+1>`) requires the replica to persist
  `master_replid` and `master_repl_offset` across reconnects; today the client
  always sends `PSYNC ? -1`, forcing a full sync even on transient drops.

### 1.3 Drivers

- **Functional correctness** is the dominant concern: a "Redis replica" that
  silently drops every non-string mutation is, in the strict sense, not a
  replica.
- The library positions itself for Redis 6.0+ in 2026; legacy ziplist/zipmap
  support adds parser code that no current master will emit.
- Maintenance velocity is impaired by file size (I-1, I-7) and dead surface
  (I-5, I-6).
- The performance roadmap (`ROADMAP.md`) is largely orthogonal — most of its
  targets (allocations, RESP fast paths, shard hashing) survive this refactor.

---

## 2. Decisions

The refactor is structured as **eight numbered decisions**. Each is independent
in principle but they are scheduled as five sequential phases (see §4).

### D-1 — Single repository layout, internal/ for protocol & replication

**Decision.** Move `protocol/` and `replication/` under a new `internal/`
directory tree:

```
internal/
  resp/        ← was protocol/
  replproto/   ← was replication/ (without the public RDB-handler types)
  rdb/         ← was replication/rdb.go + lzf.go
```

The public surface keeps `storage/`, `server/`, `lua/` and the root package
intact.

**Why.** Today `protocol/` and `replication/` are *de facto* internal but
exported, so any change is theoretically a breaking change. Moving them to
`internal/` clarifies the contract and lets us iterate without semver pressure.

**Alternatives considered.**

- *Keep current layout.* Rejected: ambiguous public surface inhibits future
  refactor.
- *Split into multi-module workspace.* Rejected: overkill for a single library.

### D-2 — Polymorphic `Value` and tiered Storage interface (BREAKING)

**Decision.** Replace the byte-slice-only model with a typed value:

```go
package storage

type Kind uint8
const (
    KindString Kind = iota + 1
    KindList
    KindSet
    KindHash
    KindZSet
    KindStream
)

type Value struct { /* tagged union via Kind + payload pointer */ }
```

The `Storage` interface gains type-specific methods (`StringGet`, `StringSet`,
`ListPush`, `HashSet`, `ZSetAdd`, …). Pure key operations (`Del`, `Exists`,
`Expire`, `TTL`, `Type`, `Keys`, `Scan`, `MemoryUsage`, `SelectDB`) stay as
they are.

The legacy methods `Get(key) ([]byte, bool)` and
`Set(key, value []byte, expiry *time.Time) error` are replaced by
`StringGet` / `StringSet`. **There is no compatibility shim** — call sites
must migrate. A `MIGRATING.md` will document the mapping.

**Why.** Without typed values the library cannot fulfil its name. This is the
single change with the largest functional impact and there is no incremental
path that preserves the byte-slice signature.

**Alternatives considered.**

- *Encode complex types as opaque blobs.* Rejected: clients would need to
  re-implement Redis encoding to read them; defeats the purpose.
- *Keep `Set([]byte)` and add parallel typed methods.* Rejected: two ways to
  do the same thing for strings invites bugs and we already pay the cost of
  a major bump.
- *Wait for a "v2 storage" plug-in interface.* Rejected: the RDB handler and
  command-stream executor (D-4, D-5) need this *now*.

### D-3 — True streaming RDB ingest

**Decision.** Eliminate `rdbStreamBuffer`. `internal/rdb.Parse` consumes the
`io.Reader` directly from the replication socket via a small bounded buffer
(8 KiB default) and emits typed events to the handler. The disk-based
fallback (length-prefixed RDB) and diskless EOF-marked variant are both
handled by a single `framing.Reader` wrapper that hides the framing from the
parser.

**Why.** Resolves I-2; makes peak memory predictable for large datasets.

**Alternatives considered.**

- *Spool RDB to a temp file.* Rejected: violates "in-memory" name and adds
  disk I/O to the hot path.
- *Keep the buffer but bound it.* Rejected: still a regression vs. true
  streaming for >1 GB datasets.

### D-4 — Modular replication client

**Decision.** Split `replication/client.go` into:

| File | Responsibility |
|------|----------------|
| `internal/replproto/dialer.go` | TCP + TLS dial, timeouts, keepalive |
| `internal/replproto/handshake.go` | PING → AUTH → REPLCONF → PSYNC state machine |
| `internal/replproto/fullsync.go` | RDB framing detection, hand-off to `internal/rdb.Parse` |
| `internal/replproto/stream.go` | Live command loop, REPLCONF GETACK / ACK |
| `internal/replproto/heartbeat.go` | Periodic ACK ticker with shared offset counter |
| `internal/replproto/state.go` | `replicationState` enum (was `int32` plus booleans), shared offsets |

Each file ≤ 350 lines. The public façade (`replication.SyncManager`) remains.

**Why.** Resolves I-1; makes the state machine reviewable; allows the
heartbeat goroutine to be tested in isolation.

### D-5 — Command applier with registry, covering all replicated write commands

**Decision.** Introduce `internal/cmdapply` with a registry
`map[string]Applier` where each applier is a small function that translates
a parsed RESP command into one or more `Storage` calls. Initial coverage:

- Strings: `SET, SETEX, PSETEX, SETNX, MSET, MSETNX, APPEND, INCR, INCRBY, INCRBYFLOAT, DECR, DECRBY, GETSET, GETDEL, GETEX`
- Generic: `DEL, UNLINK, EXPIRE, PEXPIRE, EXPIREAT, PEXPIREAT, PERSIST, RENAME, RENAMENX, COPY`
- Lists: `LPUSH, RPUSH, LPOP, RPOP, LREM, LSET, LTRIM, LMPOP, RPOPLPUSH, LMOVE, LINSERT, LPUSHX, RPUSHX`
- Hashes: `HSET, HSETNX, HDEL, HINCRBY, HINCRBYFLOAT, HMSET (alias)`
- Sets: `SADD, SREM, SPOP, SMOVE, SDIFFSTORE, SINTERSTORE, SUNIONSTORE`
- Sorted sets: `ZADD, ZREM, ZINCRBY, ZPOPMIN, ZPOPMAX, ZRANGESTORE, ZUNIONSTORE, ZINTERSTORE, ZDIFFSTORE`
- Streams: `XADD, XDEL, XTRIM, XSETID, XGROUP, XACK, XCLAIM, XAUTOCLAIM`
- DB: `SELECT, FLUSHDB, FLUSHALL, SWAPDB, MOVE`
- Replication / control: `PING, REPLCONF, REPLPING` (idempotent / no-op)

Unknown commands return a typed `ErrUnsupportedCommand` and are surfaced via
the metrics collector so silent dropping is no longer possible.

**Why.** Resolves I-4; makes coverage explicit and testable.

**Alternatives considered.**

- *Re-use `go-redis` command parsing.* Rejected: `go-redis` is a *client*; it
  encodes commands, it does not interpret them. There is nothing to re-use
  here.
- *Reflect the RESP into a generic state apply.* Rejected: each command has
  bespoke semantics (e.g. `SET ... EX|PX|XX|NX|KEEPTTL|EXAT`); a registry of
  small functions is clearer than a giant interpreter.

### D-6 — RDB parser coverage for Redis 6.0 – 8.0

**Decision.** The parser must handle, *correctly emitting typed events*, the
full opcode/type matrix for masters from Redis 6.0 (RDB v9) through Redis 8.0
(RDB v11–14):

- **String types:** `0, 5` and the encoded forms `INT8/16/32, LZF`.
- **Listpack types:** `16 (hash), 17 (zset), 20 (set), 18 (quicklist v2)`.
- **Stream types:** `15, 19, 21, 26, 27` — initial pass may store the raw
  listpack payloads in a `KindStream` value and lazily decode on read.
- **Hash field-TTL types:** `24, 25` (Redis 7.4).
- **Array type:** `28` (Redis 8.0) — store as `KindList` with element
  metadata.
- **Module / function opcodes (`243, 245, 247`):** consume bytes correctly,
  surface as `OnModule` / `OnFunction` events that the default handler
  ignores. They must not crash the parser.
- **Legacy types `1, 2, 3, 4, 9, 10, 11, 12, 13` (pre-listpack):** kept for
  defensive decoding *only* of older RDB files; not exercised against live
  Redis ≥ 6.0 masters.

CRC64 checksum is verified when present; failure becomes a typed error and
does **not** silently truncate the dataset.

**Why.** The current parser is the bottleneck for the "N versions" claim.
Anchoring the matrix in this ADR makes test obligations explicit.

### D-7 — Persistent partial-resync state

**Decision.** Introduce an opt-in `WithReplStateFile(path string)` option.
When set, `master_replid` and `master_repl_offset` are flushed atomically
(temp file + rename) after every batch of N applied commands (default 1000)
or M milliseconds (default 1 000), whichever comes first. On `Replica.Start`
the file is read and used to attempt `PSYNC <replid> <offset+1>`.

When the option is unset, behaviour matches today (always full resync after
restart).

**Why.** Resolves the `+CONTINUE` gap without forcing a file dependency on
embedded users who do not want disk I/O.

**Alternatives considered.**

- *Always persist.* Rejected: surprises callers who chose this library
  precisely to avoid disk.
- *Pluggable persistence interface.* Considered for v2.1; out of scope here
  because nobody has asked for it yet.

### D-8 — Server: command registry, evaluate `tidwall/redcon`

**Decision.** Refactor `server/` around a command registry
(`map[string]Handler`). Then run a focused POC against
`github.com/tidwall/redcon`:

- The POC must demonstrate AUTH, custom command dispatch, RESP2 array writes,
  graceful shutdown, and per-connection state (`SELECT db`).
- If the POC succeeds, replace `server/server.go`'s connection plumbing with
  redcon and keep the command registry on top. Estimated saving: ~700 LoC.
- If the POC fails on any of the requirements above, **keep the native
  server**, but with the new registry.

The decision between "redcon" and "native + registry" is recorded as a
follow-up note in this ADR after Phase 5 completes.

**Why.** Resolves I-7 with a measurable kill criterion. Avoids the trap of
adopting a dependency on faith.

---

## 3. Out of scope

- **Cluster mode.** No `CLUSTER SLOTS`, no slot-aware sharding. Single-master
  passive replica only.
- **AOF parsing.** RDB + replication stream only.
- **Active replication / write-ahead.** The library remains read-only locally
  (write-redirection to master is the existing escape hatch).
- **RESP3 push frames** (Pub/Sub, client tracking). RESP2 only for v2.0.
- **Cluster-aware client side.** Embedders that need cluster failover should
  layer it above this library.
- **Performance roadmap items** (`ROADMAP.md`) — they continue independently
  and will benefit from the cleaner internal layout.

---

## 4. Implementation plan

Five sequential phases, each ending in a green test suite and a commit.

| Phase | Decisions | Net effect | Public API impact |
|-------|-----------|------------|-------------------|
| **P1** Structural cleanup | D-1, D-4 (split only), removes I-5 / I-6 | `internal/` tree, single Logger/Metrics, one CleanupConfig | none (internal moves only) |
| **P2** Streaming RDB | D-3 | Removes `rdbStreamBuffer`, true streaming | none |
| **P3** Polymorphic storage + command applier | D-2, D-5 (and D-4 finished) | Typed `Value`, full command coverage, RDB events emit typed payloads | **breaking** — main reason for v2.0 |
| **P4** N-version compatibility | D-6, D-7 | RDB types 16–28, partial resync persistence | additive |
| **P5** Server | D-8 | Command registry + (maybe) redcon | none externally |

Each phase ends with: tests green, `go vet` clean, `golangci-lint` clean,
benchmarks comparable (no regression > 5 %), `CHANGELOG.md` updated.

---

## 5. Compatibility, migration, deprecation

### 5.1 Public API

- v1 → v2 is a hard break. We bump the module path **only** if the standard
  Go semver-suffix rule applies (`/v2`). Default plan: yes, rename module to
  `github.com/raniellyferreira/redis-inmemory-replica/v2`.
- `MIGRATING.md` will list every renamed symbol with a one-line code mapping.

### 5.2 Storage interface

Old:

```go
v, ok := s.Get("k")
_ = s.Set("k", []byte("v"), nil)
```

New:

```go
v, ok := s.StringGet("k")
_ = s.StringSet("k", []byte("v"), storage.SetOpts{})
```

`Get`/`Set` are removed. There is no shim. Compilation breaks on first use,
which we consider a feature for a major version.

### 5.3 Wire protocol

No change. Masters and clients see the same RESP2 conversation. The only
observable difference is that complex data types now actually replicate.

### 5.4 Configuration

Removed:

- `CleanupConfigSmallDataset, CleanupConfigMediumDataset, CleanupConfigLargeDataset, CleanupConfigBestPerformance, CleanupConfigLowLatency`

Kept:

- `CleanupConfigDefault` (single preset, all fields tunable).

Added:

- `WithReplStateFile(path string)` (D-7).
- `WithUnsupportedCommandPolicy(policy Policy)` — `PolicyDrop` (current
  silent behaviour, default), `PolicyError` (close replication and bubble
  up), `PolicyMetric` (count and continue).

### 5.5 Testing obligations

- New table-driven tests for every command in D-5, comparing applied storage
  state against an oracle obtained from a real `redis-server` running in a
  Docker container under `e2e_test.go`.
- RDB v9 – v14 fixtures captured from real `redis-server` 6.0, 6.2, 7.0, 7.2,
  7.4, 8.0 binaries (or the `unstable` branch where applicable). Fixtures
  live under `testdata/rdb/` and are regenerated by a `make fixtures` target.
- Property test for partial resync: kill replica connection mid-stream, wait,
  reconnect, assert `+CONTINUE` was used and no key drift.

---

## 6. Consequences

### 6.1 Positive

- The library actually replicates non-string data; this is the primary
  functional gain.
- `replication/client.go` becomes six small files; future contributors can
  read each one in a single sitting.
- True streaming RDB cuts peak memory roughly in half for large datasets.
- Partial resync removes the "always full sync after a 50 ms blip" tax.
- A typed `Value` opens the door to native handler implementations of `INCR`,
  `LPUSH`, etc., which previously lived only in the master.

### 6.2 Negative

- One-shot major-version break. Every embedder must edit code to upgrade.
- ~3 000 LoC of new tests are required to credibly claim Redis 6.0 – 8.0
  parity.
- Streams support is intentionally minimal at first (raw listpack passthrough
  on read). Full XREAD/XREADGROUP semantics are deferred.
- Larger working set in memory: typed values carry small per-value overhead
  vs. a single `[]byte`; estimated < 5 % on string-heavy workloads.
  `[Speculation]` confirmation requires benchmarks after P3.

### 6.3 Neutral

- Internal package moves are invisible to embedders but will require
  contributors to learn the new layout.
- `tidwall/redcon` adoption is conditional and reversible; the registry
  refactor lands either way.

---

## 7. Risk register

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Stream type semantics drift from Redis 8.0 | Medium | Medium | Pin tests against real `redis-server:8.0` Docker image; degrade gracefully to passthrough if a new opcode appears. |
| Partial-resync file corruption on crash | Low | Low | Atomic temp-file + rename; on parse error, fall back to full sync and log a warning. |
| `tidwall/redcon` cannot replicate AUTH + per-connection SELECT semantics | Medium | Low | POC has explicit kill criterion; fallback path (native + registry) is already planned. |
| Migration friction for embedders on v1 | High | Medium | `MIGRATING.md` with code-mod recipes; v1 branch kept on `release/v1` with critical bug fixes only for 6 months. |
| Performance regression from typed Value | Medium | Low | Benchstat before/after on the existing benchmark suite; allow ≤ 5 % regression on string ops, target 0 % on key ops. |

---

## 8. Open questions to confirm before implementation

- **Q-1.** Is the `/v2` module-path rename acceptable, or do we keep the path
  and rely on tag-only versioning? (Recommended: rename.)
- **Q-2.** Should `WithUnsupportedCommandPolicy` default to `PolicyMetric`
  (count + log + continue) or `PolicyError` (fail loud)? (Recommended:
  `PolicyMetric` for a passive replica; opting into `PolicyError` for users
  who treat the replica as a source of truth.)
- **Q-3.** Stream support — is "raw listpack passthrough on read" acceptable
  for v2.0, with full XREADGROUP scheduling deferred to v2.1?
- **Q-4.** Do we keep the Lua engine in the same package or move it under
  `internal/lua` since the public API is `EVAL`/`EVALSHA` via the embedded
  server only?
- **Q-5.** Examples directory has 11 entries. Recommend pruning to 4 (basic,
  monitoring, lua, pattern-matching) to reduce maintenance — confirm.

---

## 9. References

- Redis source (read at `/tmp/redis-study/redis-unstable`):
  - `src/replication.c:2825–3300` — handshake state machine
  - `src/replication.c:1407–1650, 4472–4490` — REPLCONF / GETACK
  - `src/rdb.h:19–109` — opcodes and type table
  - `src/rdb.c:1880–1950, 3200–3800` — header and `rdbLoadObject`
- Current code (read in full):
  - `replica.go`, `options.go`, `interfaces.go`, `adapters.go`
  - `protocol/{reader,writer,types}.go`
  - `replication/{client,sync,rdb,lzf}.go`
  - `server/server.go`
  - `storage/{storage,memory,value,matching}.go`
- Related documents:
  - `ROADMAP.md` — performance roadmap (parallel concern)
  - `docs/redis-7x-compatibility.md` — current 7.x notes (will be folded into
    a single `docs/compatibility.md` after P4)

---

## 10. Decision log

| Date | Author | Change |
|------|--------|--------|
| 2026-05-15 | Claude (drafted) | Initial proposal, status = Proposed. |
