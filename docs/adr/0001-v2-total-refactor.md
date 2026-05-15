# ADR 0001 — Total Refactor for v2.0: Polymorphic Storage, Streaming RDB, Modular Replication, Redis 6.0+ Compatibility

- **Status:** Proposed (Q-1…Q-5 resolved 2026-05-15; awaiting reviewer sign-off)
- **Date:** 2026-05-15 (revised same day)
- **Deciders:** @raniellyferreira (project owner), engineering review pending
- **Tags:** breaking-change, replication, storage, rdb, server, observability, go-1.26
- **Target release:** next major iteration — see §5.1 for the Go module tag strategy implied by Q-1
- **Toolchain baseline:** Go 1.26 (`go 1.26` in `go.mod`); see D-9
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

The refactor is structured as **nine numbered decisions**. Each is independent
in principle but they are scheduled as five sequential phases (see §4).

### D-1 — Single repository layout, `internal/` for protocol, replication and composition root

**Decision.** Move `protocol/`, `replication/` and a new composition root
under an `internal/` directory tree:

```
internal/
  app/         ← composition root (app.Run(ctx)); see D-4
  resp/        ← was protocol/
  replproto/   ← was replication/ (without the public RDB-handler types)
  rdb/         ← was replication/rdb.go + lzf.go
  cmdapply/    ← command applier registry (see D-5)
  observ/      ← unified Logger / MetricsCollector contracts (removes adapters.go)
  lua/         ← was lua/ (moved per Q-4 resolution; only used by server/)
```

The public surface keeps `storage/`, `server/` and the root package intact.
The root package becomes a thin façade over `internal/app`. The Lua engine
ceases to be public: its only consumer is `server/`'s `EVAL` / `EVALSHA`
handlers, so internal placement gives us freedom to evolve its API without
semver pressure.

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

### D-4 — Modular replication client + composition root

**Decision.** Split `replication/client.go` into focused files and introduce
a composition root in `internal/app` so the root package's `Replica.Start`
becomes a thin call into `app.Run(ctx)`:

| File | Responsibility |
|------|----------------|
| `internal/app/run.go` | `app.Run(ctx)` composition root: load config, build providers, start subsystems, wait for `ctx.Done()`, graceful shutdown in reverse order |
| `internal/app/providers.go` | Provider functions (`provideDialer`, `provideHandshake`, `provideStorage`, `provideServer`) — one constructor per subsystem |
| `internal/replproto/dialer.go` | TCP + TLS dial, timeouts, keepalive |
| `internal/replproto/handshake.go` | PING → AUTH → REPLCONF → PSYNC state machine |
| `internal/replproto/fullsync.go` | RDB framing detection, hand-off to `internal/rdb.Parse` |
| `internal/replproto/stream.go` | Live command loop, REPLCONF GETACK / ACK |
| `internal/replproto/heartbeat.go` | Periodic ACK ticker with shared offset counter |
| `internal/replproto/state.go` | `replicationState` enum (was `int32` plus booleans), shared offsets |

Each file targets ≤ 350 lines. The public façade (`Replica`, `SyncManager`,
`SyncStatus`) remains; only its body shrinks to delegation.

**Why.** Resolves I-1 and lifts the "huge `Replica.Start` body" anti-pattern
(modern-go-development §5). Makes the state machine reviewable, allows the
heartbeat goroutine to be tested in isolation with `testing/synctest`, and
gives a single visible spot (the composition root) where ownership and
shutdown order live.

### D-5 — Command applier with registry, covering all replicated write commands

**Decision.** Introduce `internal/cmdapply` with a registry of
**function-typed appliers** — explicitly *not* a multi-method interface
(modern-go-development §7 "Strategy via Interfaces/Functions" prefers
function types when there is a single behaviour):

```go
type Applier func(ctx context.Context, st storage.Storage, args resp.Args) error

var registry = map[string]Applier{
    "SET":   applySet,
    "DEL":   applyDel,
    "LPUSH": applyLPush,
    // …
}
```

Each applier is a small file (`apply_set.go`, `apply_lpush.go`, …) with
exhaustive table-driven tests. Initial coverage:

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

### D-9 — Go 1.26 toolchain baseline and modern-Go adoption

**Decision.** Bump `go.mod` to `go 1.26` for v2.0 and adopt the
following Go 1.24 → 1.26 features where they directly improve the refactored
code:

