# ADR 0001 — Total Refactor for v2.0: Polymorphic Storage, Streaming RDB, Modular Replication, Redis 6.0+ Compatibility

- **Status:** Accepted (2026-05-23; Q-1…Q-5 resolved 2026-05-15, Q-6 absorbed into Q-1, only Q-7 — v1 maintenance window — remains deferred to release-prep)
- **Date:** 2026-05-15 (revised same day)
- **Deciders:** @raniellyferreira (project owner), engineering review pending
- **Tags:** breaking-change, replication, storage, rdb, server, observability, go-1.26
- **Target release:** v2.0.0 published at module path `github.com/raniellyferreira/redis-inmemory-replica/v2`; see §5.1 for migration strategy
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
- **First-class performance.** The library positions itself as a
  *high-performance* in-memory replica (≥ 100 k ops/sec target stated in
  `README.md`). The refactor **must not** trade clarity for regressions on
  the hot paths: RESP parse/write, sharded storage `Get/Set/Del`, RDB
  ingest throughput, and the live command applier loop. Performance is
  governed by D-10 (budgets, methodology, per-phase gates) and is *not*
  deferred to a parallel "performance roadmap".
- The library targets Redis 6.0+ in 2026; legacy ziplist/zipmap support
  adds parser code that no current master will emit.
- Maintenance velocity is impaired by file size (I-1, I-7) and dead surface
  (I-5, I-6).
- The performance roadmap (`ROADMAP.md`) **complements** this ADR — its
  micro-optimisation targets (RESP fast paths, shard hashing, RDB batching,
  Lua cache eviction) feed into the budgets defined in D-10 and are not
  superseded by the refactor.

---

## 2. Decisions

The refactor is structured as **eleven numbered decisions**. Each is independent
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

**Performance non-negotiables for this redesign** (governed by D-10):

- **String fast path is allocation-free on hit.** `StringGet(key) ([]byte, bool)`
  must return the underlying shard's `[]byte` slice directly (no copy, no
  interface boxing) — same as today's `Get`. The `Value` carrying it is a
  small struct on the stack, never returned to the caller.
- **Hot reads never traverse a pointer chain.** `Value` is a struct, not an
  interface; the `Kind` discriminator dispatches inline. Complex-type
  payloads (`*list`, `*hash`, …) are only dereferenced on type-specific
  methods, never on `Get / Exists / TTL / Type`.
- **`Value` size budget: ≤ 32 bytes** on amd64. Discriminator + expiry
  pointer + one payload pointer fit; the legacy `String []byte` field
  (header is 24 B) becomes the only "fat" arm and lives in a union slot
  shared with the typed-payload pointer (tagged-union layout).
- **No new allocations in the live command loop.** Applier dispatch uses
  function-typed `Applier` (D-5), already non-allocating. RESP arg parsing
  reuses a pooled `[][]byte` (sized by command).
- Benchmark gates are defined in D-10 §10.4; P3 will not merge if any of
  them regress.

**Alternatives considered.**

- *Encode complex types as opaque blobs.* Rejected: clients would need to
  re-implement Redis encoding to read them; defeats the purpose.
- *Keep `Set([]byte)` and add parallel typed methods.* Rejected: two ways to
  do the same thing for strings invites bugs and we already pay the cost of
  a major bump.
- *Wait for a "v2 storage" plug-in interface.* Rejected: the RDB handler and
  command-stream executor (D-4, D-5) need this *now*.
- ***`interface { Kind() Kind }` instead of a tagged struct.*** Rejected
  explicitly on performance grounds: every storage hit would allocate an
  interface box and pay an indirect call. Modern-go-development §9 calls
  this out as a hot-path anti-pattern.

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

