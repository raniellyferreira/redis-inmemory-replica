---
name: modern-go-development
description: "Deep, practical, opinionated skill for modern Go development, architecture, and code review. Covers Go 1.26 (latest stable as of 2026-05-11), idiomatic principles, enterprise project structure, dependency injection, design patterns, anti-patterns, high-performance/low-latency, concurrency, error handling & resilience, observability, security, testing, and a complete code-review checklist. Distinguishes official Go guidance from community conventions.
argument-hint: "Go source file path or code snippet to review, or topic area (e.g. 'concurrency', 'di', 'performance', 'anti-patterns', 'project-structure', 'error-handling')"
---

# Modern Go Enterprise Development & Code Review Skill

**Latest stable Go:** 1.26.3 (released 2026-05-07) — verified from [go.dev/dl](https://go.dev/dl/) accessed 2026-05-11.
**Major release baseline:** Go 1.26.0 (released 2026-02-10) — verified from [go.dev/doc/devel/release](https://go.dev/doc/devel/release) accessed 2026-05-11.
**Previous stable branch:** Go 1.25.10 (released 2026-05-07).

> According to the official Go download page ([go.dev/dl](https://go.dev/dl/), accessed 2026-05-11), the current stable release line is Go 1.26 and the latest point release is `go1.26.3`. The Go 1.26 release notes date the major release to 2026-02-10 ([go.dev/doc/go1.26](https://go.dev/doc/go1.26)).

---

## Table of Contents

1. [Skill Purpose and Usage](#1-skill-purpose-and-usage)
2. [Latest Go Version and Recent Evolution](#2-latest-go-version-and-recent-evolution)
3. [Idiomatic Modern Go Principles](#3-idiomatic-modern-go-principles)
4. [Enterprise-Grade Project Structure](#4-enterprise-grade-project-structure)
5. [Reducing Massive `main.go` Bootstrap](#5-reducing-massive-maingo-bootstrap)
6. [Dependency Injection in Go](#6-dependency-injection-in-go)
7. [Patterns Actually Useful in Go](#7-patterns-actually-useful-in-go)
8. [Anti-Patterns](#8-anti-patterns)
9. [High-Performance and Low-Latency Go](#9-high-performance-and-low-latency-go)
10. [Concurrency Review Guide](#10-concurrency-review-guide)
11. [Error Handling and Resilience](#11-error-handling-and-resilience)
12. [Observability and Operations](#12-observability-and-operations)
13. [Security and Supply Chain](#13-security-and-supply-chain)
14. [Testing Strategy](#14-testing-strategy)
15. [Code Review Checklist](#15-code-review-checklist)
16. [Skill Behavior Instructions](#16-skill-behavior-instructions)
17. [References](#17-references)

---

## 1. Skill Purpose and Usage

### How an LLM / Code Reviewer Should Use This Skill

This skill is designed for LLM-assisted code review and Go development guidance. When using this skill:

1. **Identify the scope**: Determine whether the review is about correctness, architecture, performance, security, concurrency, or a combination.
2. **Apply the checklist** (Section 15) systematically but proportionally — not every item needs deep scrutiny on every change.
3. **Distinguish official from community**: Advice labeled as official comes from Go's own docs and release notes. Advice labeled `[Community convention]` comes from widely-adopted community style guides (Uber, Google, etc.) and should be applied with judgment, not as law.
4. **Mark uncertainty**: If a claim cannot be verified from official sources, label it `[Unverified]`.
5. **Request context**: When the code under review lacks surrounding context (e.g., a diff without the struct definition), request it rather than assume.

### Review Posture

The reviewer should favor:

- **Idiomatic Go**: Code that looks like Go, not a translation from Java, Python, or C++. Prefer the Go way even when it differs from other ecosystems.
- **Simplicity**: The simplest correct solution. Reject cleverness that obscures intent without measurable benefit.
- **Explicitness**: Dependencies, lifecycle, concurrency ownership, and error paths should be visible at the call site. Reject hidden magic.
- **Performance awareness**: Measure first; optimize second. Require benchmarks/profiles for performance claims.
- **Maintainability**: Code is read far more often than written. Favor readability, small packages, and bounded complexity.
- **Enterprise readiness**: Expect production concerns — observability, graceful shutdown, secret management, resilience, migration safety — to be addressed deliberately.

---

## 2. Latest Go Version and Recent Evolution

### Version Baseline

| Version | Release Date | Status |
|---------|-------------|--------|
| **Go 1.26.3** | **2026-05-07** | **Latest stable** |
| Go 1.26.0 | 2026-02-10 | Current major release |
| Go 1.25.10 | 2026-05-07 | Previous stable branch |
| Go 1.24.x | February 2025 | Supported (prior major) |

Source: [go.dev/dl](https://go.dev/dl/), accessed 2026-05-11.

**Reviewer rule:** If a PR depends on Go 1.26 behavior, confirm `go.mod` has `go 1.26` and CI uses the expected toolchain.

```go
module example.com/service

go 1.26
```

### Go 1.26 — Key Features for Reviewers and Developers

Source: [go.dev/doc/go1.26](https://go.dev/doc/go1.26), accessed 2026-05-11.

| Area | Feature | Review Implication |
|------|---------|-------------------|
| Language | `new(expr)` — initializers with `new` | New idiom for optional pointer fields (e.g., protobuf, JSON); review for readability |
| Language | Self-referential generic constraints (`type Adder[A Adder[A]]`) | Enables new generic patterns; review for clarity and necessity |
| Runtime | Green Tea GC now **default** | 10–40% GC overhead reduction; verify latency profiles after upgrade |
| Runtime | ~30% faster cgo calls | Review cgo boundary code for correctness; performance improvement is free |
| Runtime | Heap base address randomization | Security improvement; unlikely to affect correctness |
| Runtime | Goroutine leak profile (experimental: `GOEXPERIMENT=goroutineleakprofile`) | Enable in staging to detect leaked goroutines via GC reachability |
| Tooling | Revamped `go fix` with modernizers | Run on codebase to auto-update to latest idioms |
| Crypto | `random` parameter ignored across all crypto packages | Update tests that pass custom `rand`; use `testing/cryptotest.SetGlobalRandom` |
| Crypto | `crypto/rsa` PKCS#1 v1.5 **deprecated** | Plan migration from `EncryptPKCS1v15`/`DecryptPKCS1v15` |
| Crypto | Post-quantum TLS default-on (`SecP256r1MLKEM768`, `SecP384r1MLKEM1024`) | Review TLS compatibility with legacy clients |
| Stdlib | `errors.AsType[T]` — generic, type-safe alternative to `As` | Prefer in new code |
| Stdlib | `log/slog.NewMultiHandler` | For multi-sink logging |
| Stdlib | `io.ReadAll` ~2x faster, ~half memory | Free performance improvement |
| Net/http | `ServeMux` trailing-slash redirects use **307** (was 301) | May affect clients caching redirects |
| Net/http | `ReverseProxy.Director` **deprecated** | Migrate to `Rewrite` hook |
| Experimental | `simd/archsimd` (amd64) | Not production-ready; review with caution |
| Experimental | `runtime/secret` | Securely erasing temporaries; amd64/arm64 Linux only |

### Go 1.25 — Key Features

Source: [go.dev/doc/go1.25](https://go.dev/doc/go1.25), accessed 2026-05-11.

| Area | Feature |
|------|---------|
| Runtime | Container-aware GOMAXPROCS — Kubernetes CPU limits respected automatically |
| Runtime | Green Tea GC experimental (`GOEXPERIMENT=greenteagc`) — 10–40% GC overhead reduction |
| Runtime | Trace Flight Recorder (`runtime/trace.FlightRecorder`) |
| Compiler | nil pointer bug fix — code using results before error checks may now correctly panic |
| Stdlib | `testing/synctest` graduated to GA — virtualized-time concurrent testing |
| Stdlib | Experimental `encoding/json/v2` and `encoding/json/jsontext` |
| Stdlib | `net/http.CrossOriginProtection` — CSRF using Fetch metadata |
| Stdlib | `sync.WaitGroup.Go` method |
| Tooling | `go.mod ignore` directive |
| Vet | `waitgroup` analyzer — flags misplaced `sync.WaitGroup.Add` calls |
| Vet | `hostport` analyzer — flags `fmt.Sprintf("%s:%d", host, port)` for `net.Dial` |

### Go 1.24 — Key Features

Source: [go.dev/doc/go1.24](https://go.dev/doc/go1.24), accessed 2026-05-11.

| Area | Feature |
|------|---------|
| Language | Generic type aliases fully supported |
| Tooling | `tool` directives in `go.mod`; `go get -tool`; `go tool` cached execution |
| Stdlib | `os.Root` — directory-limited filesystem access |
| Stdlib | `weak` package — weak pointers |
| Stdlib | `runtime.AddCleanup` — improved finalization (prefer over `SetFinalizer`) |
| Stdlib | `crypto/mlkem` (post-quantum), `crypto/hkdf`, `crypto/pbkdf2`, `crypto/sha3` |
| Stdlib | FIPS 140-3 compliance mechanisms (`GOFIPS140`) |
| Stdlib | Experimental `testing/synctest` |
| Stdlib | `testing.B.Loop` — benchmark iteration method |
| Stdlib | `encoding/json`: `omitzero` struct tag |
| Runtime | Swiss Table map implementation; 2–3% CPU overhead reduction |
| Crypto | `crypto/rand.Read` guaranteed not to fail; `crypto/rand.Text` |
| Vet | `tests` analyzer — flags malformed test declarations |

### Go 1.23 — Key Features

Source: [go.dev/doc/go1.23](https://go.dev/doc/go1.23), accessed 2026-05-11.

| Area | Feature |
|------|---------|
| Language | Range-over-function iterators; `iter` package |
| Stdlib | `unique` package for interning/hash-consing |
| Stdlib | `slices` and `maps` iterator helpers (`All`, `Values`, `Backward`, `Collect`, etc.) |
| Stdlib | Timer/Ticker semantic changes (unbuffered channels, GC eligibility) |
| Tooling | `godebug` directive in `go.mod`/`go.work` |
| Compiler | PGO overhead reduced from 100%+ to single-digit percentages |
| Crypto | 3DES removed from TLS defaults; X25519Kyber768 post-quantum enabled |

---

## 3. Idiomatic Modern Go Principles

### Official Baseline

These are drawn from **Effective Go** ([go.dev/doc/effective_go](https://go.dev/doc/effective_go), official but not actively updated, accessed 2026-05-11) and **Code Review Comments** ([go.dev/wiki/CodeReviewComments](https://go.dev/wiki/CodeReviewComments), accessed 2026-05-11).

#### Simplicity and Explicitness

- **Don't translate from other languages** — think in Go. Go's design prioritizes simplicity over expressiveness.
- **Omit unnecessary `else`** — let success flow down the page; handle errors first and return early.
- **Prefer synchronous functions** — CodeReviewComments §29: "Prefer synchronous functions that return results directly over asynchronous ones. Synchronous functions keep goroutines localized, are easier to reason about and test. Callers can add concurrency easily; it's hard to remove unnecessary concurrency at the caller side."
- **Avoid `panic` for normal error handling** — return `error`; use `panic` only for truly unrecoverable programmer errors.

#### Naming

- Short, evocative names for locals; more descriptive names for package-level symbols.
- **No `Get` prefix** in getters: `Name()` not `GetName()`.
- **`-er` suffix** for one-method interfaces: `Reader`, `Writer`, `Stringer`.
- **MixedCaps**, not underscores: `maxLength` not `max_length` or `MaxLength` for unexported.
- **Initialisms are consistent**: `ServeHTTP` not `ServeHttp`, `appID` not `appId`.
- **Package names**: lowercase, single word, no `util`, `common`, `misc`, `api`, `types`, `interfaces`.

#### Zero Values

- Zero values are useful. Don't over-initialize.
- `var s []string` is a valid nil slice; prefer over `s := []string{}` (CodeReviewComments §6).
- Exception: JSON encoding where nil encodes to `null` vs `[]` for empty slice.

#### Small Interfaces and Consumer-Side Interfaces

- CodeReviewComments §19: "Interfaces belong in the package that *uses* them, not the package that *implements* them."
- Don't define interfaces "for mocking" on the implementor side.
- Don't define interfaces before they are used.
- Return concrete types; let consumers define their own interfaces.

```go
// Bad: producer-side interface
package postgres
type UserRepository interface {
		FindByID(ctx context.Context, id string) (User, error)
		Save(ctx context.Context, u User) error
		Delete(ctx context.Context, id string) error
		Migrate(ctx context.Context) error  // Not relevant to most consumers
}

// Good: consumer-side interface — only what this package needs
package billing
type UserFinder interface {
		FindByID(ctx context.Context, id string) (User, error)
}
```

#### Readable Errors

- Error strings should not be capitalized or end with punctuation (CodeReviewComments §9).
- Wrap errors with context: `fmt.Errorf("load user %s: %w", id, err)`.
- Handle errors once: don't log and return the same error.

#### Composition Over Inheritance

- Go has no class inheritance. Compose behavior with structs, interfaces, and embedding.
- Embedding is not subclassing — the receiver of an embedded method is the inner type, not the outer.

#### Table-Driven Tests

- CodeReviewComments §30: Use table-driven tests to reduce boilerplate and improve clarity.
- Use `t.Run` for sub-test isolation and clear failure names.

```go
func TestParse(t *testing.T) {
		tests := []struct {
				input string
				want  int
		}{
				{"1", 1},
				{"0", 0},
				{"-1", -1},
		}
		for _, tt := range tests {
				t.Run(tt.input, func(t *testing.T) {
						got, err := Parse(tt.input)
						if err != nil {
								t.Fatalf("unexpected error: %v", err)
						}
						if got != tt.want {
								t.Errorf("Parse(%q) = %d, want %d", tt.input, got, tt.want)
						}
				})
		}
}
```

### Effective Go and CodeReviewComments Notes

- **Effective Go** (go.dev/doc/effective_go) is official but not actively updated. Its advice remains largely sound, though some examples may reference older idioms.
- **CodeReviewComments** (go.dev/wiki/CodeReviewComments) was last edited December 2023 (by Russ Cox) per the wiki page. It remains the canonical Go project code review standard. Some items may not yet reflect Go 1.22+ changes (loop variable capture, range-over-func); apply current language knowledge alongside.

---

## 4. Enterprise-Grade Project Structure

### `cmd/`, `internal/`, `pkg/` Tradeoffs

| Directory | Purpose | Recommendation |
|-----------|---------|---------------|
| `cmd/` | Entry points: `cmd/api/main.go`, `cmd/worker/main.go` | Always use. Each binary gets own subdirectory. |
| `internal/` | Private application/library code; enforced by compiler | Use extensively. This is the main body of your application. |
| `pkg/` | Public library code usable by external packages | Use sparingly. Most enterprise code is internal. Only use if the package is genuinely imported by external consumers. |

**Opinion:** Avoid `pkg/` unless you have a concrete external consumer. The Go compiler enforces `internal/` visibility; `pkg/` provides no such enforcement and tends to become a dumping ground.

### Module Boundaries

- **Single module** (`go.mod` at root) for most enterprise services. Use Go workspaces (`go.work`) for multi-module monorepo development.
- **Multi-module** only when packages have genuinely different release cadences or dependency requirements.
- **Monorepo vs multi-repo**: Monorepo with clear module boundaries and CI affected-test strategies is typically simpler for enterprise services. Multi-repo adds operational overhead but may suit open-source library ecosystems.

### Bounded Contexts and Domain Separation

Organize by domain, not by technical layer. Avoid `internal/models`, `internal/handlers`, `internal/repositories` — these become god packages shared across contexts.

**Bad:**
```text
internal/
	models/       ← everything lives here
	handlers/      ← everything lives here
	repositories/  ← everything lives here
```

**Good:**
```text
internal/
	billing/
		service.go
		repository.go
		handler.go
	shipping/
		service.go
		repository.go
		handler.go
```

### Hexagonal Architecture Adapted to Go

Ports and adapters work well in Go when the domain has real business logic. Keep ports small (consumer-side interfaces). Adapters implement ports for specific technologies (Postgres, Kafka, HTTP).

```text
internal/
	billing/
		port.go        ← interfaces (ports): OrderStore, EventPublisher
		service.go     ← domain logic, depends only on ports
		adapter/
			postgres.go  ← implements OrderStore
			kafka.go     ← implements EventPublisher
```

**Warning:** Avoid cargo-cult clean architecture with unnecessary abstraction layers. If your service is a thin CRUD wrapper, hexagonal architecture adds overhead without benefit. Apply it when the domain logic is substantial and worth protecting from infrastructure dependencies.

### Clean Architecture Adapted to Go

- Prefer packages and explicit constructors over abstract base classes or framework-heavy patterns.
- Dependency rule: domain code imports nothing from transport/storage layers.
- Don't create `UseCase` interfaces with a single `Execute` method unless there's genuine polymorphism. A plain function often suffices.

### Practical Folder Examples

#### HTTP/gRPC Service

```text
myproject/
	cmd/api/
		main.go             ← thin entry point
	internal/
		app/
			run.go            ← composition root, app.Run(ctx)
		config/
			config.go         ← typed config struct, loader
		domain/
			order/
				order.go         ← domain types, invariants
				port.go          ← consumer-side interfaces
				service.go       ← domain logic
		adapter/
			postgres/
				order_repo.go   ← implements order.OrderStore
			kafka/
				order_events.go ← implements order.EventPublisher
		transport/
			http/
				handler.go      ← HTTP handler, depends on domain port
				middleware.go
			grpc/
				handler.go      ← gRPC handler
		logger/
			logger.go
		metrics/
			metrics.go
		health/
			health.go
	migrations/
	go.mod
	go.sum
	Dockerfile
```

#### Worker Service

```text
myproject/
	cmd/worker/
		main.go
	internal/
		app/
			run.go
		config/
		domain/
			order/
				service.go
		adapter/
			postgres/
			rabbitmq/
		worker/
			processor.go      ← main processing loop
			pool.go           ← worker pool
		logger/
		metrics/
```

#### CLI Service

```text
myproject/
	cmd/cli/
		main.go
	internal/
		app/
			run.go
		config/
		domain/
		adapter/
		cli/
			root.go          ← cobra or standard flag
			cmd_x.go
	go.mod
```

---

## 5. Reducing Massive `main.go` Bootstrap

### The Problem

Massive `main.go` files that configure logging, parse config, connect databases, run migrations, wire handlers, start HTTP/gRPC servers, register metrics, and manage lifecycle — all in one function — are difficult to test, review, and maintain.

### Composition Root Pattern

`main.go` should be boring. It calls a single function that owns the application lifecycle.

```go
// cmd/api/main.go
package main

import (
		"context"
		"fmt"
		"os"
		"os/signal"
		"syscall"

		"example.com/service/internal/app"
)

func main() {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		if err := app.Run(ctx); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
		}
}
```

### `internal/app` and `app.Run(ctx)`

The `app` package is the **composition root** — the only place that knows about all subsystems and wires them together.

```go
// internal/app/run.go
package app

func Run(ctx context.Context) error {
		cfg, err := config.Load(ctx)
		if err != nil {
				return fmt.Errorf("load config: %w", err)
		}

		log := logger.New(cfg.Log)

		db, err := storage.Connect(ctx, cfg.DB, log)
		if err != nil {
				return fmt.Errorf("connect db: %w", err)
		}
		defer db.Close()

		if err := storage.Migrate(ctx, db, cfg.DB); err != nil {
				return fmt.Errorf("migrate: %w", err)
		}

		orderStore := postgres.NewOrderRepo(db)
		eventPub := kafka.NewEventPublisher(cfg.Kafka, log)

		orderSvc := order.NewService(orderStore, eventPub, log)

		m := metrics.New()
		h := httpHandler.New(orderSvc, log, m)

		srv := &http.Server{Addr: cfg.HTTP.Addr, Handler: h}

		go func() {
				log.Info("http server starting", "addr", cfg.HTTP.Addr)
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
						log.Error("http server error", "err", err)
				}
		}()

		<-ctx.Done()
		log.Info("shutting down")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
}
```

### Provider Functions

Group subsystem creation into provider functions. Each provider is responsible for one subsystem's construction and lifecycle.

```go
// internal/app/providers.go
package app

func provideDB(ctx context.Context, cfg config.DB, log *slog.Logger) (*sql.DB, error) {
		db, err := sql.Open("pgx", cfg.DSN)
		if err != nil {
				return nil, fmt.Errorf("open db: %w", err)
		}
		db.SetMaxOpenConns(cfg.MaxOpenConns)
		db.SetMaxIdleConns(cfg.MaxIdleConns)
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

		if err := db.PingContext(ctx); err != nil {
				return nil, fmt.Errorf("ping db: %w", err)
		}
		log.Info("database connected")
		return db, nil
}

func provideHTTPServer(cfg config.HTTP, handler http.Handler) *http.Server {
		return &http.Server{
				Addr:              cfg.Addr,
				Handler:           handler,
				ReadHeaderTimeout: 5 * time.Second,
				IdleTimeout:       120 * time.Second,
		}
}
```

### Lifecycle Orchestration

For non-trivial services with multiple startable/stoppable components, use a simple lifecycle manager:

```go
type Component struct {
		Name string
		Start func(ctx context.Context) error
		Stop  func(ctx context.Context) error
}

func Run(ctx context.Context) error {
		// ... wire components ...
		components := []Component{
				{Name: "http", Start: srv.ListenAndServe, Stop: srv.Shutdown},
				{Name: "grpc", Start: grpcSrv.Serve, Stop: grpcSrv.GracefulStop},
				{Name: "worker", Start: worker.Run, Stop: worker.Shutdown},
		}

		// Start all
		for _, c := range components {
				if err := c.Start(ctx); err != nil {
						return fmt.Errorf("start %s: %w", c.Name, err)
				}
		}

		// Wait for shutdown signal
		<-ctx.Done()

		// Stop all in reverse order
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		var errs []error
		for i := len(components) - 1; i >= 0; i-- {
				if err := components[i].Stop(shutdownCtx); err != nil {
						errs = append(errs, fmt.Errorf("stop %s: %w", components[i].Name, err))
				}
		}
		return errors.Join(errs...)
}
```

### Bad vs Good: Main.go Bootstrap

```go
// BAD: 500-line main.go
func main() {
		cfg := parseFlags() + loadEnv() + readfile()  // 50 lines
		log := setupLogger(cfg)                        // 30 lines
		db := connectDB(cfg)                           // 40 lines + migrate
		redis := connectRedis(cfg)                     // 30 lines
		kafka := connectKafka(cfg)                     // 40 lines
		userService := NewUserService(db, redis)       // 20 lines
		orderService := NewOrderService(db, kafka)     // 20 lines
		handler := NewHandler(userService, orderService)// 30 lines
		middleware := setupMiddleware(cfg, log)         // 25 lines
		srv := &http.Server{...}                       // 20 lines
		go srv.ListenAndServe()                        // 10 lines
		go startMetrics(cfg)                           // 15 lines
		go startGRPC(cfg)                              // 30 lines
		go startWorkers(cfg)                           // 30 lines
		// ... shutdown logic ...                       // 50 lines
}
```

```go
// GOOD: 10-line main.go + composition root
func main() {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := app.Run(ctx); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
		}
}
```

---

## 6. Dependency Injection in Go

### Manual DI with Constructors (Preferred)

Explicit constructors are the idiomatic Go approach. Every dependency is visible at the call site.

```go
type OrderService struct {
		store  OrderStore
		events EventPublisher
		log    *slog.Logger
}

func NewOrderService(store OrderStore, events EventPublisher, log *slog.Logger) *OrderService {
		return &OrderService{store: store, events: events, log: log}
}
```

**Guidelines:**
- Inject concrete dependencies unless the consumer needs an interface seam for testing or alternative implementations.
- Keep interfaces at the consumer side (see Section 3).
- Inject config as typed structs, not raw environment variable access.
- Own lifecycle explicitly: start, readiness, shutdown, cleanup.

### Interface Placement

```go
// billing/order.go — consumer defines what it needs
type OrderStore interface {
		FindByID(ctx context.Context, id string) (Order, error)
		Save(ctx context.Context, o Order) error
}

type OrderService struct {
		store OrderStore  // depends on interface, not concrete type
}
```

```go
// adapter/postgres/order_repo.go — implements the interface
type OrderRepo struct { db *sql.DB }

func (r *OrderRepo) FindByID(ctx context.Context, id string) (order.Order, error) { ... }
func (r *OrderRepo) Save(ctx context.Context, o order.Order) error { ... }

// Compile-time check
var _ billing.OrderStore = (*OrderRepo)(nil)
```

### Config Injection

```go
// Bad: global config access
func NewService() *Service {
		dsn := os.Getenv("DATABASE_URL") // hidden dependency
		...
}

// Good: explicit config
type Config struct {
		DSN         string
		MaxConns    int
		Timeout     time.Duration
}

func NewService(cfg Config, log *slog.Logger) *Service {
		...
}
```

### Lifecycle Injection

For components with start/stop semantics, inject a lifecycle interface:

```go
type Starter interface {
		Start(ctx context.Context) error
}

type Stopper interface {
		Stop(ctx context.Context) error
}
```

### Test Seams

Constructors that accept interfaces provide natural test seams:

```go
func TestOrderService(t *testing.T) {
		mockStore := &MockOrderStore{...}
		mockEvents := &MockEventPublisher{...}
		svc := NewOrderService(mockStore, mockEvents, slog.Default())
		// test svc behavior
}
```

### Google Wire

Source: [github.com/google/wire](https://github.com/google/wire) — [Community convention]

Wire generates DI code at compile time. It is type-safe and produces readable output.

**When useful:** Large projects with many components where manual wiring becomes error-prone or tedious to maintain.

**Tradeoffs:**
- ✅ Compile-time safety; no runtime reflection.
- ✅ Generated code is readable and debuggable.
- ❌ Requires `wire.go` + `wire_gen.go` maintenance.
- ❌ Adds build step complexity.
- ❌ Overkill for small-to-medium services.

### Uber Fx / Dig

Source: [go.uber.org/fx](https://go.uber.org/fx) — [Community convention]

Fx/Dig uses runtime reflection for dependency resolution.

**When useful:** Services that need complex lifecycle management (start/stop hooks, dependency ordering).

**Tradeoffs:**
- ✅ Powerful lifecycle management (OnStart/OnStop hooks).
- ✅ Decorator/replace support for testing.
- ❌ Runtime resolution — errors appear at startup, not compile time.
- ❌ Harder to trace dependency graph by reading code.
- ❌ Reflection overhead and magic make debugging harder.
- ❌ Can encourage service-locator anti-pattern.

### Why Magical Service Locators / Global Containers Are Risky

```go
// BAD: service locator / global container
var Container = &DIContainer{}

func (s *OrderService) Process(ctx context.Context, id string) error {
		store := Container.Get("order_store").(OrderStore) // runtime cast, hidden dependency
		...
}
```

**Problems:**
- Dependencies are invisible at the call site and constructor.
- Runtime failures instead of compile-time failures.
- Impossible to understand the dependency graph by reading code.
- Makes testing harder — you must set up the global container and reset it between tests.
- Encourages circular dependencies.

---

## 7. Patterns Actually Useful in Go

These patterns are not mandatory. Use them when they reduce coupling and improve clarity. Avoid applying patterns from other ecosystems wholesale.

### Functional Options

Best for APIs with optional settings that may grow. Avoid for mandatory dependencies.

```go
type ClientOption func(*Client)

func WithTimeout(d time.Duration) ClientOption {
		return func(c *Client) { c.timeout = d }
}

func WithRetryMax(n int) ClientOption {
		return func(c *Client) { c.retryMax = n }
}

func NewClient(baseURL string, opts ...ClientOption) *Client {
		c := &Client{baseURL: baseURL, timeout: 5 * time.Second}
		for _, opt := range opts { opt(c) }
		return c
}
```

### Strategy via Interfaces/Functions

Use an interface when the strategy has multiple methods. Use a function type when there is a single behavior.

```go
// Interface-based strategy (multiple behaviors)
type Serializer interface {
		Serialize(v any) ([]byte, error)
		Deserialize(data []byte, v any) error
}

// Function-based strategy (single behavior)
type MatchFunc func(line string) bool

func Grep(r io.Reader, match MatchFunc) ([]string, error) { ... }
```

### Adapter Pattern

Convert one interface to another. Common for integrating with third-party APIs.

```go
// Adapt a third-party rate limiter to your interface
type RateLimiter interface {
		Allow(ctx context.Context, key string) (bool, error)
}

type ThirdPartyAdapter struct {
		limiter *thirdparty.Limiter
}

func (a *ThirdPartyAdapter) Allow(ctx context.Context, key string) (bool, error) {
		return a.limiter.Check(ctx, key)
}
```

### Decorator / Middleware

In Go, decorators are often simple function wrappers or middleware chains.

```go
// HTTP middleware
type Middleware func(http.Handler) http.Handler

func Logging(log *slog.Logger) Middleware {
		return func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						start := time.Now()
						next.ServeHTTP(w, r)
						log.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
				})
		}
}

func Chain(h http.Handler, mws ...Middleware) http.Handler {
		for i := len(mws) - 1; i >= 0; i-- {
				h = mws[i](h)
		}
		return h
}
```

### Repository Pattern — When Useful and When Harmful

**Useful when:** The domain has genuine persistence logic (e.g., complex queries, aggregate roots, multiple data stores behind one abstraction).

**Harmful when:** Every table gets a `Repository` interface with `FindByID`, `Save`, `Delete`, `List` — this is just CRUD with extra steps. If there's no domain logic protecting invariants, a query function is simpler.

```go
// Harmful: CRUD repository with no domain value
type UserRepository interface {
		FindByID(ctx context.Context, id string) (User, error)
		FindAll(ctx context.Context) ([]User, error)
		Save(ctx context.Context, u User) error
		Delete(ctx context.Context, id string) error
}

// Useful: intent-revealing interface with domain operations
type OrderStore interface {
		FindOpenByCustomer(ctx context.Context, custID string) ([]Order, error)
		SaveWithItems(ctx context.Context, o Order, items []Item) error
}
```

### Unit of Work / Transaction Boundary Pattern

Manage transaction boundaries explicitly, not implicitly through repositories.

```go
type DBTX interface {
		QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
		ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type OrderService struct {
		db *sql.DB
}

func (s *OrderService) PlaceOrder(ctx context.Context, o Order) error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
				return fmt.Errorf("begin tx: %w", err)
		}
		defer tx.Rollback() // no-op if committed

		if err := s.saveOrder(ctx, tx, o); err != nil {
				return err
		}
		if err := s.reserveInventory(ctx, tx, o.Items); err != nil {
				return err
		}
		return tx.Commit()
}
```

### Outbox / Inbox Patterns

[Community/industry convention] For ensuring consistency between database writes and message publishing:

```go
// Outbox: write business data + outbox message in one DB transaction
func (s *OrderService) PlaceOrder(ctx context.Context, o Order) error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil { return err }
		defer tx.Rollback()

		if err := saveOrder(ctx, tx, o); err != nil { return err }
		if err := saveOutboxMsg(ctx, tx, "order.placed", o); err != nil { return err }
		return tx.Commit()
}

// Relay: poll outbox table and publish to broker
func (r *Relay) Run(ctx context.Context) error {
		for {
				select {
				case <-ctx.Done():
						return ctx.Err()
				case <-time.After(500 * time.Millisecond):
						msgs, err := r.pollOutbox(ctx)
						if err != nil { continue }
						for _, m := range msgs {
								if err := r.publish(ctx, m); err != nil { r.log.Error("publish", "err", err) }
						}
				}
		}
}
```

Key: make consumers idempotent, since the relay may publish duplicates.

### Worker Pools, Pipelines, Fan-Out/Fan-In

Source: [go.dev/blog/pipelines](https://go.dev/blog/pipelines), accessed 2026-05-11.

```go
// Worker pool with bounded concurrency
func Process(ctx context.Context, items <-chan Item, workers int) <-chan Result {
		out := make(chan Result, workers)
		var wg sync.WaitGroup
		wg.Add(workers)

		for i := 0; i < workers; i++ {
				go func() {
						defer wg.Done()
						for item := range items {
								select {
								case <-ctx.Done():
										return
								case out <- process(item):
								}
						}
				}()
		}

		go func() {
				wg.Wait()
				close(out)
		}()

		return out
}
```

**Critical:** Always propagate context cancellation; always close output channels; always wait for workers to finish.

### Factories Only When Justified

Use constructors first. Add factories when selection logic, lifecycle, or configuration complexity justifies them.

```go
// Constructor — sufficient for most cases
func NewPostgresStore(db *sql.DB) *PostgresStore { ... }

// Factory — justified when selection is dynamic
func NewStore(cfg Config) (Store, error) {
		switch cfg.Driver {
		case "postgres":
				return NewPostgresStore(cfg.Postgres)
		case "mysql":
				return NewMySQLStore(cfg.MySQL)
		default:
				return nil, fmt.Errorf("unsupported driver: %s", cfg.Driver)
		}
}
```

### Composition Over Inheritance

Go has no class inheritance. Compose behavior with structs, interfaces, and functions.

```go
// Compose behaviors through struct fields
type Server struct {
		auth    Authenticator
		limiter RateLimiter
		log     *slog.Logger
}
```

### DDD Bounded Contexts in Go

Use bounded contexts to separate domain language and invariants. Each context owns its types, interfaces, and logic. Avoid one giant `models` package shared across contexts.

```text
internal/
	billing/
		model.go      ← Order, Invoice — billing's view
		service.go
	shipping/
		model.go      ← Order, Shipment — shipping's view (different Order!)
		service.go
```

Shared kernel: if contexts must share types, create a minimal `internal/sharedkernel` with only the common concepts.

---

## 8. Anti-Patterns

### Anonymous Structs Abuse

Anonymous structs are fine for localized tests or throwaway JSON fixtures. They are harmful when the same shape is used in multiple places.

```go
// Bad: anonymous struct used as a function parameter
func CreateUser(in struct {
		Name string
		Age  int
}) error { ... }

// Good: named type
type CreateUserInput struct {
		Name string
		Age  int
}
func CreateUser(in CreateUserInput) error { ... }
```

### Huge `main.go` Bootstrap

See Section 5 for full treatment. A massive `main.go` with 300+ lines of wiring, migration, and server setup hides dependencies and makes testing impossible.

```go
// Bad: main.go does everything
func main() {
		// 300+ lines: config, DB, Redis, Kafka, migrations, handlers, middleware,
		// servers, workers, metrics, graceful shutdown...
}
```

### Overengineering Interfaces

```go
// Bad: interface with one implementation "for mocking"
type UserRepository interface {
		Find(ctx context.Context, id string) (*User, error)
}
type postgresUserRepo struct { db *sql.DB }
// Only one implementation ever exists

// Good: accept concrete type; let consumer define interface if needed
type UserRepo struct { db *sql.DB }
func (r *UserRepo) Find(ctx context.Context, id string) (*User, error) { ... }
```

### Interfaces Defined at Producer Side by Default

CodeReviewComments §19 is explicit: "Interfaces belong in the package that uses values of the interface type, not the package that implements them."

```go
// Bad: postgres package defines the interface it implements
package postgres
type OrderRepository interface { ... }  // Forces all consumers to accept the full interface

// Good: billing package defines only what it needs
package billing
type OrderFinder interface {
		FindByID(ctx context.Context, id string) (Order, error)
}
```

### Package-Level Mutable Global State

```go
// Bad: hidden global state
var DB *sql.DB
var Config AppConfig

func GetUser(id string) (*User, error) {
		return GetUserFromDB(DB, id) // DB is hidden; tests must modify global
}

// Good: explicit dependency
type UserService struct { db *sql.DB }
func (s *UserService) GetUser(ctx context.Context, id string) (*User, error) {
		return GetUserFromDB(s.db, id)
}
```

### `init()` Abuse

```go
// Bad: init wires hidden dependencies
func init() {
		db, _ := sql.Open("pgx", os.Getenv("DATABASE_URL"))
		globalDB = db
}

// Good: explicit initialization
func NewService(cfg Config) *Service {
		db, err := sql.Open("pgx", cfg.DSN)
		if err != nil { return nil, err }
		return &Service{db: db}, nil
}
```

Use `init()` only for registering side effects that the Go ecosystem conventionally requires (e.g., `database/sql` driver registration via `import _ "github.com/lib/pq"`).

### Context Stored in Structs

CodeReviewComments §3: "Don't add a Context member to a struct type."

```go
// Bad
type Client struct {
		ctx context.Context // Stale context; no cancellation propagation
}

// Good
type Client struct { http *http.Client }
func (c *Client) Fetch(ctx context.Context, url string) (*Response, error) { ... }
```

### Panic for Normal Control Flow

```go
// Bad: panic for expected conditions
func Parse(s string) int {
		n, err := strconv.Atoi(s)
		if err != nil {
				panic(fmt.Sprintf("invalid number: %s", s))
		}
		return n
}

// Good: return error
func Parse(s string) (int, error) {
		return strconv.Atoi(s)
}
```

### Unbounded Goroutines

```go
// Bad: one goroutine per item with no bound
func Process(items []Item) {
		for _, item := range items {
				go process(item) // 1M items = 1M goroutines
		}
}

// Good: bounded worker pool
func Process(ctx context.Context, items []Item, workers int) error {
		sem := make(chan struct{}, workers)
		g, ctx := errgroup.WithContext(ctx)
		for _, item := range items {
				sem <- struct{}{}
				item := item
				g.Go(func() error {
						defer func() { <-sem }()
						return process(ctx, item)
				})
		}
		return g.Wait()
}
```

### Channels Used Where Mutex/Simple Function Call Is Better

```go
// Bad: channel for simple state access
type Counter struct { ch chan func() }
func NewCounter() *Counter {
		c := &Counter{ch: make(chan func())}
		go func() {
				var n int
				for f := range c.ch { f() }
		}()
		return c
}

// Good: mutex for shared state
type Counter struct {
		mu sync.Mutex
		n  int
}
func (c *Counter) Inc() { c.mu.Lock(); c.n++; c.mu.Unlock() }
func (c *Counter) Get() int { c.mu.Lock(); defer c.mu.Unlock(); return c.n }
```

### Reflection-Heavy Code Without Need

```go
// Bad: generic map-based validation
func Validate(v any, rules map[string]Rule) error { ... } // runtime errors, no IDE support

// Good: typed validation
func ValidateOrder(o Order) error {
		if o.Total < 0 { return errors.New("total must be non-negative") }
		return nil
}
```

Use reflection only for genuinely dynamic needs (e.g., generic marshaling frameworks, `encoding/json` internals).

### Optional Params via `map[string]any`

```go
// Bad: untyped optional parameters
func NewClient(opts map[string]any) *Client {
		timeout, ok := opts["timeout"].(time.Duration) // runtime panic on wrong type
		...
}

// Good: functional options
func NewClient(baseURL string, opts ...ClientOption) *Client { ... }
```

### Magical DI Containers / Service Locator

See Section 6 for full treatment. Service locators hide dependencies, cause runtime failures, and make testing harder.

### God Packages and Utility Dumping Grounds

CodeReviewComments §25: "Avoid meaningless package names like `util`, `common`, `misc`, `api`, `types`, `interfaces`."

```go
// Bad: kitchen-sink packages
package util      // contains Base64Encode, UUID, Truncate, ParseInt...
package types     // contains every struct in the project
package helpers   // undefined responsibility

// Good: focused packages
package encoding  // Base64Encode, HexEncode
package uuid     // UUID generation
package strutil  // Truncate, Slugify (consider if small enough to merge)
```

### Premature Micro-Optimizations

```go
// Bad: premature optimization without evidence
func Format(s string) string {
		var b strings.Builder // "strings.Builder is faster"
		b.Grow(len(s) + 10)   // "avoid allocations"
		...
		return b.String()
}

// Good: simple code first; optimize when benchmarks prove it matters
func Format(s string) string {
		return "prefix_" + s + "_suffix"
}
```

### Ignored Errors

```go
// Bad: ignored error
file, _ := os.Open(path)
data, _ := io.ReadAll(file)
file.Close()

// Good: handle errors
file, err := os.Open(path)
if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
}
defer file.Close()
data, err := io.ReadAll(file)
if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
}
```

### Leaky Abstractions Around Database Transactions

```go
// Bad: repository hides transaction; caller cannot control boundary
type OrderRepo struct { db *sql.DB }
func (r *OrderRepo) Save(o Order) error {
		tx, _ := r.db.Begin() // hidden transaction; no way to combine with other ops
		...
		return tx.Commit()
}

// Good: explicit transaction boundary; repository accepts DBTX
type DBTX interface {
		ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type OrderRepo struct{}

func (r *OrderRepo) Save(ctx context.Context, tx DBTX, o Order) error { ... }

// Service controls the transaction
func (s *Service) PlaceOrder(ctx context.Context, o Order) error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil { return err }
		defer tx.Rollback()
		if err := s.orderRepo.Save(ctx, tx, o); err != nil { return err }
		if err := s.inventoryRepo.Reserve(ctx, tx, o.Items); err != nil { return err }
		return tx.Commit()
}
```

---

## 9. High-Performance and Low-Latency Go

### Measurement-First Mindset

Source: [go.dev/doc/diagnostics](https://go.dev/doc/diagnostics), accessed 2026-05-11.

Never optimize without measurement. Use the standard toolchain first:

```bash
# Benchmarks
go test ./... -run=^$ -bench=. -benchmem -count=5
go test ./pkg -cpuprofile cpu.prof -memprofile mem.prof -bench .

# Profiles
go tool pprof cpu.prof
go tool pprof http://localhost:6060/debug/pprof/heap

# Race detector
go test ./... -race

# Execution tracer
go test -trace=trace.out ./pkg
go tool trace trace.out

# Mutex and block profiles
go test -mutexprofile mutex.prof -blockprofile block.prof ./pkg
```

### Benchmarks

Use `testing.B.Loop` (Go 1.24+) for accurate, efficient benchmarks:

```go
func BenchmarkEncode(b *testing.B) {
		for b.Loop() {
				_, err := json.Marshal(largePayload)
				if err != nil {
						b.Fatal(err)
				}
		}
}
```

### pprof, trace, runtime/metrics, Execution Tracer

- **pprof**: CPU, heap, goroutine, block, mutex profiles. Source: [pkg.go.dev/runtime/pprof](https://pkg.go.dev/runtime/pprof).
- **trace**: Execution tracer shows goroutine scheduling, GC pauses, syscalls. Source: [pkg.go.dev/runtime/trace](https://pkg.go.dev/runtime/trace).
- **runtime/metrics**: Programmatic access to runtime statistics. Source: [pkg.go.dev/runtime/metrics](https://pkg.go.dev/runtime/metrics).
- **Trace Flight Recorder** (Go 1.25+): Continuous in-memory trace for post-incident analysis.

### Allocation Reduction and Escape Analysis

```bash
# Show escape analysis decisions
go build -gcflags=all=-m=2 ./...

# Verify specific function
go build -gcflags=all=-m ./pkg/service.go 2>&1 | grep "escape"
```

Common allocation reduction strategies:
- **Preallocate slices** when size is known: `make([]T, 0, n)` not `[]T{}`.
- **Avoid `string([]byte)`** conversions in hot paths; use `unsafe.String` only when justified and isolated.
- **Reuse buffers** with clear ownership (e.g., `sync.Pool`).
- **Prefer `strings.Builder`** for repeated string concatenation.
- **Avoid retaining backing arrays** via small slices: `b = b[:0:0]` or copy.

### Pooling Caveats and `sync.Pool` Tradeoffs

Source: [pkg.go.dev/sync#Pool](https://pkg.go.dev/sync#Pool), accessed 2026-05-11.

`sync.Pool` is for **temporary** objects. It is NOT a durable cache.

- `Get` may return nil or any item — no ordering guarantees.
- Items may be reclaimed at any GC cycle.
- Do NOT depend on pool retention, ordering, or object identity.

```go
// Good: Pool for temporary buffers
var bufPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

func Process(w io.Writer, data []byte) error {
		b := bufPool.Get().(*bytes.Buffer)
		b.Reset()
		defer bufPool.Put(b)
		// use b
		return nil
}
```

### GC Tuning: GOGC / GOMEMLIMIT

Source: [pkg.go.dev/runtime/debug](https://pkg.go.dev/runtime/debug), accessed 2026-05-11.

- **GOGC** (default 100): GC runs when heap grows by this percentage. Increase to reduce GC frequency; decrease for lower latency at cost of more frequent GC.
- **GOMEMLIMIT**: Soft memory limit. GC works to keep heap under this value. Critical for containers. Set to 80–90% of container memory limit.
- **Green Tea GC** (Go 1.26 default): 10–40% GC overhead reduction. Disable with `GOEXPERIMENT=nogreenteagc`.

```go
// In main or app.Run:
debug.SetGCPercent(200)                   // Less frequent GC
debug.SetMemoryLimit(1 * debug.GiB)       // Container-aware limit
```

**Caveats:** Tune GC only after load testing with production-like traffic. Measure latency percentiles (p50, p95, p99, p999) before and after changes.

### Memory Layout, Cache Locality, False Sharing

**Struct field ordering** affects memory layout and cache performance:

```go
// Bad: 24 bytes + 6 bytes padding = 30 bytes (on 64-bit)
type Stats struct {
		Active   bool      // 1 byte + 7 padding
		Count    int64     // 8 bytes
		Total    int64     // 8 bytes
		Archived bool      // 1 byte + 7 padding
		Errors   int64     // 8 bytes
}

// Good: 25 bytes + 7 padding = 32 bytes, but better cache line usage
// Group fields by size: largest first
type Stats struct {
		Count    int64     // 8 bytes
		Total    int64     // 8 bytes
		Errors   int64     // 8 bytes
		Active   bool      // 1 byte
		Archived bool      // 1 byte
		// 6 bytes padding
}
```

**False sharing**: When multiple goroutines write to different fields of the same struct that share a cache line (~64 bytes), mutual invalidation causes performance degradation. Use padding or separate structs for hot data.

```go
// Bad: false sharing
type Counters struct {
		Requests int64 // Goroutine A writes
		Errors   int64 // Goroutine B writes — same cache line!
}

// Good: pad to separate cache lines
type Counters struct {
		Requests int64
		_        [7]int64 // padding to 64 bytes
		Errors   int64
}
```

### Interfaces/Generics Cost Considerations

- **Interfaces** incur allocation when a value type is stored in an interface (boxing). In hot paths, consider whether an interface is necessary.
- **Generics** improve type safety and reduce code duplication. They are not automatically faster. Generic functions over interfaces may be slower or faster depending on the monomorphization and inlining decisions.
- **Type switches** on interfaces have minimal overhead; don't avoid them for performance reasons alone.
- Measure. Use `-benchmem` and pprof before deciding.

### Avoiding Goroutine Leaks

```go
// Bad: goroutine leak — channel never closed
func FetchAll(urls []string) []string {
		ch := make(chan string)
		for _, u := range urls {
				go func(u string) { ch <- fetch(u) }(u)
		}
		var results []string
		for range urls {
				results = append(results, <-ch)
		}
		return results
		// LEAK: if fetch() blocks forever on one URL, other goroutines block forever on ch <-
}

// Good: use context and errgroup
func FetchAll(ctx context.Context, urls []string) ([]string, error) {
		g, ctx := errgroup.WithContext(ctx)
		ch := make(chan string, len(urls))
		for _, u := range urls {
				u := u
				g.Go(func() error {
						select {
						case <-ctx.Done():
								return ctx.Err()
						case ch <- fetch(u):
								return nil
						}
				})
		}
		go func() {
				g.Wait()
				close(ch)
		}()
		var results []string
		for s := range ch {
				results = append(results, s)
		}
		return results, g.Wait()
}
```

### Timers/Tickers Correct Use

```go
// Bad: time.After in a loop allocates a new timer each iteration
for msg := range ch {
		select {
		case <-time.After(5 * time.Second): // NEW allocation each loop!
				return
		case <-done:
				return
		}
}

// Good: reuse a timer
timer := time.NewTimer(5 * time.Second)
defer timer.Stop()
for msg := range ch {
		select {
		case <-timer.C:
				return
		case <-done:
				return
		default:
				process(msg)
				if !timer.Stop() {
						<-timer.C
				}
				timer.Reset(5 * time.Second)
		}
}
```

### Context Deadlines/Timeouts

```go
// Bad: no timeout on external call
func Fetch(ctx context.Context, url string) ([]byte, error) {
		resp, err := http.Get(url) // no context, no timeout
		...
}

// Good: context with timeout
func Fetch(ctx context.Context, url string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
				return nil, fmt.Errorf("create request: %w", err)
		}
		resp, err := http.DefaultClient.Do(req)
		...
}
```

### Channels vs Mutexes

- **Use channels** for ownership transfer, cancellation, fan-in/fan-out, pipelines.
- **Use `sync.Mutex`/`sync.RWMutex`** for protecting shared in-memory state.
- **Don't use channels** when a function call or mutex is simpler and clearer.

### `net/http` Tuning, Clients, Connection Reuse, Timeouts

Source: [pkg.go.dev/net/http](https://pkg.go.dev/net/http), accessed 2026-05-11.

```go
// Bad: new client per request, no timeouts
func call(url string) ([]byte, error) {
		resp, err := (&http.Client{}).Get(url)
		...
}

// Good: shared client with proper timeouts
var httpClient = &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
				DialContext: (&net.Dialer{
						Timeout:   5 * time.Second,
				}).DialContext,
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   10,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   5 * time.Second,
				ResponseHeaderTimeout: 5 * time.Second,
		},
}
```

- Reuse `http.Client` and `http.Transport` — they are safe for concurrent use.
- Use `Server.Shutdown(ctx)` for graceful shutdown.
- Be aware `ServeMux` routing changed significantly in Go 1.22 (methods, wildcards).

### `database/sql` Pooling, Context, Transactions

Source: [pkg.go.dev/database/sql](https://pkg.go.dev/database/sql), accessed 2026-05-11.

- `*sql.DB` is a concurrency-safe pool handle.
- Always use `QueryContext`, `ExecContext`, `BeginTx`.
- Always close `Rows`.
- Always commit or roll back transactions.
- Tune `SetMaxOpenConns`, `SetMaxIdleConns`, `SetConnMaxLifetime` using production metrics.
- Remember context cancellation depends on driver support.

### JSON Encoding Considerations

Source: [pkg.go.dev/encoding/json](https://pkg.go.dev/encoding/json), accessed 2026-05-11.

- Decide whether unknown fields should be rejected for public APIs (use `Decoder.DisallowUnknownFields`).
- Use `Encoder.SetEscapeHTML(false)` only when appropriate for the sink.
- `encoding/json/v2` (Go 1.25+, experimental via `GOEXPERIMENT=jsonv2`): substantially faster decoding; API is subject to change.
- Third-party JSON libraries (e.g., `sonnet`, `jsoniter`, `go-json`) are **[Community convention / third-party]**. Require benchmarks and compatibility tests before replacing the standard package.

### Logging Performance: slog, Structured Logs, Sampling

Source: [pkg.go.dev/log/slog](https://pkg.go.dev/log/slog), accessed 2026-05-11.

- Use structured logging with `log/slog` (standard library since Go 1.21).
- Use `LogAttrs` for hot paths to avoid allocation of `any` slice.
- Use `slog.InfoContext` when context is available.
- Consider log sampling (`Handler` wrapper that samples) for very high-volume paths.
- Use `NewMultiHandler` (Go 1.26) for multi-sink logging.
- Avoid expensive computations in log statements when the level is disabled.

```go
// Avoid allocation when level is disabled
slog.LogAttrs(ctx, slog.LevelDebug, "processing",
		slog.String("id", id),
		slog.Int("count", count),
)
```

### Backpressure, Rate Limiting, Bounded Queues

```go
// Bounded queue with backpressure
type Queue struct {
		ch chan Job
}

func NewQueue(size int) *Queue {
		return &Queue{ch: make(chan Job, size)}
}

func (q *Queue) Enqueue(ctx context.Context, job Job) error {
		select {
		case q.ch <- job:
				return nil
		default:
				return fmt.Errorf("queue full, try again later")
		}
}
```

- Use bounded channels or semaphore patterns to prevent unbounded memory growth.
- Return errors (or HTTP 429/503) when capacity is exceeded — do not block indefinitely.
- Rate limiting: `golang.org/x/time/rate` [Community convention] or custom token bucket.

### Low-Latency Service Checklist

- [ ] Timeouts on all external calls (DB, HTTP, gRPC, cache).
- [ ] Connection pooling and reuse.
- [ ] Bounded goroutine pools; no unbounded fan-out.
- [ ] Context propagation through all layers.
- [ ] GC tuning verified with production-like load (GOGC, GOMEMLIMIT).
- [ ] Struct field ordering for cache locality in hot structs.
- [ ] Allocation profiling on hot paths (`-benchmem`, pprof).
- [ ] Preallocation of slices/maps where size is known.
- [ ] No `time.After` in loops.
- [ ] `sync.Pool` for reusable temporary objects (not caches).
- [ ] Latency percentile monitoring (p50, p95, p99, p999).
- [ ] No reflection in hot paths without justification.
- [ ] JSON encoding choices benchmarked.

---

## 10. Concurrency Review Guide

### Race Conditions

- Use `go test -race` in CI for all packages that touch concurrent code.
- Data races are undefined behavior — they may pass tests and fail in production.

```go
// Bad: data race
var counter int
go func() { counter++ }()  // write
fmt.Println(counter)        // read — race!

// Good: atomic or mutex
var counter atomic.Int64
go func() { counter.Add(1) }()
fmt.Println(counter.Load())
```

### Data Ownership

**The code that starts a goroutine should know how it stops.** Make ownership explicit:

- Variables owned by a single goroutine need no synchronization.
- Shared state must be protected by mutex, atomic, or channel-based ownership transfer.

### Synchronization

| Primitive | Use When |
|-----------|---------|
| `sync.Mutex` | Protect shared mutable state |
| `sync.RWMutex` | Read-heavy shared state (measure to confirm benefit) |
| `sync.WaitGroup` | Wait for a set of goroutines to finish |
| `sync.Once` | One-time initialization |
| `sync/atomic` | Lock-free counters, flags, pointer swaps |
| `sync.Cond` | Wait/signal on condition (rarely needed; prefer channels) |
| `errgroup.Group` | Wait for goroutines + collect first error |
| Channels | Ownership transfer, cancellation, pipelines, fan-in/fan-out |

### Cancellation

```go
// Bad: no cancellation path
go func() {
		for { doWork() } // runs forever
}()

// Good: context-based cancellation
g, ctx := errgroup.WithContext(ctx)
g.Go(func() error {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
				select {
				case <-ctx.Done():
						return ctx.Err()
				case <-ticker.C:
						if err := doWork(ctx); err != nil {
								return err
						}
				}
		}
})
```

### Goroutine Lifecycle

Every goroutine must have a clear exit condition:

1. Context cancellation.
2. Channel close.
3. Done signal via `sync.WaitGroup` or `errgroup.Group`.
4. Finite work completion.

### `errgroup` and `WaitGroup`

```go
// errgroup: concurrent work with error collection
g, ctx := errgroup.WithContext(ctx)
g.SetLimit(runtime.GOMAXPROCS(0))

for _, item := range items {
		item := item
		g.Go(func() error {
				return process(ctx, item)
		})
}
if err := g.Wait(); err != nil {
		return err
}
```

```go
// sync.WaitGroup: fire-and-forget with wait
var wg sync.WaitGroup
for _, item := range items {
		wg.Add(1)
		go func(item Item) {
				defer wg.Done()
				process(item)
		}(item)
}
wg.Wait()
```

### Channel Closing Rules

- **Only the sender closes a channel.** Never close from the receiver side.
- **A closed channel never blocks on read** — it returns zero values immediately.
- Closing a nil or already-closed channel panics.
- Use `sync.Once` if multiple goroutines might attempt to close:

```go
type SafeChannel struct {
		ch   chan int
		once sync.Once
}

func (s *SafeChannel) Close() {
		s.once.Do(func() { close(s.ch) })
}
```

### Context Propagation

```go
// Bad: context not propagated
func (s *Service) Process(r Request) error {
		return s.store.Save(r.Data) // no context → no cancellation, no tracing
}

// Good: context propagated
func (s *Service) Process(ctx context.Context, r Request) error {
		return s.store.Save(ctx, r.Data) // cancellation, timeouts, tracing all work
}
```

### Backpressure

Don't accept work faster than you can process it. Use bounded channels, semaphores, or explicit rejection:

```go
// Semaphore pattern
sem := make(chan struct{}, maxConcurrent)
for _, item := range items {
		sem <- struct{}{} // blocks when at capacity
		go func(item Item) {
				defer func() { <-sem }()
				process(ctx, item)
		}(item)
}
```

---

## 11. Error Handling and Resilience

### `errors.Is`, `errors.As`, `errors.Join`

Source: [pkg.go.dev/errors](https://pkg.go.dev/errors), accessed 2026-05-11.

```go
// errors.Is: compare against sentinel/wrapped errors
if errors.Is(err, sql.ErrNoRows) { ... }

// errors.As: extract specific error type
var timeout *net.TimeoutError
if errors.As(err, &timeout) { ... }

// errors.AsType[T] (Go 1.26): generic, type-safe version of As
if terr := errors.AsType[*net.TimeoutError](err); terr != nil { ... }

// errors.Join: combine multiple errors
err := errors.Join(err1, err2, err3)
```

### Wrapping and Sentinel Errors

```go
// Sentinel error
var ErrNotFound = errors.New("not found")

// Wrap with context
if err != nil {
		return fmt.Errorf("fetch user %s: %w", id, err) // preserves ErrNotFound in chain
}

// Consumer checks
if errors.Is(err, ErrNotFound) { ... }
```

**Guideline:** Wrap errors at package boundaries with operational context. Don't wrap inside the same package where the error originates — that's what the error message itself should convey.

### Domain Errors

Define domain-specific error types for business logic:

```go
type OrderError struct {
		OrderID string
		Code    string
		Err     error
}

func (e *OrderError) Error() string { return fmt.Sprintf("order %s: %s: %v", e.OrderID, e.Code, e.Err) }
func (e *OrderError) Unwrap() error { return e.Err }
```

### Retryable Errors

[Community convention] Mark errors as retryable:

```go
type RetryableError struct {
		Err error
}

func (e *RetryableError) Error() string { return e.Err.Error() }
func (e *RetryableError) Unwrap() error { return e.Err }

func IsRetryable(err error) bool {
		var r *RetryableError
		return errors.As(err, &r)
}
```

### Timeouts, Retries with Jitter

```go
func Retry(ctx context.Context, maxAttempts int, baseDelay time.Duration, fn func() error) error {
		var err error
		for attempt := 0; attempt < maxAttempts; attempt++ {
				if err = fn(); err == nil {
						return nil
				}
				if !IsRetryable(err) {
						return err
				}
				delay := baseDelay * time.Duration(1<<uint(attempt)) // exponential backoff
				delay = jitter(delay) // add jitter to avoid thundering herd
				select {
				case <-ctx.Done():
						return ctx.Err()
				case <-time.After(delay):
				}
		}
		return fmt.Errorf("max retries exceeded: %w", err)
}

func jitter(d time.Duration) time.Duration {
		jitter := time.Duration(rand.Int63N(int64(d / 2))) // Go 1.22+ rand.Int63N
		return d + jitter
}
```

### Circuit Breakers [Community Convention]

Use when downstream failure can cascade. Implement with `golang.org/x/time` or dedicated libraries.

- **Open**: Fail fast; don't call downstream.
- **Half-open**: Allow one probe to test recovery.
- **Closed**: Normal operation.

Circuit breakers are not universal. Use them for external dependencies where failure cascading is a real risk.

### Idempotency

- All retried write operations must be idempotent.
- Use idempotency keys for API requests: `[Community/industry convention]` client sends `Idempotency-Key` header; server stores result and returns it for duplicate requests.

### Outbox/Inbox

See Section 7 for implementation patterns. Critical for DB-to-message-broker consistency.

### Sagas/Process Managers [Community/Industry Convention]

Use sagas when a business transaction spans multiple services and 2PC is not available. Implement as:
- **Choreography**: Each service emits events; others react.
- **Orchestration**: A central coordinator calls services and manages compensating actions.

Use only when the business process genuinely spans service boundaries. Don't introduce saga complexity within a single service.

---

## 12. Observability and Operations

### Structured Logging with `slog`

Source: [pkg.go.dev/log/slog](https://pkg.go.dev/log/slog), accessed 2026-05-11.

```go
// Initialize in composition root
log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		AddSource: true,
}))

// Use context-aware logging
slog.InfoContext(ctx, "order placed",
		slog.String("order_id", id),
		slog.Int("item_count", len(items)),
		slog.Duration("duration", elapsed),
)

// Go 1.26: Multi-handler for dual output
handler := slog.NewMultiHandler(
		slog.NewJSONHandler(os.Stdout, nil),          // structured for log aggregator
		slog.NewTextHandler(os.Stderr, nil),           // human-readable for console
)
log := slog.New(handler)
```

### OpenTelemetry

Source: [opentelemetry.io/docs/languages/go/](https://opentelemetry.io/docs/languages/go/), accessed 2026-05-11.

- Use OTel Go SDK for traces and metrics (stable), logs (beta/experimental).
- Prefer OTLP export to an OpenTelemetry Collector in production.
- Propagate trace/span IDs into logs for correlation.
- Define semantic conventions consistently across services.

### Metrics

- Latency percentiles (p50, p95, p99, p999).
- Error rates by type and endpoint.
- Saturation (goroutine count, DB pool utilization, queue depth).
- Dependency call latency and error rates.

### Tracing

- Trace across service boundaries.
- Include operation name, key identifiers, and error information.
- Use `context` propagation to carry trace context.

### Correlation IDs

```go
type contextKey struct{}

func WithCorrelationID(ctx context.Context, id string) context.Context {
		return context.WithValue(ctx, contextKey{}, id)
}

func CorrelationID(ctx context.Context) string {
		v, _ := ctx.Value(contextKey{}).(string)
		return v
}
```

### Health / Readiness / Liveness

- **Liveness**: Process is stuck and should be restarted. Keep it cheap (`SELECT 1`).
- **Readiness**: Instance can receive traffic. Check downstream dependencies.
- **Startup**: Slow initialization gate before liveness/readiness.

```go
func healthHandler(w http.ResponseWriter, r *http.Request) {
		if err := db.PingContext(r.Context()); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
		}
		w.WriteHeader(http.StatusOK)
}
```

### Graceful Shutdown

```go
func Run(ctx context.Context) error {
		srv := &http.Server{Addr: ":8080", Handler: h}

		go func() {
				<-ctx.Done()
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				srv.Shutdown(shutdownCtx)
		}()

		return srv.ListenAndServe()
}
```

### Config and Secrets

- Typed config structs with clear source precedence: defaults → file → env → flags → secret manager.
- Never log secrets, tokens, passwords, or connection strings. Source: [OWASP Logging Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html).
- Use `GOFIPS140` and secret management for enterprise compliance (Go 1.24+).

### Migrations

- Treat migrations as reviewed, tested, versioned code.
- Include rollback/forward-fix strategy.
- Pre-migration safety checks (e.g., column existence before adding).
- Run migrations before serving traffic (or gate readiness on migration completion).

### Feature Flags [Community Convention]

Use feature flags for gradual rollouts, A/B testing, and kill switches. Libraries: `flipt`, `unleash`, `launchdarkly`. Ensure flags are not a substitute for proper configuration.

### Runbooks

For each service, maintain a runbook covering:
- Startup/shutdown procedure.
- Health check endpoints and expected behavior.
- Common failure modes and recovery steps.
- Scaling and capacity guidance.

---

## 13. Security and Supply Chain

### govulncheck

Source: [go.dev/security/vuln](https://go.dev/security/vuln/), accessed 2026-05-11.

Run in CI:

```bash
govulncheck ./...
```

`govulncheck` analyzes call graphs and reports only vulnerabilities in code you actually call, reducing noise vs. flat vulnerability scanning.

### Modules and Pinned Versions

- Use `go mod tidy` and review `go.sum` changes in PRs.
- Use `GOPRIVATE`, `GONOPROXY`, `GONOSUMDB` correctly for private modules.
- Consider `GOAUTH` (Go 1.24+) for authenticated private module fetches.
- Pin toolchain version in `go.mod` for reproducibility.
- Use `go mod graph` and `go mod why` to understand dependency trees.

### Minimal Containers

```dockerfile
# Multi-stage build
FROM golang:1.26 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/server ./cmd/api

FROM scratch
COPY --from=builder /bin/server /bin/server
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["/bin/server"]
```

- Use `scratch` or `distroless` base images.
- `CGO_ENABLED=0` for static binary.
- Don't run as root. Use `USER` directive (not available in `scratch` — use `distroless` or set up user in builder).
- Scan images in CI with Trivy or Snyk [Community tools].

### Secrets

- Never hardcode secrets. Use environment variables, secret managers, or mounted volumes.
- Never log secrets.
- Use `runtime/secret` (Go 1.26, experimental) for cryptographic forward secrecy.
- Use `GOFIPS140` for compliance (Go 1.24+).

### TLS

- Post-quantum TLS key exchanges are **default-on** in Go 1.26 (`SecP256r1MLKEM768`, `SecP384r1MLKEM1024`). Source: [go.dev/doc/go1.26](https://go.dev/doc/go1.26).
- SHA-1 is disallowed in TLS 1.2 (Go 1.25+).
- 3DES cipher suites were removed from defaults (Go 1.23+).
- Don't disable TLS verification in production (`InsecureSkipVerify: true`).

### Input Validation, SSRF, Path Traversal

- Validate and encode user input at boundaries.
- Use `os.Root` (Go 1.24+) for directory-limited filesystem access.
- SSRF: validate/allowlist URLs before fetching; use `httputil.ReverseProxy` with `Rewrite` (not deprecated `Director`).
- Path traversal: use `filepath.Clean` and verify the cleaned path stays within allowed directories.

### SQL Injection

- Use parameterized queries — always. `database/sql` parameterization is safe.
- Never interpolate user input into SQL strings.

### Command Injection

- Avoid `exec.Command` with user-controlled arguments when possible.
- If required, validate and sanitize each argument explicitly.

### Authentication/Authorization Boundaries

- Define authn/authz boundaries at the transport layer (middleware) and enforce at the domain layer.
- Don't rely solely on network-level security for multi-tenant data access.
- Verify that auth checks happen before any domain logic executes.

---

## 14. Testing Strategy

### Unit Tests

- Test domain logic in isolation. Use consumer-side interfaces for test seams.
- Mock only external dependencies; test real code paths where practical.
- Prefer table-driven tests.

### Table-Driven Tests

```go
func TestParse(t *testing.T) {
		tests := []struct {
				name    string
				input   string
				want    int
				wantErr bool
		}{
				{"positive", "42", 42, false},
				{"zero", "0", 0, false},
				{"negative", "-1", -1, false},
				{"invalid", "abc", 0, true},
		}
		for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
						got, err := Parse(tt.input)
						if (err != nil) != tt.wantErr {
								t.Errorf("Parse(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
						}
						if got != tt.want {
								t.Errorf("Parse(%q) = %d, want %d", tt.input, got, tt.want)
						}
				})
		}
}
```

### Fuzzing

Source: [go.dev/doc/tutorial/fuzz](https://go.dev/doc/tutorial/fuzz), accessed 2026-05-11.

```go
func FuzzParse(f *testing.F) {
		f.Add("42")
		f.Add("0")
		f.Fuzz(func(t *testing.T, input string) {
				_, err := Parse(input)
				if err != nil {
						return // valid error is fine
				}
				// If no error, Parse should return a valid result
				// Add property checks here
		})
}
```

### Race Detector

```bash
go test -race ./...
```

Run in CI for all packages. Race conditions are undefined behavior.

### Integration Tests with testcontainers [Community Convention]

[github.com/testcontainers/testcontainers-go](https://github.com/testcontainers/testcontainers-go)

- Use for testing against real databases, message brokers.
- Ensure integration tests are isolated and repeatable.
- Tag integration tests with build tags to separate from unit tests:

```go
//go:build integration

func TestPostgresStore(t *testing.T) { ... }
```

### Contract Tests

- Use for verifying API contracts between services.
- Tools: Pact [Community convention], Protobuf compatibility checks, OpenAPI diff.

### Benchmarks

```go
func BenchmarkEncode(b *testing.B) {
		for b.Loop() { // Go 1.24+ B.Loop
				_, _ = json.Marshal(largePayload)
		}
}
```

### Golden Files

Store expected output in files; compare against them in tests:

```go
func TestRender(t *testing.T) {
		got := render(input)
		want, err := os.ReadFile("testdata/render.golden")
		if err != nil { t.Fatal(err) }
		if string(got) != string(want) {
				t.Errorf("render mismatch")
				if *update { os.WriteFile("testdata/render.golden", got, 0644) }
		}
}
```

### httptest

Source: [pkg.go.dev/net/http/httptest](https://pkg.go.dev/net/http/httptest), accessed 2026-05-11.

```go
func TestHandler(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintln(w, "ok")
		})

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}
}
```

### Fake Clocks

Use `testing/synctest` (Go 1.25+, GA) for deterministic time-based concurrent testing:

```go
func TestExpiry(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
				cache := NewCache(5 * time.Second)
				cache.Set("key", "value")

				// Advance virtual time
				synctest.Wait() // wait for all goroutines to block

				_, ok := cache.Get("key")
				if !ok {
						t.Error("key should not have expired yet")
				}
		})
}
```

### Deterministic Concurrency Testing

`testing/synctest` (Go 1.25+) provides virtualized time for deterministic concurrent tests. Prior to Go 1.25, use build-tag-gated experimental version (`GOEXPERIMENT=synctest` in Go 1.24).

---

## 15. Code Review Checklist

### Correctness

- [ ] Are errors handled once with useful context?
- [ ] Are nil/zero values handled intentionally?
- [ ] Are boundary conditions tested?
- [ ] Are transactions committed or rolled back in all paths?
- [ ] Are HTTP status codes, retries, and idempotency correct?
- [ ] Are return values used before error checks? (Go 1.25+ may panic — review carefully.)

### API Design

- [ ] Is the API minimal and hard to misuse?
- [ ] Are exported names documented with useful doc comments?
- [ ] Are optional parameters handled via functional options, not maps?
- [ ] Is the package name meaningful (not `util`, `common`, `misc`)?

### Concurrency

- [ ] Any data races? Run `go test -race`.
- [ ] Does every goroutine have cancellation, backpressure, and a wait path?
- [ ] Are channels closed by the sender/owner only?
- [ ] Is shared state protected by mutex/atomic/channel ownership?
- [ ] Are timers/tickers stopped when no longer needed?
- [ ] Is context propagated through all layers?

### Performance

- [ ] Are performance claims backed by benchmarks/profiles?
- [ ] Are hot paths allocation-heavy? Check with `-benchmem` and pprof.
- [ ] Are HTTP transports and DB pools reused?
- [ ] Are logs avoiding expensive computation when disabled?
- [ ] Are caches bounded and observable?
- [ ] Is struct field ordering considered for hot structs?

### Security

- [ ] Any secrets in logs, errors, metrics, traces, panics, or config dumps?
- [ ] Is `math/rand` avoided for keys/tokens? (Official CodeReviewComments §5)
- [ ] Is user input validated and encoded at boundaries?
- [ ] Has `govulncheck` been run?
- [ ] Is `unsafe` justified and isolated?
- [ ] Are SQL queries parameterized?
- [ ] Is TLS verification not disabled?

### Architecture

- [ ] Are dependencies explicit (no globals, no service locators)?
- [ ] Are interfaces small and placed near consumers?
- [ ] Are package boundaries coherent (no god packages)?
- [ ] Is the domain separated from transport/storage where valuable?
- [ ] Is `main.go` small and wiring-focused?
- [ ] Is there a bounded context separation for complex domains?

### Dependency Injection

- [ ] Are dependencies injected via constructors, not globals?
- [ ] Are interfaces defined at the consumer side?
- [ ] Is config injected as typed structs?
- [ ] Is lifecycle managed explicitly (start, stop)?
- [ ] Are test seams natural (no monkeypatching)?

### Tests

- [ ] Unit tests for domain logic?
- [ ] Table-driven tests where appropriate?
- [ ] Fuzz tests for parsers and validators?
- [ ] Integration tests for DB/HTTP boundaries?
- [ ] Benchmarks for hot paths?
- [ ] Race detector run in CI?
- [ ] `testing/synctest` for time-dependent concurrent code?

### Observability

- [ ] Logs include action, outcome, identifiers, error context?
- [ ] Metrics cover latency, errors, saturation, queue depth, dependency calls?
- [ ] Traces cross service boundaries with correlation IDs?
- [ ] Health/readiness endpoints reflect real serving ability?

### Operations

- [ ] Graceful shutdown implemented?
- [ ] Migrations are versioned, tested, and reversible?
- [ ] Feature flags documented with expiry? [Community convention]
- [ ] Runbook exists and is current?
- [ ] Container resource limits match `GOMEMLIMIT`?

### Maintainability

- [ ] Is naming clear and idiomatic?
- [ ] Is code formatted by `gofmt`?
- [ ] Are exported identifiers documented?
- [ ] Is reflection/generic abstraction justified?
- [ ] Are package-level variables immutable or clearly safe?
- [ ] Is the PR size reasonable? Large PRs should be split.

---

## 16. Skill Behavior Instructions

### How an AI Using This Skill Should Review Go Code

1. **Determine scope**: Read the diff/file and identify which checklist categories are relevant. Not every PR requires deep scrutiny on every dimension.

2. **Apply severity levels**:

| Severity | Meaning | Action |
|----------|---------|--------|
| 🔴 **Critical** | Bug, security vulnerability, data loss, data race, goroutine leak | Must fix before merge |
| 🟠 **High** | Incorrect error handling, missing timeout, unbounded concurrency, leaky abstraction | Should fix before merge |
| 🟡 **Medium** | Non-idiomatic Go, missing test, over-engineered interface, missing doc comment | Discuss; fix recommended |
| 🔵 **Low** | Style preference, minor naming, micro-optimization without evidence | Optional; mention but don't block |
| ⚪ **Info** | Notable pattern, reference to docs, suggestion for future work | Informational; no action required |

3. **Output format for code review findings**:

```
### Finding: [Title]
- **Severity**: 🔴 Critical | 🟠 High | 🟡 Medium | 🔵 Low | ⚪ Info
- **Category**: Correctness | Concurrency | Performance | Security | Architecture | DI | Tests | Observability | Operations | Maintainability
- **Location**: file.go:L42
- **Description**: What the issue is and why it matters.
- **Suggestion**: Concrete fix or improvement.
- **Source**: Official doc link or [Community convention] label.
```

4. **When to request more context**:
	 - When a diff shows a function signature but not the struct/interface it belongs to.
	 - When concurrency patterns depend on call-site behavior not visible in the diff.
	 - When business invariants are referenced but not documented.
	 - When an interface has only one implementation in the diff but may be consumed elsewhere.
	 - When performance claims lack benchmark evidence.

5. **Distinguish official vs community**:
	 - Prefix advice from official Go docs with "Official:" and cite the source.
	 - Prefix community advice with "[Community convention]" and name the source.
	 - Prefix uncertain claims with "[Unverified]" and explain what was checked.

6. **Be proportional**: A small bugfix PR does not need a full architecture review. A large feature PR deserves systematic coverage.

---

## 17. References

### Official Go Sources

| Resource | URL | Notes |
|----------|-----|-------|
| Go Downloads | [go.dev/dl](https://go.dev/dl/) | Latest stable releases; accessed 2026-05-11 |
| Release History | [go.dev/doc/devel/release](https://go.dev/doc/devel/release) | All releases; accessed 2026-05-11 |
| Go 1.26 Release Notes | [go.dev/doc/go1.26](https://go.dev/doc/go1.26) | Current major release; accessed 2026-05-11 |
| Go 1.25 Release Notes | [go.dev/doc/go1.25](https://go.dev/doc/go1.25) | Prior major release; accessed 2026-05-11 |
| Go 1.24 Release Notes | [go.dev/doc/go1.24](https://go.dev/doc/go1.24) | Prior major release; accessed 2026-05-11 |
| Go 1.23 Release Notes | [go.dev/doc/go1.23](https://go.dev/doc/go1.23) | Prior major release; accessed 2026-05-11 |
| Effective Go | [go.dev/doc/effective_go](https://go.dev/doc/effective_go) | Official but not actively updated; accessed 2026-05-11 |
| Code Review Comments | [go.dev/wiki/CodeReviewComments](https://go.dev/wiki/CodeReviewComments) | Last edited Dec 2023; still canonical; accessed 2026-05-11 |
| Go Specification | [go.dev/ref/spec](https://go.dev/ref/spec) | Language version go1.26; accessed 2026-05-11 |
| Go Memory Model | [go.dev/ref/mem](https://go.dev/ref/mem) | Concurrency semantics; accessed 2026-05-11 |
| Modules Reference | [go.dev/ref/mod](https://go.dev/ref/mod) | Module system docs; accessed 2026-05-11 |
| Diagnostics | [go.dev/doc/diagnostics](https://go.dev/doc/diagnostics) | Profiling, tracing, debugging; accessed 2026-05-11 |
| Profiling Go Programs | [go.dev/blog/pprof](https://go.dev/blog/pprof) | Official pprof blog; accessed 2026-05-11 |
| Fuzzing Tutorial | [go.dev/doc/tutorial/fuzz](https://go.dev/doc/tutorial/fuzz) | Official fuzzing guide; accessed 2026-05-11 |
| Go Vulnerability Management | [go.dev/security/vuln](https://go.dev/security/vuln/) | govulncheck; accessed 2026-05-11 |
| `govulncheck` | [pkg.go.dev/golang.org/x/vuln/cmd/govulncheck](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck) | Accessed 2026-05-11 |
| `context` | [pkg.go.dev/context](https://pkg.go.dev/context) | Go 1.26.3 docs; accessed 2026-05-11 |
| `log/slog` | [pkg.go.dev/log/slog](https://pkg.go.dev/log/slog) | Go 1.26.3 docs; accessed 2026-05-11 |
| `slog` blog | [go.dev/blog/slog](https://go.dev/blog/slog) | Official slog intro; accessed 2026-05-11 |
| `net/http` | [pkg.go.dev/net/http](https://pkg.go.dev/net/http) | Go 1.26.3 docs; accessed 2026-05-11 |
| `database/sql` | [pkg.go.dev/database/sql](https://pkg.go.dev/database/sql) | Go 1.26.3 docs; accessed 2026-05-11 |
| `encoding/json` | [pkg.go.dev/encoding/json](https://pkg.go.dev/encoding/json) | Go 1.26.3 docs; accessed 2026-05-11 |
| `testing` | [pkg.go.dev/testing](https://pkg.go.dev/testing) | Go 1.26.3 docs; accessed 2026-05-11 |
| `testing/synctest` | [pkg.go.dev/testing/synctest](https://pkg.go.dev/testing/synctest) | Go 1.26.3 docs; accessed 2026-05-11 |
| `runtime/debug` | [pkg.go.dev/runtime/debug](https://pkg.go.dev/runtime/debug) | GC tuning; accessed 2026-05-11 |
| `runtime/pprof` | [pkg.go.dev/runtime/pprof](https://pkg.go.dev/runtime/pprof) | Profiling; accessed 2026-05-11 |
| `runtime/trace` | [pkg.go.dev/runtime/trace](https://pkg.go.dev/runtime/trace) | Execution tracer; accessed 2026-05-11 |
| `runtime/metrics` | [pkg.go.dev/runtime/metrics](https://pkg.go.dev/runtime/metrics) | Runtime metrics; accessed 2026-05-11 |
| `sync.Pool` | [pkg.go.dev/sync#Pool](https://pkg.go.dev/sync#Pool) | Accessed 2026-05-11 |
| Pipelines Blog | [go.dev/blog/pipelines](https://go.dev/blog/pipelines) | Concurrency patterns; accessed 2026-05-11 |
| Context Blog | [go.dev/blog/context](https://go.dev/blog/context) | Official context usage; accessed 2026-05-11 |
| Managing Dependencies | [go.dev/doc/modules/managing-dependencies](https://go.dev/doc/modules/managing-dependencies) | Accessed 2026-05-11 |
| Workspaces Tutorial | [go.dev/doc/tutorial/workspaces](https://go.dev/doc/tutorial/workspaces) | Accessed 2026-05-11 |

### Community / Industry Sources

| Resource | URL | Status |
|----------|-----|--------|
| Uber Go Style Guide | [github.com/uber-go/guide/master/style.md](https://raw.githubusercontent.com/uber-go/guide/master/style.md) | [Community convention]; accessed 2026-05-11 |
| Google Go Style Guide | [google.github.io/styleguide/go/guide.html](https://google.github.io/styleguide/go/guide.html) | [Community convention]; accessed 2026-05-11 |
| Dave Cheney — Practical Go | [dave.cheney.net/practical-go](https://dave.cheney.net/practical-go/presentations/qcon-china.html) | [Community convention]; accessed 2026-05-11 |
| Dave Cheney — Error Handling | [dave.cheney.net/2016/04/27/dont-just-check-errors-handle-them-gracefully](https://dave.cheney.net/2016/04/27/dont-just-check-errors-handle-them-gracefully) | [Community convention]; accessed 2026-05-11 |
| Dave Cheney — Zen of Go | [dave.cheney.net/2020/02/23/the-zen-of-go](https://dave.cheney.net/2020/02/23/the-zen-of-go) | [Community convention]; accessed 2026-05-11 |
| OpenTelemetry Go Docs | [opentelemetry.io/docs/languages/go/](https://opentelemetry.io/docs/languages/go/) | Accessed 2026-05-11 |
| Kubernetes Probes | [kubernetes.io/docs/concepts/configuration/liveness-readiness-startup-probes/](https://kubernetes.io/docs/concepts/configuration/liveness-readiness-startup-probes/) | Accessed 2026-05-11 |
| OWASP Logging Cheat Sheet | [cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html) | Accessed 2026-05-11 |
| Transactional Outbox Pattern | [microservices.io/patterns/data/transactional-outbox.html](https://microservices.io/patterns/data/transactional-outbox.html) | [Community/industry pattern]; accessed 2026-05-11 |
| Google Wire | [github.com/google/wire](https://github.com/google/wire) | [Community convention]; accessed 2026-05-11 |
| Uber Fx | [go.uber.org/fx](https://go.uber.org/fx) | [Community convention]; accessed 2026-05-11 |
| testcontainers-go | [github.com/testcontainers/testcontainers-go](https://github.com/testcontainers/testcontainers-go) | [Community convention]; accessed 2026-05-11 |
| SLSA Levels | [slsa.dev/spec/v1.0/levels](https://slsa.dev/spec/v1.0/levels) | Accessed 2026-05-11 |

---

*This skill document was verified against official Go sources accessed on 2026-05-11. Community conventions are labeled as such. Uncertain claims are labeled [Unverified]. All code examples are illustrative and may need adaptation to specific project contexts.*