| Adoption | Where | Reason |
|----------|-------|--------|
| `go 1.26` directive in `go.mod` | root | Unlocks everything below; still works with 1.25 toolchain via `toolchain` line if needed |
| `go fix` modernizers | repo-wide, P1 prep | Auto-update legacy idioms |
| `errors.AsType[T]` | error checks in `internal/replproto`, `internal/cmdapply` | Type-safe replacement for `errors.As` boilerplate |
| `slog.NewMultiHandler` | `internal/observ` (when user provides custom Logger) | Multi-sink without manual fan-out |
| `new(expr)` initializers | `internal/cmdapply` for optional fields | Less boilerplate for `*X` literals |
| `testing/synctest` (Go 1.25+ GA) | `heartbeat_*_test.go`, applier timeout tests | Deterministic time without `time.Sleep` |
| `testing.B.Loop` | benchmarks (already partially used) | Standardise benchmark iteration |
| `sync.WaitGroup.Go` (Go 1.25+) | replproto fan-out paths | Replace `wg.Add(1) + go func(){ defer wg.Done() }` |
| `runtime/trace.FlightRecorder` | optional `WithFlightRecorder()` option (deferred to v2.1) | Post-incident analysis |
| `os.Root` | not adopted in v2.0 | No filesystem path handling on hot paths |
| Green Tea GC default | runtime-level, free | Already on by default in 1.26 |
| `crypto/rsa` PKCS#1 v1.5 deprecation | not affected | Library does not use RSA |

**Why.** The current `go.mod` declares `go 1.25.2`; a v2.0 break is the
right moment to consolidate on a single modern baseline. Each adoption above
has a concrete payoff in the refactored code, not "use it because it is
new". Items not adopted are listed explicitly so reviewers know they were
considered.

**Alternatives considered.**

- *Stay on `go 1.25`.* Rejected: we lose `errors.AsType[T]` and
  `slog.NewMultiHandler`, both of which clean up code we are about to
  rewrite anyway.
- *Bump to `go 1.26` but skip the modernizers.* Rejected: `go fix`
  modernizers are exactly the kind of small mechanical cleanup that is
  cheap to do at the start of a refactor and expensive to retrofit later.

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

Five sequential phases. Each phase is one or more PRs against the
v2-development branch. Phases land in order; later phases may depend on
earlier phases' API changes.

### 4.1 Phase summary

| Phase | Decisions | Net effect | Public API impact | Est. PR count | Est. LoC delta |
|-------|-----------|------------|-------------------|---------------|----------------|
| **P0** Pre-flight | D-9 (toolchain only) | `go.mod` to 1.26, `go fix` modernizers, baseline benchmarks captured | none | 1 | ±100 |
| **P1** Structural cleanup | D-1, D-4 (split only), removes I-5 / I-6 | `internal/` tree, composition root, single Logger/Metrics, one CleanupConfig | none (internal moves only) | 2 | +1 200 / −800 |
| **P2** Streaming RDB | D-3 | Removes `rdbStreamBuffer`, true streaming, framing wrapper | none | 1 | +250 / −200 |
| **P3** Polymorphic storage + command applier | D-2, D-5 | Typed `Value`, full command coverage, RDB events emit typed payloads | **breaking** — main reason for v2.0 | 3 | +4 000 / −500 |
| **P4** N-version compatibility | D-6, D-7 | RDB types 16–28, partial-resync persistence | additive | 2 | +1 500 / −100 |
| **P5** Server | D-8 | Command registry + (maybe) redcon | none externally | 1 | depends on POC |

**Universal exit criteria for every phase:**

- All tests green: `go test ./...` (including `-race`).
- Lint clean: `golangci-lint run` (project config).
- `go vet ./...` clean.
- Benchmarks comparable: `benchstat` shows no regression > 5 % on baseline
  benchmarks captured in P0; regressions must be justified or fixed.
- `CHANGELOG.md` entry added under `## [Unreleased]`.
- ADR updated with any decision changes during the phase.

### 4.2 Phase P0 — Pre-flight

**Goal.** Establish the toolchain baseline so all later phases can rely on
Go 1.26 features without per-PR debate.

**Tasks.**

1. Bump `go.mod` to `go 1.26` and `toolchain go1.26.3`.
2. `gofmt -s -w .` and `go fix ./...` across the repo.
3. Capture baseline benchmarks: `go test -run=^$ -bench=. -benchmem -count=10 ./... | tee docs/baselines/v2-pre-baseline.txt`.
4. Verify CI matrix runs on Go 1.26.
5. Update `README.md` Go version badge.

