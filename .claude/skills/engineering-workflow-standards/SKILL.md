---
name: engineering-workflow-standards
description: Use when implementing features, refactors, or bugfixes - work spans architecture decisions, multi-file changes, or anything beyond a trivial one-line edit. Establishes mandatory exploration, delegation, testing, and final-review discipline for the user.
---

# Engineering Workflow Standards

## Overview

The user's standing rules for any non-trivial engineering task. **Pillar 0 (Requirement Fidelity)** comes before everything. Then five workflow pillars: **fresh branch from updated `main`**, **understand the hot path before exploring**, **delegate to the right subagent at the right model tier**, **write the necessary backend/frontend unit tests before review**, **never skip the final architect review**.

**Violating the letter of these rules is violating the spirit.** "I already understand the project", "this is small enough to skip review", "manual testing is enough", or **"the user surely meant the canonical pattern, not their literal spec"** are rationalizations — see the table below.

## When to Use

**Apply when:**
- Implementing any feature, refactor, or bugfix that touches more than one file
- Making any architecture or design decision
- Writing new modules, packages, services, or endpoints
- Modifying domain logic, persistence, or public APIs
- The user gave a task description without explicitly opting out of this workflow

**Skip when:**
- Pure documentation edit (typos, wording)
- One-line config tweak with no logic
- The user explicitly says "just do X quickly" or "skip the review"

## The Pillars

### 0. Requirement Fidelity (Pillar Zero — Comes Before Everything)

**The user's stated requirement is literal until proven metaphorical.** When the user describes a format, a protocol, an identifier shape, an algorithm, a wire structure, a UI affordance, or any other concrete artifact — implement that artifact **exactly as described**, even when:

- The "canonical" alternative looks cleaner.
- A standard library / framework primitive is available and "battle-tested."
- It feels suboptimal, ugly, redundant, or naive.
- It would take longer or be more complex to implement.
- You are tempted to "improve" or "modernize" the spec.

**Examples of literal specs you must NOT silently substitute:**

| User said | Common (wrong) substitution | Why the substitution is a violation |
|---|---|---|
| "Channel ID format `01-237-XXXXXX`" | UUID — easier, FK-friendly | The user designed the format for a reason (token cost, routability, parseability). UUID erases that. |
| "Use SHA-1 for the cache key (the system needs to match what's already there)" | SHA-256 — "more secure" | The user constrained the algorithm by external compatibility. Substituting breaks compat. |
| "Numeric ID starting from 1" | UUID / nanoid | The user wants human-readable IDs for ops/logs. |
| "Implement a state machine with states A, B, C" | Boolean flag — "simpler" | The user wants explicit transitions; collapsing to a flag silently loses an axis. |
| "Reply must be in JSON Lines format" | JSON array — "more standard" | The user designed for streaming/append; an array breaks that property. |

**The discipline:**

1. **Treat every concrete artifact in the spec as literal.** Format strings, prefix conventions, algorithm names, protocol names, version numbers, file paths, URL shapes, table names, column names, error message texts — all literal until the user explicitly says "or equivalent."
2. **If the literal spec looks suboptimal, surface the trade-off and ask** — do not silently choose the "better" alternative. The user may have constraints you cannot see (token cost, third-party compatibility, regulatory, branding, ops familiarity, future plans).
3. **If a subagent or your own draft drifts from the literal spec, flag it.** Even if the test still passes. Even if the diff is smaller. Drift on the spec compounds; it is not "polish."
4. **The test of fidelity is round-trip:** could a third party, given the implementation, reconstruct the user's exact words for the contested artifact? If not, you drifted.

**Red flag thoughts that mean STOP:**
- "It's just a metaphor / illustrative example."
- "User probably meant the concept, not the literal text."
- "I'll use UUID/standard-X — it's a strict superset / more battle-tested."
- "We can revisit the format later."
- "The literal spec would be ugly / not idiomatic / harder to validate."
- "This will be faster / simpler if I substitute."

**When in doubt, ask.** Never substitute silently. **Time and complexity are NEVER valid reasons to deviate from a literal user spec.** The cost of asking is one round-trip; the cost of silent drift is a full rewrite plus the user's trust.

This pillar is enforced by the final review (Pillar 6): the architect must verify literal fidelity, not just "intent."

### 1. Fresh Branch from Updated `main` (Mandatory, First Step)

**Before any exploration or code reading**, create a fresh working branch from an up-to-date `main`. The sequence is rigid:

```bash
git checkout main
git pull --ff-only origin main      # fail loudly if main has diverged locally
git checkout -b <type>/<short-slug> # e.g. fix/sandbox-readonly-fs, feat/inbox-migrate
```