Each file targets ≤ 350 lines (soft) and is bound by the **700-line hard
cap from D-11**. The public façade (`Replica`, `SyncManager`,
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

### D-10 — Performance as a first-class concern: budgets, methodology, gates

**Decision.** Performance is not a secondary goal of the refactor; it is a
**ship gate**. Every phase has explicit budgets, every hot-path PR carries
a benchstat diff in its description, and any regression > the per-budget
threshold blocks merge unless explicitly waived by the maintainer with
written justification in the PR.

#### 10.1 Hot paths (must never regress)

| Hot path | Definition | Budget (vs. P0 baseline) |
|----------|------------|--------------------------|
| **String `Get` hit** | shard lookup + return existing `[]byte` slice | **0 alloc/op**, ns/op ≤ baseline |
| **String `Set` (no expiry)** | shard write of a new `[]byte` | **≤ 1 alloc/op** (the value copy itself), ns/op ≤ baseline |
| **String `Del` hit** | shard delete | **0 alloc/op**, ns/op ≤ baseline |
| **RESP parse common command** (`GET k`, `SET k v`, `PING`) | byte-slice scan + arg array build | **≤ 2 allocs/op** (the arg slice + one bulk-string copy), ns/op ≤ baseline |
| **RESP write reply** (simple string, integer, bulk string ≤ 256 B) | direct write to `bufio.Writer` | **0 alloc/op**, ns/op ≤ baseline |
| **Live applier dispatch** | registry lookup + applier invocation | **0 alloc/op** on the dispatch itself; applier-internal allocs are command-specific budgets |
| **RDB string-type ingest** | decode + storage write per key | **≤ 1 alloc/op** (the storage copy), measured per-key throughput ≥ baseline |
| **Shard selection** | xxhash mod shard count | **0 alloc/op**, ≤ baseline ns/op |

Baseline is captured in **P0** (`docs/baselines/v2-pre-baseline.txt`).
Comparison is run via `benchstat baseline.txt new.txt`. A "regression" is
a confidence-interval-not-crossed worsening on the *p-value*-adjusted
benchstat output, **not** a single noisy run.

#### 10.2 Warm paths (≤ 5 % regression tolerated)

These paths run on every replication command but are not as latency-
sensitive as the hot paths:

- RDB ingest of complex types (List/Set/Hash/ZSet) — measured as keys/sec.
- Pattern-match `KEYS *` / `SCAN` cursors.
- `INFO` section generation.
- Lua `EVAL` of cached script (`EVALSHA`).
- TLS handshake (replication and server).

#### 10.3 Cold paths (no regression budget)

Connection setup, full-resync handshake, graceful shutdown, error paths.
These run rarely and may grow in cost if it pays for clarity elsewhere.

#### 10.4 Methodology

1. **Baseline first.** P0 captures `-bench=. -benchmem -count=10` across
   all packages with benchmarks. Output goes to
   `docs/baselines/v2-pre-baseline.txt` and is the only reference point
   for the rest of the refactor.
2. **Benchstat in every perf-sensitive PR.** Any PR that touches
   `storage/`, `internal/resp/`, `internal/rdb/`, `internal/cmdapply/`,
   or `internal/replproto/stream.go` **must** paste a benchstat diff in
   the PR description. CI lint enforces this via a check that fails when
   one of these paths changes and the PR body has no
   `<!-- benchstat -->` block.
3. **Escape analysis is non-negotiable on hot paths.** `go build -gcflags=all=-m=2`
   is run on `internal/resp` and `storage/memory.go`; new "escapes to heap"
   lines in `Get` / `Set` / `parseCommand` paths block merge. A test target
   `make escape-analysis` produces the diff.
4. **pprof in CI nightly.** A `bench-pprof` job (added in P0) runs the
   benchmark suite with `-cpuprofile` and `-memprofile`, uploads the
   profiles as artifacts. The PR template asks "did you inspect the
   profile?" for perf PRs.
5. **`-race` always on in CI.** Already true; reaffirmed because the
   sharded-storage and goroutine refactors increase the surface where
   races could appear.
6. **`testing.B.Loop`** (Go 1.24+) is the canonical benchmark loop —
   already in D-9; restated here because consistent loop semantics is a
   precondition for benchstat comparability.

#### 10.5 Specific design rules carried through every phase

These are decisions that bind subsequent phases without being re-negotiated:

- **No interface boxing on hot paths.** `Storage` is an interface only at
  the public boundary; `internal/cmdapply` calls into `*MemoryStorage`
  concretely. `Value` is a struct, not an interface.
- **`sync.Pool` for bounded-size, temporary objects only.** Examples:
  RESP arg slices (sized by command type), RDB read buffers (8 KiB), the
  per-connection response builder in the embedded server. Pools are
  **never** used as caches; pool items are reset on `Get` and must not
  retain state across uses (modern-go-development §9 "Pooling caveats").
- **No `bytes.Buffer` or `strings.Builder` in the hot replication loop.**
  Direct `bufio.Reader` / `bufio.Writer` operations on the wire.
- **Pre-size slices and maps whenever the size is statically known.**
  Applier signatures pass argument counts so appliers can size internal
  state up-front.
- **No reflection on hot paths.** Period.
- **`unsafe.String` is permitted in two places only,** both isolated
  behind a single helper in `internal/resp`: (a) interning a `[]byte`
  that the parser owns into a `string` key for `map[string]Applier`
  lookup, (b) returning a `string` view of a bulk-string payload that
  the caller promises not to mutate. Every other use is rejected in
  review.
- **Struct field ordering** for `Value`, `shard`, and the per-connection
  state in `server/` is locked by a `go vet` `-fieldalignment` check in
  CI.
- **Atomic, not mutex, for read-mostly counters.** Replication offset,
  command counters, sync-completed flag use `sync/atomic`. Shard locks
  remain mutex-based because writes are equally frequent.
- **No `time.After` in loops.** All long-running goroutines reuse a
  single `time.Timer` and `Reset`.
- **Lua script cache** gains an explicit bound (`WithLuaScriptCacheSize`,
  default 128) with LRU eviction. Unbounded cache today is a latent
  memory leak; modern-go-development §9.5 calls this out.

#### 10.6 Per-phase gates

Each phase's exit criteria already required "benchmarks comparable; no
regression > 5 %". D-10 sharpens this:

- **P0** captures the baseline; no gate (there is nothing to compare to).
- **P1** must show **0 % regression** on hot paths (it is a pure move /
  delete). Any regression here is a bug.
- **P2** must show **memory-peak reduction ≥ 40 %** on the 50 MB RDB
  ingest test, and **≤ 0 % regression** on RDB throughput
  (keys-per-second). It's a streaming change; throughput must hold.
- **P3** is the hardest gate: typed `Value` must not regress hot-path
  alloc counts (see D-2 non-negotiables and 10.1 table). String `Get/Set`
  may be re-run against a synthetic 1 M-key workload with `pprof
  --diff_base` to prove no new allocations enter the path. Permitted
  tolerance: ns/op may rise up to **3 %** on string ops because of the
  added `Kind` switch, *only* if `alloc/op` is unchanged. Complex-type
  paths set their own baselines in this phase (no prior comparator).
- **P4** is additive; new code paths have their own budgets (e.g., HFE
  hash decode ≤ 1.5× plain hash decode per element). Existing paths must
  not regress.
- **P5** changes the server transport. Gate: a `go-redis/v9` round-trip
  benchmark (`GET k` with 64 concurrent clients) must show **≤ 5 %
  regression** on latency p50 and **≤ 10 %** on p99 vs the P4 baseline.
  The redcon-vs-native decision is also measured: whichever is faster on
  this benchmark wins the tie-breaker if functional parity is achieved.

#### 10.7 What is *not* in scope

- **SIMD / `simd/archsimd`** (Go 1.26 experimental) — too new, not stable.
- **`unsafe.Pointer` cleverness beyond the two `unsafe.String` cases above.**
- **PGO (Profile-Guided Optimisation).** Worth doing post-v2.0 once the
  hot paths are stable; tracked as a follow-up in `ROADMAP.md`, not
  blocking the refactor.
- **Custom allocator / arena** for the storage shards. Premature; revisit
  if Go 1.27+ ships an arena API.

**Why.** Without explicit budgets and gates, performance becomes whatever
the last PR happened to produce. The library has a published "≥ 100 k
ops/sec" claim and a measured baseline in `ROADMAP.md`; the refactor
either preserves and improves that, or it changes the value proposition
of the library. The non-negotiable framing here makes the trade-offs
visible at PR review instead of post-merge.

**Alternatives considered.**

- *Treat performance as a follow-up roadmap.* Rejected: the typed-`Value`
  change in P3 is exactly the moment when a regression could land and be
  hard to undo. Gating it now is cheaper than removing it later.
- *Looser tolerances (10–15 % on hot paths).* Rejected: hot-path
  regressions compound. A 10 % regression on `Get` is a 10 % regression
  on every read in production.

### D-11 — File size cap: 700 lines for non-test `.go` files (CI-enforced)

**Decision.** **No non-test `.go` file may exceed 700 lines.** The cap is
absolute and CI-enforced. `_test.go` files are exempt because they
legitimately grow with table-driven cases and fixture data; production
code does not get the same indulgence.

**Soft target.** New code under `internal/` and new files in any package
aim for ≤ 350 lines (the target restated in D-4 for the replication
client split). 700 is the *hard* cap; 350 is the *aspiration*. The gap
between 350 and 700 is for files that legitimately host a self-contained
concept (e.g., a complete RESP parser) and where splitting would create
artificial seams.

#### 11.1 Current violators

A baseline scan at the time of writing shows three files over the cap:

| File | Lines | Plan |
|------|-------|------|
| `replication/client.go` | 1 446 | Already planned: split in P1 per D-4 |
| `server/server.go` | 1 140 | Already planned: split in P5 per D-8 |
| `storage/memory.go` | 994 | **New plan:** split in P3a (added to that phase's tasks) |

`e2e_test.go` at 1 634 lines is exempt as a `_test.go` file but is on
the "split when convenient" list — large test files slow down test-run
caching and IDE responsiveness. Not a v2.0 blocker.

#### 11.2 Why 700 and not 500 / 1000

- **500** is too aggressive for files with rich documentation comments
  (RDB parser, RESP types). It would force splits that hurt cohesion.
- **1000** is too loose — `server/server.go` at 1 140 is precisely the
  kind of "everything in one place" file we want to avoid, and 1000
  would not have caught it during the development that produced it.
- **700** matches the modern-Go community heuristic and leaves headroom
  for legitimately large but focused files. It is large enough that we
  expect ≤ 5 % of files to ever brush against it.

#### 11.3 CI enforcement

P0 adds a `make file-size-check` target and a `.github/workflows/lint.yml`
step:

```bash
# fails with exit code 1 if any non-test .go file exceeds 700 lines
find . -name '*.go' -not -name '*_test.go' -not -path './vendor/*' \
  -exec wc -l {} + | awk '$1 > 700 && $2 != "total" { print; bad=1 } END { exit bad }'
```

A second variant warns at 600 lines (yellow zone) so contributors get
early signal before tripping the gate.

#### 11.4 Waiver process

A waiver requires:
1. A `//nolint:filesize // <one-line rationale>` annotation at the top of
   the file.
2. A linked issue explaining the planned split and target removal date.
3. Maintainer approval in the PR.

Waivers are visible (annotations + issue) and time-boxed (target date).
No silent oversize files.

#### 11.5 What is not in scope

- **Function-level line caps.** Go has community conventions (Google
  style guide suggests ~80 lines max per function) but enforcement is
  noisy. Reviewers raise it case-by-case.
- **Package-level file-count caps.** A package may legitimately have
  many small files. No upper bound.
- **Test-file caps.** `_test.go` exempt by rule. Reviewers nudge when a
  test file passes ~2 000 lines but it is not a gate.

**Why.** The single biggest readability win in P1 is splitting
`replication/client.go`. The single biggest review friction we've seen
historically is large files where a reviewer cannot hold the entire flow
in working memory. A hard cap converts a perennial review conversation
into a single CI check.

**Alternatives considered.**

- *Soft target only, no CI gate.* Rejected: soft targets erode. The
  current state of the repo (three files > 700) is evidence.
- *Stricter cap (500).* Rejected for the reasons in §11.2.
- *Exempt generated files.* Not needed today (no `go generate` outputs
  near the cap). If introduced later, the exemption goes in
  `file-size-check` itself, not as a per-file `//nolint`.

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
- `go vet ./...` clean (including `-fieldalignment`).
- **Hot-path budgets met** per D-10 §10.1 — `benchstat` diff against the
  P0 baseline shows **0 % regression on hot paths** and ≤ 5 % on warm
  paths. Cold paths have no budget.
- **Escape-analysis diff inspected** for any change touching `storage/`,
  `internal/resp/`, `internal/rdb/`, `internal/cmdapply/`, or
  `internal/replproto/stream.go` — no new heap escapes on hot-path
  functions.
- **`govulncheck ./...`** clean.
- PR description carries a `<!-- benchstat -->` block whenever a
  perf-sensitive path changed (enforced by a CI check added in P0).
- `CHANGELOG.md` entry added under `## [Unreleased]`.
- ADR updated with any decision changes during the phase.

### 4.2 Phase P0 — Pre-flight (toolchain + performance baseline)

**Goal.** Establish the toolchain baseline so all later phases can rely on
Go 1.26 features without per-PR debate, **and** lock in the performance
baseline that gates every subsequent phase (D-10).

**Tasks.**

1. Bump `go.mod` to `go 1.26` and `toolchain go1.26.3`.
2. `gofmt -s -w .` and `go fix ./...` across the repo (separate commit
   from the bump for review hygiene).
3. **Capture the full performance baseline.** A `make baseline` target:
   - `go test -run=^$ -bench=. -benchmem -count=10 -timeout=30m ./... | tee docs/baselines/v2-pre-baseline.txt`.
   - `go test -run=^$ -bench=. -count=10 -cpuprofile=docs/baselines/v2-pre-cpu.prof -memprofile=docs/baselines/v2-pre-mem.prof ./storage ./protocol ./replication`.
   - `go build -gcflags=all=-m=2 ./storage ./protocol ./replication 2> docs/baselines/v2-pre-escape.txt`.
   - All four artefacts committed under `docs/baselines/`.
4. **Add a CI `bench-regression` job** that runs after the lint/test
   matrix on perf-sensitive PRs. It runs the bench suite under
   `-count=6`, computes `benchstat` against the committed baseline, and
   fails if any hot-path budget from D-10 §10.1 is breached. Workflow
   file: `.github/workflows/bench-regression.yml`.
5. **Add a CI `benchstat-required` lint** that fails when a PR touches
   `storage/**`, `protocol/**`, `replication/**` (and later
   `internal/{resp,rdb,replproto,cmdapply}/**`) without a
   `<!-- benchstat -->` block in its body. Implemented as a tiny
   `actions/github-script` step in the test workflow.
6. **Add `make escape-analysis`** target: `go build -gcflags=all=-m=2`
   on the hot packages, diff against the committed
   `docs/baselines/v2-pre-escape.txt`, fail on any new "escapes to heap"
   line inside a function listed in D-10 §10.1.
7. **Add `go vet -fieldalignment`** to the lint workflow (already in
   golangci-lint v2.x as `fieldalignment`); fix any preexisting
   findings as part of P0 so later phases start clean.
8. **Add `make file-size-check`** (D-11): fail CI if any non-test `.go`
   file exceeds 700 lines; warn at 600. Wired into `.github/workflows/lint.yml`.
   Pre-existing violators (`replication/client.go`, `server/server.go`,
   `storage/memory.go`) are scheduled for split in P1, P5, P3a
   respectively and are allowed to remain over-cap only until their
   phase lands. To unblock P0 itself, the gate is added in **report-only
   mode** in P0 and **switched to blocking** at the start of P1.
9. Verify CI matrix runs on Go 1.26 across all existing jobs.
10. Update `README.md` Go version badge and add a short "Performance
    guarantees" section pointing at D-10.

**Tests added/changed.** None functional. Adds the CI gates above and a
guard test `TestGoModVersion` that fails if `go.mod` regresses below
`go 1.26`.

**Acceptance.**

- CI green on Go 1.26 across all existing jobs.
- `docs/baselines/v2-pre-baseline.txt`, `v2-pre-cpu.prof`,
  `v2-pre-mem.prof`, `v2-pre-escape.txt` committed.
- `bench-regression` workflow runs on a no-op PR and passes (sanity).
- `benchstat-required` lint blocks a synthetic PR that edits
  `storage/memory.go` without a benchstat block (sanity).
- `make escape-analysis` produces zero diff on a clean tree.
- `make file-size-check` runs in **report-only mode**; the three known
  violators (`replication/client.go`, `server/server.go`,
  `storage/memory.go`) are listed in the workflow summary but do not
  fail the run. The gate flips to blocking at the start of P1.

**Risks.**

- *Noisy `go fix` diff.* Mitigation: separate commit so reviewers can
  diff toolchain changes from semantic ones.
- *Benchmark noise on CI runners.* Mitigation: `-count=6` minimum,
  benchstat's p-value adjustment, and a "retry once" policy on
  bench-regression failures before blocking.
- *Pre-existing `fieldalignment` findings.* Mitigation: fix them in P0
  itself; if any are intentional (e.g., padding for false-sharing
  avoidance), annotate with `//nolint:fieldalignment` and a comment.

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
   cleanup, sampling. **Simultaneously split the file** (D-11 — current
   994 lines, must end below 700). Target layout:
   - `storage/memory.go` (≤ 350 lines): orchestrator (`MemoryStorage`
     struct, constructor, shard selection, generic ops `Del`/`Exists`/
     `Type`/`Keys`/`Scan`/`MemoryUsage`/`SelectDB`/`FlushAll`/`Info`).
   - `storage/memory_string.go`: `StringGet`/`StringSet`/`StringSetNX`/
     `StringIncrBy`/`Append` and any string-only helpers.
   - `storage/memory_list.go`, `memory_set.go`, `memory_hash.go`,
     `memory_zset.go`, `memory_stream.go`: one file per `Kind`.
   - `storage/memory_expire.go`: TTL/`Expire`/`PTTL`/`Persist` and the
     sampling cleanup goroutine.
   - `storage/memory_shard.go`: the shard struct, lock layout, and the
     `shards` slice management (kept small to stay readable on the hot
     path).
4. Compile-time check at the package boundary:
   ```go
   var _ Storage = (*MemoryStorage)(nil)
   ```
5. **Rename module path** (per revised Q-1 resolution, §5.1):
   - Edit `go.mod` top line to
     `module github.com/raniellyferreira/redis-inmemory-replica/v2`.
   - Rewrite all internal imports across the tree from
     `github.com/raniellyferreira/redis-inmemory-replica/...` to
     `github.com/raniellyferreira/redis-inmemory-replica/v2/...`.
   - This is one mechanical commit at the head of P3a; the semantic
     storage redesign comes in subsequent commits so the rename diff is
     reviewable in isolation.
   - Update `README.md` install command and CI workflow paths.

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

**Q-1 resolution (revised 2026-05-15 after Codex review):** **rename the
module path to `github.com/raniellyferreira/redis-inmemory-replica/v2`**.
This is the only Go-canonical way to publish a v2+ tag from a repository
that already has a `go.mod` file.

**Why the earlier "+incompatible" strategy was wrong.** Go's module
reference ([go.dev/ref/mod#non-module-compat](https://go.dev/ref/mod#non-module-compat))
restricts `+incompatible` to modules that **do not** have a `go.mod`
file ("not yet aware of module semantics"). This repository *has* a
`go.mod` declaring `module github.com/raniellyferreira/redis-inmemory-replica`,
so a `v2.0.0+incompatible` tag would be rejected by Go's tooling at
release time. The original draft of this section proposed
`+incompatible` and was incorrect; this revision fixes it.

**Concrete consequences of the `/v2` rename:**

1. **`go.mod` top line changes** to
   `module github.com/raniellyferreira/redis-inmemory-replica/v2`.
2. **All internal imports change** from
   `github.com/raniellyferreira/redis-inmemory-replica/...` to
   `github.com/raniellyferreira/redis-inmemory-replica/v2/...`.
   Mechanical sed across the tree; one commit.
3. **v1 users keep working** because v1 tags remain valid at the old path
   (`github.com/raniellyferreira/redis-inmemory-replica@v1.4.x`). They do
   **not** auto-upgrade to v2 on `go get -u` — they must opt in.
4. **v2 users opt in** with:
   ```bash
   go get github.com/raniellyferreira/redis-inmemory-replica/v2@latest
   ```
5. **Both versions can coexist** in a single program if needed (different
   import paths), which gives consumers a soft-migration window.
6. **`MIGRATING.md`** documents the import-line change as step 1, then
   the renamed symbols (Storage methods, removed interfaces, etc.).

**Timing.** The `go.mod` rename and import-path rewrite happen in **P3a**
(the first BREAKING phase), gated behind a single mechanical commit so the
rename is reviewable in isolation from the semantic changes.

**The `release/v1` branch** is cut from the last v1.x.y commit (currently
`v1.4.0`) before P3a lands, so v1 users have a clear maintenance line for
critical fixes. The maintenance window length is tracked as Q-7 in §8.2.

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

### 5.5 Testing and performance obligations

**Functional testing:**

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
- **Race detector** (`go test -race`) is a CI gate for every package that
  starts goroutines.
- **`govulncheck ./...`** runs in CI and blocks merge on any vulnerability
  reachable from our call graph.

**Performance testing (D-10):**

- **`testing.B.Loop` (Go 1.24+)** is the only allowed benchmark loop in new
  code; legacy `for i := 0; i < b.N; i++` benchmarks are migrated
  opportunistically. Mixed forms break benchstat comparability.
- **Benchstat artefacts in every perf-sensitive PR.** PR body carries a
  `<!-- benchstat -->` fenced block; the `benchstat-required` CI lint
  enforces it.
- **`bench-regression` CI job** runs `benchstat` against the committed P0
  baseline and fails if any D-10 §10.1 budget is breached.
- **Escape analysis** (`make escape-analysis`) inspected for every PR
  changing hot-path files. Any new "escapes to heap" line inside a hot-path
  function is a merge blocker.
- **CPU and memory profiles** captured nightly and uploaded as CI
  artefacts via a `bench-pprof` job; PR template asks "did you inspect
  the profile?" for perf PRs.
- **Adversarial workloads.** P3c oracle test fires 1 000 random commands;
  P4a fixtures span 6 Redis versions; P5 e2e benchmarks `go-redis/v9` at
  64 concurrent clients to stress the embedded server.

---

## 6. Consequences

### 6.1 Positive

- The library actually replicates non-string data; this is the primary
  functional gain.
- `replication/client.go` becomes six small files; future contributors can
  read each one in a single sitting.
- **True streaming RDB cuts peak memory roughly in half** for large datasets
  (P2 gate: ≥ 40 % reduction on the 50 MB ingest test).
- **Hot-path budgets are now enforced in CI**, not aspirational. Future
  contributors cannot regress `Get`/`Set` allocations without breaking the
  build. This is a structural quality win that survives the refactor.
- Partial resync removes the "always full sync after a 50 ms blip" tax.
- A typed `Value` opens the door to native handler implementations of `INCR`,
  `LPUSH`, etc., which previously lived only in the master.
- Function-typed appliers (D-5) and concrete `*MemoryStorage` calls inside
  `internal/cmdapply` (D-10 §10.5) keep the live replication loop free of
  interface boxing — an improvement over today's interface-mediated
  dispatch.

### 6.2 Negative

- One-shot major-version break. Every embedder must edit code to upgrade.
- ~3 000 LoC of new tests are required to credibly claim Redis 6.0 – 8.0
  parity.
- Streams support is intentionally minimal at first (raw listpack passthrough
  on read). Full XREAD/XREADGROUP semantics are deferred.
- **The typed `Value` carries up to a 3 % ns/op cost on string ops** vs the
  current byte-slice-only path (the `Kind` switch in the hot path). D-10 §10.6
  caps this at 3 % *only if* alloc/op stays at zero; alloc regressions are
  not permitted. `[Inference]` based on similar tagged-union designs; actual
  numbers come out of the P3 benchstat run.
- **Performance gates raise the bar for every PR.** Contributors must learn
  benchstat and escape analysis. We accept this cost because the library's
  value proposition is performance.
- Larger working set in memory: typed values carry small per-value overhead
  vs. a single `[]byte`. D-10 §10.1 budgets `Value` at ≤ 32 B (vs 24 B for a
  bare `[]byte` header); ~33 % per-value overhead on string-heavy datasets.
  `[Speculation]` aggregate impact depends on average key/value size;
  confirmed by P3 memory benchmarks.

### 6.3 Neutral

- Internal package moves are invisible to embedders but will require
  contributors to learn the new layout.
- `tidwall/redcon` adoption is conditional and reversible; the registry
  refactor lands either way.

---

## 7. Risk register

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| **Hot-path regression sneaks in via the typed `Value` (D-2)** | **Medium** | **High** | D-10 §10.1 budgets are CI-enforced; PR-level benchstat block required; escape-analysis diff checked. P3 will not merge if alloc/op rises on `Get`/`Set`/`Del`. |
| **Hidden allocation introduced by `internal/cmdapply` registry lookup** | Low | Medium | `map[string]Applier` lookup is allocation-free in Go; key is a `string` interned from the RESP byte slice via the single allowed `unsafe.String` helper (D-10 §10.5). Benchstat on the dispatcher in P3c. |
| **`sync.Pool` items retain stale state across uses** | Medium | Medium | Pool items are `Reset()`-checked on `Get`; a `go vet` custom check (added in P0 if practical, otherwise reviewed manually) flags `Put` of dirty items. |
| **Lua script cache unbounded growth** (pre-existing latent bug) | Low | Medium | `WithLuaScriptCacheSize` + LRU eviction in P5 (D-10 §10.5). Default 128; tunable. |
| Stream type semantics drift from Redis 8.0 | Medium | Medium | Pin tests against real `redis-server:8.0` Docker image; degrade gracefully to passthrough if a new opcode appears. |
| Partial-resync file corruption on crash | Low | Low | Atomic temp-file + rename; on parse error, fall back to full sync and log a warning. |
| `tidwall/redcon` cannot replicate AUTH + per-connection SELECT semantics, **or is slower than native** | Medium | Low | POC has explicit functional kill criterion **and** a perf tie-breaker (D-10 §10.6 P5 gate); fallback path (native + registry) is already planned. |
| Migration friction for embedders on v1 | High | Medium | `MIGRATING.md` with code-mod recipes; `release/v1` branch kept for critical bug fixes only (window: Q-7). |
| CI bench-regression false positives from runner noise | Medium | Low | `-count=6` minimum, benchstat p-value adjustment, retry-once policy before blocking. |

---

## 8. Open questions

### 8.1 Resolved (2026-05-15)

- **Q-1 — Module path.** ✅ **Rename to `.../v2`** (revised after Codex
  review identified that `+incompatible` is invalid for modules that
  already have a `go.mod` file). Mechanical sed at the head of P3a; v1
  tags keep working at the old import path. Full strategy in §5.1.
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
- **Q-6 — Exact tag.** ✅ **`v2.0.0`** at module path `.../v2`. (Resolved
  by the same Codex correction that fixed Q-1; the prior deferral was
  premised on `+incompatible`.)

### 8.2 New, deferred

- **Q-7 — `release/v1` maintenance window.** Risk register §7 already lists
  "v1 branch kept for 6 months". Reviewer to confirm 6 months is right,
  given Q-1 means v1 users have to explicitly opt in to `.../v2` and may
  stay on v1 longer than expected.

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
| 2026-05-15 | @raniellyferreira (revised after Codex PR #29 review) | **Q-1 superseded.** Codex correctly pointed out that `v2.0.0+incompatible` is invalid for modules that already have a `go.mod` (see `go.dev/ref/mod#non-module-compat`). Q-1 re-resolved as: **rename to `.../v2`** (Go-canonical). §5.1 rewritten with the `/v2` strategy, v1 maintenance line, and import-rewrite step. P3a task 5 restored to "rename module path". Q-6 (exact tag) resolved by the same correction (now simply `v2.0.0`). Q-7 (v1 maintenance window) remains deferred. |
| 2026-05-15 | @raniellyferreira (perf elevation) | **Added D-10: performance as a first-class concern.** Defines hot/warm/cold path classification, per-budget CI gates (`bench-regression`, `benchstat-required`, escape-analysis), design rules carried through every phase (no interface boxing on hot paths, `Value` as struct not interface, `sync.Pool` only for temporaries, `unsafe.String` limited to two named helpers, struct field alignment vet, atomic for read-mostly counters, no `time.After` in loops, bounded Lua cache). Per-phase gates now have specific numeric budgets (P1: 0 % regression; P2: ≥ 40 % memory reduction on RDB ingest; P3: 0 alloc regression, ≤ 3 % ns/op on string ops; P5: ≤ 5 % latency p50 regression on go-redis round-trip). P0 expanded with `make baseline`, `make escape-analysis`, `bench-regression` and `benchstat-required` CI jobs. D-2 amended with performance non-negotiables for the typed `Value`. §1.3 elevates performance to a primary driver (not orthogonal). §5.5 split into functional and performance obligations. §6.1, §6.2, §7 updated with concrete perf impacts and risks. |
| 2026-05-15 | @raniellyferreira (file-size cap) | **Added D-11: hard cap of 700 lines for non-test `.go` files; `_test.go` exempt.** Soft target 350. CI-enforced via `make file-size-check` added to P0 in report-only mode and flipped to blocking at the start of P1. Pre-existing violators identified: `replication/client.go` (1 446 — split in P1), `server/server.go` (1 140 — split in P5), `storage/memory.go` (994 — split added to P3a tasks with concrete file layout: `memory.go` orchestrator + per-Kind files + `memory_expire.go` + `memory_shard.go`). Waiver process: `//nolint:filesize` annotation + linked issue + maintainer approval + target removal date. D-4 wording aligned: 350 is soft target, 700 is the hard cap from D-11. |
| 2026-05-23 | @raniellyferreira (acceptance) | **Status flipped Proposed → Accepted.** All blocking questions (Q-1…Q-5, Q-6) resolved in earlier entries; only Q-7 (v1 maintenance window) remains deferred to release-prep PR. P0 work may now begin: bump toolchain to Go 1.26.3 (latest stable as of 2026-05-23, supersedes the original `go 1.26` baseline of D-9), establish `make baseline` / `make escape-analysis` / `bench-regression` / `benchstat-required` / `file-size-check` / `fieldalignment` gates. PR #29 (this ADR) merged at acceptance. |