**Tests added/changed.** None functional. Add a CI job that fails if
`go.mod` regresses below `go 1.26`.

**Acceptance.**

- CI green on Go 1.26 across all existing jobs.
- `docs/baselines/v2-pre-baseline.txt` committed.
- ADR `[Speculation]` markers near "<5 % regression" in §6.2 can now be
  measured against this baseline.

**Risks.** Low. If `go fix` produces a noisy diff, it is split into a
separate commit so reviewers can diff toolchain changes from semantic
changes.

---

### 4.3 Phase P1 — Structural cleanup

**Goal.** Make the codebase reviewable. No behaviour change. Two PRs:
**P1a** moves files (no logic changes); **P1b** removes dead surface and
introduces the composition root.

#### P1a — Move to `internal/`

**Tasks.**

1. Create `internal/resp/` from `protocol/`. Update all imports.
2. Create `internal/replproto/` from `replication/` (excluding `rdb.go`,
   `lzf.go`). Update imports.
3. Create `internal/rdb/` from `replication/rdb.go` and `replication/lzf.go`.
4. Create `internal/observ/` and move `Logger`, `MetricsCollector`,
   `Field` types from `interfaces.go` and `replication/client.go` into it.
   Delete `adapters.go`.
5. The root package keeps the public `Logger` / `MetricsCollector` aliases
   pointing at `internal/observ` so the public API does not change:
   ```go
   // package redisreplica
   type Logger = observ.Logger
   type MetricsCollector = observ.MetricsCollector
   ```
6. Delete the now-empty `protocol/` and `replication/` directories.

**Tests added/changed.** Existing tests keep passing as-is; only their
`import` lines change.

**Acceptance.**

- `go build ./...` succeeds.
- All existing tests pass.
- `git diff --stat` shows ~95 % renames, ~5 % import-line edits.

**Risks.** Medium for review noise (large diff). Mitigation: separate
commit per directory move; PR description points reviewers at
`git log --follow` for individual files.

#### P1b — Composition root, remove dead surface, single CleanupConfig

**Tasks.**

1. Create `internal/app/run.go` exposing `app.Run(ctx context.Context, cfg Config) error`.
2. Create `internal/app/providers.go` with one provider per subsystem.
3. Refactor root `Replica` so that `Replica.Start(ctx)` is delegation to
   `app.Run`, and `Replica.Close` stops the context.
4. Delete `storage/storage.go` interfaces:
   `TransactionalStorage`, `Transaction`, `StorageObserver`,
   `SnapshotStorage`, `Snapshot`, `MemoryLimitedStorage`,
   `CleanupConfigurableStorage`. Keep `Storage`, `CleanupConfig`.
5. Delete cleanup presets:
   `CleanupConfigSmallDataset`, `CleanupConfigMediumDataset`,
   `CleanupConfigLargeDataset`, `CleanupConfigBestPerformance`,
   `CleanupConfigLowLatency`. Keep `CleanupConfigDefault`.
6. Update `WithCleanup*` options accordingly.
7. Update `MIGRATING.md` skeleton listing what was removed (no replacement
   shim — these had no users).

**Tests added/changed.**

- Existing tests for cleanup presets removed (they pinned the constants
  themselves; with one preset there is nothing to assert).
- New test: `internal/app/run_test.go` — start/stop lifecycle with a fake
  master; uses `testing/synctest` for deterministic timing.

**Acceptance.**

- All existing public-API tests pass unchanged.
- `goimports -l .` produces no output.
- `golangci-lint run` clean.

**Risks.** Low. The deleted interfaces have no implementations; removing
them cannot break a real consumer. CHANGELOG must call this out for users
who imported the type names without using them.

---

### 4.4 Phase P2 — True streaming RDB ingest

**Goal.** Replace `rdbStreamBuffer` so a 1 GB RDB does not require 2 GB of
peak RAM.

**Tasks.**

1. Add `internal/replproto/framing.go` with two `io.Reader` implementations:
   - `lengthPrefixed{r io.Reader, remaining int64}` — reads exactly N bytes
     then signals EOF.
   - `eofMarked{r io.Reader, marker [40]byte, buf []byte}` — reads until
     the trailing marker is observed.