Rules:
- Never start work on `main`, on a stale branch, or on a branch from a previous task.
- If `git status` is dirty, stop and ask the user — do not stash or discard.
- If `pull --ff-only` fails, stop and ask — do not force or reset without permission.
- The branch name must reflect the task (`fix/`, `feat/`, `refactor/`, `chore/`, `docs/`).

### 2. Understand the Hot Path Before Exploring (Avoid False Positives)

Before dispatching the Explore subagent, you must form a precise hypothesis of **the hot path of the problem** — the exact code path, request flow, or call chain where the bug lives or where the feature must plug in.

Steps:
1. Re-read the user's task description and pin down: input → which entry point → which layer(s) → which output/side effect.
2. Identify the *symptom* vs. the *suspected cause*. Do not conflate them.
3. Write the hot path explicitly (mentally or in a brief plan): "Request hits handler X → service Y → adapter Z → DB. Symptom is at Z, but root cause is likely at Y."
4. Only then dispatch Explore — and tell it which hot path to validate, not just "study the project."

**Why this matters:** skipping this step produces *false positives* — fixes that touch unrelated code, "improvements" the user didn't ask for, or surface-level patches that miss the real cause. If you cannot articulate the hot path, you are not ready to explore. Ask the user for clarification first.

### 3. Explore (Mandatory, After Branch + Hot Path)

Dispatch the **Explore** subagent (or `general-purpose` if domain-specific) to validate the hot path and surface relevant files, modules, patterns, and conventions. Output: confirmed hot path + the existing code that must be respected or modified.

Never start coding from assumptions about the codebase. Even if you "remember" the project from a previous session, re-explore — code drifts.

### 4. Delegate Development to Subagents at the Right Tier

Pick the model tier by complexity, not by convenience:

| Complexity | Model | Use for |
|------------|-------|---------|
| Simple, mechanical, well-scoped | `claude-haiku-4-5` | Boilerplate, small CRUD, isolated bug fixes, test scaffolding |
| Medium, multi-file, requires reasoning | `claude-sonnet-4-6` | Most feature work, refactors, integration code |
| Very complex, cross-cutting, design-heavy | `claude-opus-4-7` | Architecture changes, hard concurrency/distributed problems, core domain redesigns |

Dispatch via the `Agent` tool with the `model` parameter set explicitly. Default to Sonnet 4.6 when unsure — do **not** default to Opus.

**Subagent selection by domain:**

| Domain | Subagents |
|--------|-----------|
| Backend (architecture, design) | `principal-software-architect` |
| Backend (implementation) | `software-engineer` |
| Frontend | `senior-product-engineer-front` |
| Data / pipelines | `data-engineer` or `principal-data-architect` |
| Security / DevSecOps | `principal-ai-devsecops-architect` |

### 5. Unit Tests Before Final Review (Non-Negotiable)

**Before dispatching the final architect review**, the implementation MUST be accompanied by the necessary unit tests covering the changes — both on the **backend** (domain logic, services, adapters, handlers, idempotency, error paths, security checks) and on the **frontend / interface** (components, hooks, state, user interactions, accessibility, edge cases).

Rules:
- Tests live alongside the production code and follow the project's existing testing conventions (framework, naming, structure, fixtures, mocks).
- New behavior, branches, and bugfixes must each have at least one corresponding unit test that **would fail without the change** (red → green).
- Tests must actually run and pass locally before the review is requested — no `skip`, no `todo`, no commented-out assertions, no `t.Skip` / `it.skip` / `xit` / `xdescribe`.
- Coverage focus is on **behavior**, not lines: assert observable contracts (inputs, outputs, side effects, error returns), not internal implementation details.
- If a layer is genuinely untestable as-is (e.g. tightly coupled legacy code, hidden dependencies), the subagent must refactor enough to make it testable rather than skip the test.
- "There's no time", "the change is too small", "manual testing is enough", or "the integration tests already cover it" are **not** valid reasons to skip unit tests. If the user explicitly opts out, document it in the task report.
- Run the project's standard test commands (e.g. `go test ./...`, `npm test`, `pnpm test`, project-specific scripts) and only proceed to the review when they pass.

**Why this is non-negotiable:** the architect review presumes the change is already proven by tests. Sending unreviewed, untested code into the final review wastes the highest-tier reviewer's cycles, lets regressions slip in, and breaks the user's quality bar.

### 6. Final Review (Non-Negotiable)

After implementation is complete, **with the corresponding backend and frontend unit tests written and passing**, and before reporting the task as done, dispatch the **`principal-software-architect`** subagent with **`claude-opus-4-7`** to review the changes. The reviewer must run:

- `/requesting-code-review` or `/superpowers:requesting-code-review`
- `/golang-code-review` (when the changes touch Go code)

**Mandatory context for the reviewer:** the dispatch prompt MUST include the **user's original request verbatim** (the task description / problem statement as the user wrote it), so the reviewer can validate not only code quality but also that the **business rule** was correctly understood and implemented. The architect must explicitly answer: "Does the implementation actually solve what the user asked for?" — including acceptance criteria, edge cases mentioned by the user, and any constraints (functional, non-functional, regulatory) stated in the original request. Code that is technically clean but misses or misinterprets the business rule is a failed review.

**Mandatory fidelity gate — the reviewer MUST explicitly verify:** for every concrete artifact in the user's request (format, prefix, suffix, identifier shape, algorithm, protocol, file path, column name, error text, state-machine shape, etc.), does the implementation match the user's words **literally**? If the implementation chose a "canonical / cleaner / more battle-tested" alternative — UUID instead of a custom format, SHA-256 instead of SHA-1, an array instead of JSON Lines, a boolean instead of an enum, etc. — that is a **failed review** (Pillar 0 violation), regardless of code quality. The reviewer prompt must include this checklist:

- [ ] Did the implementation introduce any identifier, format, or shape that the user did not request?
- [ ] Did the implementation substitute any user-named primitive (algorithm, protocol, library) with another?
- [ ] Are all literal artifacts from the user's request (regex shapes, prefixes, literal numbers, file extensions, column types) present byte-exact in the code?
- [ ] If anything was changed for "cleanliness / convention / performance," is there an explicit ADR or user approval for the deviation?

A failed fidelity gate is a BLOCKER, not a MINOR. Send back even if everything else is green.

If the review surfaces issues — even minor ones, whether on code quality, business-rule fidelity, OR Pillar 0 fidelity — dispatch the appropriate subagent again to fix them. Re-review until clean. **Never let anything slip.**

## Engineering Principles (Inherited from `principal-software-architect`)

This skill **inherits** its engineering, architecture, and security standards from the project-local agent definition:

> `./.claude/agents/principal-software-architect.md`

That file is the single source of truth. Before starting any non-trivial work, **read it** and apply every principle it defines — including (non-exhaustive):

- **DDD**, **Hexagonal Architecture (Ports & Adapters)**, **Twelve-Factor App**, **Idempotency** in APIs/events
- **Secure by Design** and **Zero Trust**; **Threat Modeling**; OWASP Top 10 mitigations; encryption at rest and in transit; secrets management
- **Clean Code** + **SOLID** with relentless focus on **SRP**
- **DRY / KISS / YAGNI**
- **HA / RTO / RPO** awareness; SPOF elimination in distributed designs
- The agent's epistemic discipline: never present speculation as fact; label `[Inference]`, `[Speculation]`, or `[Unverified]` when applicable; never use absolute terms like *guarantee*, *prevent*, *eliminates* without a real source

If `./.claude/agents/principal-software-architect.md` does not exist in the current project, fall back to the principles listed above as defaults — but note its absence to the user.

If a subagent returns code that violates any of these, send it back. Do not patch over violations yourself in a hurry.

## Standard Flow

```
0.  Fidelity   → list every concrete artifact in the user's request (formats,
                 prefixes, identifier shapes, algorithms, protocols, file paths,
                 column names, error texts). Treat each as literal. If any is
                 unclear, ask BEFORE branching.
1.  Branch     → checkout main, pull --ff-only, create fresh <type>/<slug> branch
2.  Hot path   → articulate input→entry→layers→output; separate symptom from cause
3.  Explore    → dispatch Explore subagent to validate hot path + surface conventions
4.  Plan       → if non-trivial, write a brief plan grounded in exploration findings.
                 The plan MUST restate the literal artifacts from step 0.
5.  Implement  → dispatch domain subagent at correct model tier. The dispatch
                 prompt MUST quote the literal artifacts from step 0 verbatim.
6.  Unit tests → write the necessary backend AND frontend unit tests (NON-NEGOTIABLE)
7.  Verify     → run tests/build/lint; tests must pass; re-dispatch if anything fails.
                 ALSO grep the diff for any deviation from the artifacts in step 0
                 (e.g. UUID where literal format was specified, SHA-256 where SHA-1
                 was specified). Send back to subagent if drift found.
8.  Review     → dispatch principal-software-architect (Opus) with the two review
                 skills AND the fidelity-gate checklist (see Pillar 6).
9.  Fix        → re-dispatch implementation subagent for any review findings,
                 including Pillar 0 (fidelity) violations.
10. Report     → only now mark the task complete.
```