2. Add `internal/replproto/fullsync.go` (split from old `client.go`):
   detects which framing the master used, wraps the socket reader, calls
   `internal/rdb.Parse(framedReader, handler)`.
3. Delete `rdbStreamBuffer` and the buffering logic in
   `replication/client.go:1235–1280`.
4. Add a new test that ingests a 50 MB synthetic RDB while
   `runtime.ReadMemStats` confirms heap stays bounded (< 8 MB above
   baseline).

**Tests added/changed.**

- `internal/rdb/parse_streaming_test.go` — feeds a slow `io.Pipe` into
  `Parse`; asserts it consumes incrementally.
- Existing RDB tests pass without modification.
- New benchstat assertion: peak memory regression < 5 %.

**Acceptance.**

- Memory test shows ≥ 40 % reduction in peak heap during 50 MB RDB ingest.
- All existing replication tests green.
- No new public API.

**Risks.**

- *EOF marker false positive* if the marker bytes appear inside the RDB
  payload. Mitigation: marker is a 40-byte hex string; statistically
  improbable, but the test suite includes a fixture with a near-collision.
- *Slow reader starvation* — bounded buffer must not deadlock if the
  master pauses sending. Mitigation: read deadline propagated from
  `WithReadTimeout`.

---

### 4.5 Phase P3 — Polymorphic storage + command applier (BREAKING)

**Goal.** Make the replica functionally correct: every replicated write
command actually mutates the right typed value.

This is the biggest phase. It is split into **three PRs** that each leave
the tree compiling and tested.

#### P3a — Typed `Value` and `Storage` redesign

**Tasks.**

1. Redesign `storage/value.go`:
   ```go
   type Kind uint8
   const (
       KindString Kind = iota + 1
       KindList
       KindSet
       KindHash
       KindZSet
       KindStream
   )
   type Value struct {
       Kind Kind
       Expiry *time.Time
       // tagged union; only one field non-nil per Kind
       String []byte
       List   *list
       Set    *hashset
       Hash   *hash
       ZSet   *zset
       Stream *stream
   }
   ```
2. Redesign `storage.Storage` interface:
   - String ops: `StringGet`, `StringSet`, `StringSetNX`, `StringIncrBy`, `Append`.
   - Generic ops: `Del`, `Exists`, `Expire`, `TTL`, `PTTL`, `Type`, `Keys`,
     `Scan`, `MemoryUsage`, `SelectDB`, `CurrentDB`, `FlushAll`, `Info`.
   - List, Set, Hash, ZSet, Stream type-specific methods.
3. Reimplement `storage/memory.go` for typed values; preserve sharding,
   cleanup, sampling.
4. Compile-time check at the package boundary:
   ```go
   var _ Storage = (*MemoryStorage)(nil)
   ```
5. **No module-path change** (per Q-1 resolution, §5.1). Internal imports
   keep using `github.com/raniellyferreira/redis-inmemory-replica/internal/...`.
   The breaking nature of P3a is carried by the import-line `MIGRATING.md`
   recipes plus the `+incompatible` tag strategy (Q-6).

**Tests.**

- Rewrite `storage/storage_test.go` with table-driven tests per kind.
- Existing string-only tests survive with `StringGet` / `StringSet`
  renames.
- Property tests: TTL eviction respects `Kind` (hash with 0 fields after
  HDEL is removed; empty list after LPOP is removed; etc.).

**Acceptance.**

- All storage tests pass.
- `go build ./...` compiles every existing call site after the import
  path bump.
- `MIGRATING.md` lists every renamed method with one-line code-mod
  recipe.

#### P3b — RDB handler emits typed events

**Tasks.**

1. `internal/rdb/handler.go` defines:
   ```go
   type Handler interface {
       OnDatabase(db int)
       OnAux(key, value []byte)
       OnResizeDB(dbSize, expiresSize uint32) // optional via RDBResizeHandler today
       OnString(key, value []byte, expiry *time.Time)
       OnList(key string, items [][]byte, expiry *time.Time)
       OnSet(key string, members [][]byte, expiry *time.Time)
       OnHash(key string, fields map[string][]byte, expiry *time.Time)
       OnZSet(key string, members []ZSetMember, expiry *time.Time)
       OnStream(key string, payload StreamPayload, expiry *time.Time)
       OnEnd(crc64 [8]byte)
   }
   ```
2. Update `internal/rdb/parse.go` to decode listpack-encoded hash, zset,
   set; quicklist v2; stream listpacks (raw passthrough, see Q-3).
3. Update `internal/replproto`'s default RDB handler to call typed
   `Storage` methods.

**Tests.**

- Ingest fixture RDBs (from P4 fixtures or hand-crafted minimal RDBs)
  containing each kind; assert storage state.
- Round-trip property test: write known values via `Storage`, dump RDB
  via a small helper, re-ingest, compare.

**Acceptance.**

- All RDB types in §6.0 of D-6 produce non-empty typed values.
- Existing string-only RDB tests still pass.

#### P3c — Command applier registry

**Tasks.**

1. Create `internal/cmdapply/registry.go` with the `Applier` function
   type and a default registry pre-populated with all commands listed in
   D-5.
2. One `apply_<command>.go` file per command (or per family for tightly
   related ones, e.g., `apply_strings.go` for SET/GETSET/INCR/INCRBY).
3. Update `internal/replproto/stream.go` to dispatch via the registry
   instead of the current 4-command switch.
4. Add `WithUnsupportedCommandPolicy(Policy)` option (subject to Q-2).

**Tests.**

- Per-command table-driven tests covering happy path + key edge cases
  (`SET ... NX|XX|EX|PX|EXAT|PXAT|KEEPTTL`, `LPUSH` to non-list error,
  `HINCRBY` overflow, etc.).
- E2E oracle test: spin up `redis-server:7.4` in a container, fire 1 000
  random commands at master, assert replica state matches via
  `MEMORY USAGE` + per-key comparison.

**Acceptance.**

- ≥ 95 % of commands in D-5 covered with at least one happy-path test.
- E2E oracle test passes.
- Unknown command produces `ErrUnsupportedCommand` and is reported via
  metrics; default policy decided per Q-2.

**Risks.**

- Stream commands (`XADD`, `XREADGROUP` semantics on apply) are subtle
  and may need a follow-up PR; raw passthrough storage in P3b reduces
  this risk.
- `INCRBYFLOAT` precision behavior must match Redis exactly; we have a
  fixture-based test.

---

### 4.6 Phase P4 — N-version compatibility

**Goal.** Credibly claim "Redis 6.0 through 8.0" support.

#### P4a — RDB type coverage

**Tasks.**

1. Implement listpack decoder (`internal/rdb/listpack.go`); covers types
   16, 17, 20.
2. Implement quicklist v2 decoder (type 18).
3. Implement stream decoders for types 15, 19, 21, 26, 27 — raw passthrough
   into `KindStream` for now.
4. Implement hash-with-field-TTL decoder (types 24, 25).
5. Implement RDB type 28 (Array, Redis 8.0).
6. Add `make fixtures` target that spins up Redis 6.0, 6.2, 7.0, 7.2, 7.4,
   8.0 in containers, runs a known data-population script, dumps RDB,
   stores under `testdata/rdb/redis-{version}/`.
7. Add fixture-driven test: load each fixture, assert storage matches
   expected keys.

**Tests.**

- One subtest per fixture under `TestRDBFixtures/redis-{version}`.
- Negative test: malformed listpack returns typed error, parser does not
  panic.

**Acceptance.**

- All 6 fixtures parse without error and yield expected storage state.
- CRC64 verified for all fixtures.

#### P4b — Persistent partial-resync state

**Tasks.**

1. Add `WithReplStateFile(path string)` option in root package.
2. Implement `internal/replproto/replstate.go`:
   - `Save(replid string, offset int64) error` — atomic temp + rename.
   - `Load() (replid string, offset int64, ok bool)`.
3. Hook into stream loop: flush every N commands or M ms, whichever
   first. Defaults: N=1000, M=1000.
4. Modify handshake to send `PSYNC <replid> <offset+1>` when state file
   loaded successfully.
5. On `+CONTINUE` response, continue. On `+FULLRESYNC`, discard local
   state and proceed normally.

**Tests.**

- Property test: kill replica connection mid-stream, wait for
  reconnection, assert `+CONTINUE` was received and storage has no key
  drift.
- Crash-recovery test: write state file with synthetic values, restart
  replica with same option, observe `PSYNC` arguments via mock master.
- Corrupted state file: assert fallback to full sync + warning log.

**Acceptance.**

- Restart with valid state → master accepts `+CONTINUE`.
- Restart with corrupt state → fallback to full sync.