## Red Flags — STOP if you catch yourself thinking:

| Rationalization | Reality |
|-----------------|---------|
| "I already know this project" | Re-explore. Code drifts between sessions. |
| "This is too small for the architect review" | The user said "don't let anything pass." Run the review. |
| "Sonnet is overkill, I'll just do it inline" | Delegate. Inline work bypasses the discipline. |
| "Opus would do this better, I'll use it for everything" | Opus only for genuinely complex work. Default to Sonnet. |
| "The review will probably say it's fine" | Then it costs little to confirm. Run it. |
| "I'll skip `/golang-code-review` — it's only one Go file" | Touched Go = run it. |
| "Subagent output is good enough, ship it" | If it violates SRP/Clean Arch, send it back. |
| "I'll just work on the current branch, it's clean" | Always cut a fresh branch from updated `main`. No exceptions. |
| "main is probably up to date" | `git pull --ff-only` proves it. Don't assume. |
| "I roughly know where the bug is, let's start exploring" | "Roughly" produces false positives. Articulate the hot path first. |
| "The symptom is at file X, so the fix is at file X" | Symptom ≠ cause. Trace the hot path back to the root. |
| "The change is small, no need for unit tests" | Small changes break things too. Write the test. |
| "Manual testing already covered it" | Manual testing is not reproducible and does not gate the review. |
| "Integration/E2E tests already exercise this path" | Unit tests are still required for the new logic. They are complementary, not substitutes. |
| "Backend is tested, frontend is just UI" | Frontend logic, hooks and components require unit tests too. |
| "I'll add tests after the review" | The review presumes tests exist and pass. Order is: tests → review. |
| "The legacy code is impossible to test" | Refactor enough to make it testable, then test it. |
| "The reviewer doesn't need the user's original request" | Without it, the architect can't validate the business rule. Always pass the verbatim request. |
| "I'll just summarize what the user asked" | Summaries lose acceptance criteria, edge cases and constraints. Pass the original prompt verbatim. |
| "The user-specified format is illustrative / a metaphor" | It is literal until they say otherwise. Implement byte-exact, then ask if you doubt. |
| "UUID / SHA-256 / standard-X is a strict superset, no harm" | The user chose the spec for reasons you cannot see (token cost, compat, regulatory). Substituting silently is a Pillar 0 violation. |
| "The literal spec is ugly / not idiomatic — I'll polish" | Polish on user-supplied artifacts is drift. Ask before "improving." |
| "Implementing literally would take longer / be more complex" | Time and complexity are NEVER valid reasons to deviate from a literal user spec. |
| "We can revisit the exact format in a follow-up PR" | No. Get it right the first time. Silent drift compounds; the follow-up is a rewrite. |
| "The subagent gave me UUID-based code, I'll just merge — close enough" | Close-enough on user-supplied artifacts is a failed review. Send it back. |

## Common Mistakes

- **Starting work on `main` or a stale branch** — always cut a fresh branch from a freshly pulled `main` first
- **Skipping the hot-path articulation** — leads to false positives and unrelated edits
- **Confusing symptom and cause** — the file where the error surfaces is rarely the file that needs the fix
- **Coding before exploring** — produces code that fights existing patterns
- **Picking model tier by habit (always Sonnet, always Opus)** — match tier to actual complexity
- **Sending code to the final review without unit tests** — backend AND frontend unit tests must exist and pass first
- **Adding tests that pass without exercising the change** — every new behavior needs a test that would fail without the implementation
- **Leaving skipped/todo tests behind** — `skip`, `todo`, `xit`, `t.Skip` are not allowed in the final state
- **Treating the final review as optional** — it is the user's hard requirement
- **Accepting first subagent output without critical reading** — review every diff before trusting it
- **Forgetting to pass the two review skills to the architect** — the architect must run both (`/requesting-code-review` and `/golang-code-review` for Go)
- **Dispatching the final review without the user's original request** — the reviewer must validate the business rule against the verbatim task description, not just the diff
- **Treating user-supplied formats / identifiers / protocols as approximate** — every concrete artifact in the user's request (format string, prefix, algorithm, identifier shape, file extension, column name, error text) is **literal**. Substituting with a "canonical" alternative because it's "easier / cleaner / more standard" is a Pillar 0 violation, even if tests pass.
- **Silent substitution by subagents** — when a subagent returns a diff that drifts from the user's literal spec, flag it before merging. Do NOT polish drift into "good enough." Send it back to be redone literally.
- **Skipping the fidelity gate in the final review** — the architect must explicitly answer "did we implement the user's literal artifacts byte-exact?" not just "does it work?"