---

### 4.7 Phase P5 — Server: registry, optional redcon

**Goal.** Resolve I-7. Make adding/shadowing commands trivial.

**Tasks.**

1. Refactor `server/server.go` around `map[string]CommandHandler`:
   ```go
   type CommandHandler func(c *Conn, cmd resp.Args) error
   ```
   One handler per file (`cmd_get.go`, `cmd_info.go`, …). Replace the
   30-case switch.
2. POC `tidwall/redcon` in a throwaway branch:
   - Verify AUTH (with master_user + password), per-conn `SELECT`,
     RESP2 array writes, graceful shutdown via `srv.Close()`.
   - Verify Lua `EVAL`/`EVALSHA` integration is feasible.
3. Decision gate (recorded as a follow-up note in this ADR):
   - **If POC passes:** replace `server/server.go` connection plumbing
     with redcon; the command registry from step 1 plugs in unchanged.
   - **If POC fails:** keep native server; ship the registry refactor
     only.
4. Update e2e tests to confirm `go-redis/v9` works against the new server
   for all read commands.
5. Prune `examples/` from 11 to 4 (subject to Q-5).

**Tests.**

- E2E with `go-redis/v9` for `GET`, `MGET`, `INFO`, `ROLE`, `KEYS`,
  `SCAN`, `LRANGE`, `HGETALL`, `SMEMBERS`, `ZRANGE`, `EVAL`.
- Per-handler unit tests for command registry.

**Acceptance.**

- Registry refactor lands either way.
- POC outcome documented in this ADR's decision log.
- Examples directory has 4 working examples.

**Risks.**

- redcon AUTH semantics differ from our current implementation
  (specifically, ACL-style `AUTH user pass`). POC must explicitly
  validate this.
- Lua integration may need a custom `redcon.Conn` wrapper.

---

### 4.8 Cross-phase tracking

We maintain a tracking issue per phase on GitHub with a checklist mapping
1:1 to the tasks above. Each PR closes one or more checkboxes. The ADR
itself stays the source of truth for *intent*; issues track *status*.

---

## 5. Compatibility, migration, deprecation

### 5.1 Public API and module versioning strategy

**Q-1 resolution:** keep the same module path
(`github.com/raniellyferreira/redis-inmemory-replica`) — **no `/v2` suffix
will be added**.

This is a deliberate trade-off with consequences that must be understood by
anyone tagging a release:

- **Go's module rule** (`go.dev/ref/mod#major-version-suffixes`): a module
  at v2 or higher *must* either end in a major-version suffix (`/v2`,
  `/v3`, …) **or** be marked `+incompatible`.
- **Implication for this project:** because we are not adopting the suffix,
  the next breaking release must be tagged either:
  - **`v2.0.0+incompatible`** — Go's documented escape hatch for modules
    that pre-date the path-suffix rule. `go get -u` will *not* upgrade
    v1 users automatically; they must opt in with
    `go get github.com/raniellyferreira/redis-inmemory-replica@v2.0.0+incompatible`.
    Recommended path.
  - **A v1.x.y tag with breaking changes** — violates semver and silently
    breaks `go get -u` users. Not recommended.
  - **A v0.x.y reset** — admits public-API instability but throws away
    accumulated trust in v1 tags. Not recommended.
- **PRs in this refactor** will therefore not assume a `/v2` import path;
  files import `github.com/raniellyferreira/redis-inmemory-replica/internal/...`
  as usual.
- `MIGRATING.md` will list every renamed symbol with a one-line code-mod
  recipe **and** open with a banner explaining the `+incompatible` tag and
  the upgrade command.

**Sub-decision deferred:** the exact tag (`v2.0.0+incompatible` vs other)
is recorded as **Q-6** in §8; it does not block any phase before P5.

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
- `WithUnsupportedCommandPolicy(policy Policy)` — three values:
  - **`PolicyMetric`** *(default per Q-2)* — increment
    `unsupported_command_total{command="..."}` counter, log at warn, keep
    streaming. Safe for passive replicas where occasional unknown commands
    must not break replication.
  - `PolicyError` — fail loud: close the replication connection, surface
    `ErrUnsupportedCommand` to the caller. Opt-in for users treating the
    replica as a source of truth.
  - `PolicyDrop` — current silent behaviour, kept only as an opt-in for
    backwards-compat investigation. Documented as discouraged.

### 5.5 Testing obligations

- New table-driven tests for every command in D-5, comparing applied storage
  state against an oracle obtained from a real `redis-server` running in a
  Docker container under `e2e_test.go`.
- RDB v9 – v14 fixtures captured from real `redis-server` 6.0, 6.2, 7.0, 7.2,
  7.4, 8.0 binaries (or the `unstable` branch where applicable). Fixtures
  live under `testdata/rdb/` and are regenerated by a `make fixtures` target.
- Property test for partial resync: kill replica connection mid-stream, wait,
  reconnect, assert `+CONTINUE` was used and no key drift.
- **`testing/synctest` (Go 1.25+ GA)** is required for all new time-based
  concurrent tests (heartbeat, timeouts, applier deadlines). Existing tests
  using `time.Sleep` in `heartbeat_*_test.go` are migrated as part of P1.
- **`errors.AsType[T]` (Go 1.26)** is the preferred form in new code instead
  of the older `var x *T; errors.As(err, &x)` idiom.
- **`testing.B.Loop` (Go 1.24+)** standard form for all benchmarks; the few
  legacy `for i := 0; i < b.N; i++` benchmarks are migrated opportunistically.
- **Race detector** (`go test -race`) is a CI gate for every package that
  starts goroutines.
- **`govulncheck ./...`** runs in CI and blocks merge on any vulnerability
  reachable from our call graph.

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

## 8. Open questions

### 8.1 Resolved (2026-05-15)

- **Q-1 — Module path.** ✅ **Keep same path; do not add `/v2` suffix.**
  Consequences in §5.1. Reviewer must confirm acceptance of the
  `+incompatible` tag strategy before any v2 tag is cut.
- **Q-2 — Default unsupported-command policy.** ✅ **`PolicyMetric`** (count
  + log + continue). See §5.4 for the full enum; `PolicyError` available as
  opt-in for replica-as-source-of-truth use cases.
- **Q-3 — Streams scope.** ✅ **Raw listpack passthrough in v2.0**;
  consumer-group / XREADGROUP semantics deferred to v2.1. See D-6 and
  §6.2. Replication state stays consistent because streams are stored
  byte-for-byte; only *interactive* stream commands on the server side are
  limited.
- **Q-4 — Lua engine placement.** ✅ **Move to `internal/lua`** as part of
  P1. Reflected in D-1 layout.
- **Q-5 — Examples directory.** ✅ **Prune to 4**: `basic`, `monitoring`,
  `lua-demo`, `pattern-matching`. Delete: `cluster`, `database-filtering`,
  `fixes-demo`, `psync-demo`, `rdb-logging-demo`, `replica-lua-demo`,
  `timeout-demo`. Pruning is part of P5.

### 8.2 New, deferred

- **Q-6 — Exact tag for the v2 release.** Recommended: `v2.0.0+incompatible`
  (Go's documented escape hatch). Alternatives in §5.1. Decision can wait
  until P5 nears completion; pin it in the release-prep PR.
- **Q-7 — `release/v1` maintenance window.** Risk register §7 already lists
  "v1 branch kept for 6 months". Reviewer to confirm 6 months is right,
  given Q-1 means v1 users won't get auto-upgraded.

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
| 2026-05-15 | Claude (revised) | Added D-9 (Go 1.26 baseline + modern-Go adoption); expanded §4 with phase-by-phase detail (P0–P5, multi-PR breakdown, per-phase tasks/tests/acceptance/risks); updated D-1 with `internal/app` and `internal/cmdapply` and `internal/observ`; updated D-4 with composition-root pattern; updated D-5 to make function-typed `Applier` explicit; added §5.5 obligations for `testing/synctest`, `errors.AsType[T]`, `testing.B.Loop`, race detector, `govulncheck`. |
| 2026-05-15 | @raniellyferreira (decisions) | Q-1 resolved: keep same module path, no `/v2` suffix (§5.1 expanded with `+incompatible` strategy + sub-question Q-6). Q-2 resolved: `PolicyMetric` default. Q-3 resolved: streams raw passthrough in v2.0, full semantics deferred to v2.1. Q-4 resolved: Lua moves to `internal/lua` (D-1 layout updated). Q-5 resolved: prune examples to 4 (basic, monitoring, lua-demo, pattern-matching) in P5. New deferred questions Q-6 (exact tag) and Q-7 (v1 maintenance window) added in §8.2. |
