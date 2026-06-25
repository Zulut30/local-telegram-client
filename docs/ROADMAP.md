# Local Telegram Client — Engineering Roadmap

> Forward-looking, actionable plan. The original milestone spec lives in
> [`PLAN.md`](../PLAN.md) (historical); the public goal table lives in the
> [`README.md`](../README.md). This document turns those goals into concrete,
> prioritized engineering work with acceptance criteria.
>
> Last updated: 2026-06-26. Derived from a five-dimension code audit
> (correctness, Bot API fidelity, security/ops, frontend/UX, testing/architecture)
> with adversarial verification of every finding.

## 1. Where the project is today

`local-telegram-client` is a single Go binary that emulates the Telegram Bot API
(`/bot<token>/<method>`) plus a `go:embed`-bundled React IDE. The core developer
loop — inject user input → bot polls `getUpdates` (or receives a webhook) → bot
calls `send*`/`edit*` → trace correlation → SSE → browser — works and is covered
by integration tests.

**Solid foundations**

- Framework/language-agnostic: works at the HTTP protocol level, so any SDK works.
- Stateful chat loop, long-poll + webhook delivery, in-memory media, trace
  correlation with an explicit `inferred` flag.
- Bot API 10.1 method registry with case-insensitive routing, compat/strict modes,
  and a `/_sim/coverage` matrix.
- Clean dependency budget (stdlib + `telego` as a dev/test dep only).

**Audit headline (verified)**

| Severity | Count | Notes |
|---|---:|---|
| Critical | 2 | 1 backend critical was a **false positive** (see §2); 1 real (remote-mode token) |
| High | 27 | |
| Medium | 27 | |
| Low | 11 | |

Two audit findings were rejected during verification (no real bug). Two more were
overridden by checking the actual Telegram spec (see §2).

## 2. Audit corrections (do NOT "fix" these)

Checking the authoritative `telego` typed signatures and the Bot API spec
disproved several findings. They are recorded here so nobody re-opens them:

- **`copyMessage` → `MessageId`** (`{message_id}`), **`copyMessages` → `[]MessageId`**,
  **`forwardMessages` → `[]MessageId`**. The current code already returns these exact
  shapes. The audit's "return full Message" claim (incl. the only backend *critical*)
  is wrong. `forwardMessage` correctly returns a full `Message`.
- **Strict-mode `HTTP 501`** is an intentional, documented simulator signal
  (`--api-mode strict`), not an HTTP-semantics bug. Keep it; it has test coverage.
- `editMessageCaption` / `editMessageMedia` **should** return a `Message` (not `true`)
  — this one is real and is in M1 below.

## 3. Prioritization model

Work is ordered by **(impact on a real bot author) × (likelihood) ÷ (risk)**:

1. **P0 — Blocks a documented use case** (remote mode unusable, SSE drops).
2. **P1 — Fidelity/correctness a real bot will hit** (reply context, edit returns).
3. **P2 — Hardening, tests, ops** (DoS limits, SSRF guard, coverage).
4. **P3 — Roadmap features** (persistence, scenario record/replay, payments…).
5. **P4 — Polish** (a11y, theming, container hygiene).

---

## 4. Milestones

### M1 — Stabilization & fidelity (maps to G1/G2) — *in progress*

The first pass; closes the P0/P1 items that make the tool unreliable or
subtly wrong. Each item references the audit finding.

**Correctness / ops (P0–P2)**

- [x] SSE write-timeout: disable the per-connection write deadline on
  `/_sim/events` via `http.ResponseController` so long-lived streams survive the
  server `WriteTimeout` (F11).
- [x] `/version` endpoint + build info via `-ldflags`/`debug.ReadBuildInfo` (F48).
- [x] Constant-time token comparison (`subtle.ConstantTimeCompare`) (F62).
- [x] `http.MaxBytesReader` body limit on JSON/form bodies; reuse the 32 MiB
  multipart cap as a single configurable limit (F18).
- [x] Security headers on `/_sim/file/{id}` downloads: `X-Content-Type-Options:
  nosniff`, `Content-Disposition` already set; add `X-Frame-Options`/CSP for the UI (F66).
- [x] `--persist` is currently silently ignored — make it honest (clear "reserved"
  error, mirroring `--log-file`) until M5 implements it (F46/F47/F53).
- [x] Recorder timer cleanup on ring eviction (stop+delete orphaned timers) (F37).
- [x] `showcase-bot` webhook handler body limit (F50).

**Bot API fidelity (P1)**

- [x] Populate `reply_to_message` in returned `Message` when `reply_to_message_id`
  resolves to a stored message; add the field to `tg.Message` (F3/F9).
- [x] `editMessageCaption` / `editMessageMedia` return the edited `Message` (F4).
- [x] Accept and echo `message_thread_id`, `business_connection_id`,
  `link_preview_options` on send methods + add fields to `tg.Message` (F30/F31/F32).
- [x] `inline_message_id` path in edit methods → return `true` instead of 400 (F7).
- [x] `getUpdates` honors `allowed_updates` (filter message vs callback_query) (F5).
- [x] `editMessageText` no longer clobbers unspecified fields (F6).
- [x] `getFile` returns a `file_path` that is downloadable through a Telegram-style
  `GET /file/bot<token>/<file_path>` route, so SDK `getFile` → download works end-to-end (F33).
- [x] `reply_parameters` (modern reply API) accepted alongside `reply_to_message_id` (F35).
- [x] SSRF guard: in remote mode, `setWebhook` rejects URLs resolving to
  private/loopback/link-local addresses unless `--allow-private-webhooks` is set (F19, pulled
  forward from M6).

**Tests (P2)**

- [x] `internal/trace`: unit tests for the correlation state machine — long-poll
  flush boundary, webhook close, orphan calls, ring eviction, timer TTL (F20).
- [x] `internal/store`: table-driven tests for inject/getUpdates offset semantics,
  edit/delete, reset (F21, also pins down the F36 "off-by-one" non-bug).
- [x] `internal/events`: hub broadcast, slow-subscriber drop, unregister (F22).
- [x] `internal/webhook`: delivery success/failure, error tracking, info (F24).
- [x] `internal/tg`: JSON (de)serialization round-trips for Update/Message (F51).

**Frontend resilience (P0/P1)**

- [x] Remote-mode token bootstrap: read `?token=` / a `<meta>` tag, persist in
  `sessionStorage`, attach to every `fetch` (`X-Sim-Token`) and to the
  `EventSource` URL (`?token=`). Without this the IDE is unusable in remote mode (F2).
- [x] SSE reconnect → re-sync: on `EventSource` `open`/reconnect, re-`refresh()`
  state and traces so no events are lost across the gap (F12/F16).
- [x] React error boundary around the app shell (F15).
- [x] Theme persistence in `localStorage` (F39).
- [x] Guard `JSON.parse` in every SSE handler (F42).
- [x] Clean up `setTimeout` handles for chat-action/draft expiry on unmount (F14).
- [x] Bound `FormattedText` recursion depth (F17).
- [x] Unique React keys for keyboard buttons (F43); `FileReader` error handling (F44).

**CI / packaging (P2/P4)**

- [x] CI: `tsc --noEmit` typecheck for the frontend; `govulncheck` for Go (F40/F64).
- [x] Docker: run as non-root `USER`, add `HEALTHCHECK` hitting `/healthz` (F63/F65).

**M1 acceptance:** `make build-frontend && make build`, `go vet ./...`,
`go test ./...` all green; remote mode IDE works end-to-end with a token; SSE
survives > 60s idle; new unit tests cover the trace/store/hub/webhook packages.

---

### M2 — Testing IDE: scenario record & replay (maps to G3)

Turn the emulator into a regression harness. This is the biggest product lever
after stabilization and needs the architectural seams below.

- **Event-log abstraction** (F27): a canonical, append-only log of control-plane
  injections + bot Bot API calls + outcomes, independent of the in-memory store.
- **Recorder interface** (F54): put `*trace.Recorder` behind an interface so the
  event log and alternative sinks can subscribe.
- **`POST /_sim/scenarios` record / `…/replay`**: capture a session (ordered
  updates + expected bot responses) to JSON; replay it headlessly against a bot
  with no network, asserting on the bot's calls.
- **Assertions**: declarative expectations (`expect sendMessage with text ~= …`,
  latency budget, call order) surfaced in the IDE and as a CI-friendly exit code.
- **API explorer panel** in the IDE: pick a method, fill params, fire, see the
  trace — backed by the coverage matrix.

**Acceptance:** record a showcase session → replay in a Go test with zero network
→ deterministic pass; a failing assertion returns non-zero from a `sim replay` CLI.

---

### M3 — Persistence & artifacts (maps to G4)

- **Persistence interface** (F28) with two impls: `memory` (default) and a
  file-backed store. Wire the now-honest `--persist` flag to it (F46/F53).
- Start with a **JSON snapshot** (zero new runtime deps, cross-compiles cleanly);
  evaluate `modernc.org/sqlite` (pure-Go) only if query/scale needs justify it.
- **Media directory** option (spill large uploads to disk; in-memory stays default).
- **Import/export** of a session (chats + messages + traces + media manifest).

**Acceptance:** restart with `--persist` retains chats/messages/traces; export a
session and re-import it byte-stable.

---

### M4 — Bot API depth: payments, Stars, Mini Apps, inline mode (maps to G2)

Make the high-value "not yet semantic" stubs stateful, prioritized by real usage:

1. **Inline mode**: inject inline queries from the IDE; `answerInlineQuery`
   renders results; selecting a result produces the chosen-result update.
2. **Payments / Stars**: `sendInvoice` → pre-checkout → successful-payment update
   lifecycle; `getStarTransactions`/balance backed by an in-memory ledger.
3. **Mini Apps**: `answerWebAppQuery`, `savePreparedInlineMessage` round-trips.
4. **Reactions, polls** stored statefully; `sendMediaGroup` rendered as a true
   album group in the UI (F34).

Each method graduates from `compatibility_stub`/`not_yet_semantic` to `stateful`
in `coverage.go`, with an integration test and a coverage-matrix bump.

**Acceptance:** the coverage summary's `stateful` count rises; a payments and an
inline-mode bot run end-to-end against the sim.

---

### M5 — Observability & trace tooling (maps to G5)

- Trace **search/filter** (by method, chat, status, latency) in the IDE.
- **JSONL export** of traces; `/_sim/traces` pagination.
- **Metrics**: `/metrics` (Prometheus text) for call counts, latency histogram,
  error rate, SSE subscriber count.
- Structured ops logs already use `slog`; wire `--log-file` (currently rejected)
  to `lumberjack` rotation (the PLAN's optional dependency).

**Acceptance:** filter to error traces in the UI; scrape `/metrics`; rotate logs.

---

### M6 — Self-hosted production hardening (maps to G6)

- ~~**SSRF guard** for `setWebhook` (F19)~~ — done early in M1.
- Optional **per-IP rate limiting** and request-size config on `/_sim/*`.
- **HSTS** + security headers in remote mode behind TLS (F45).
- Hardened systemd unit + Caddy examples; container image SBOM + `govulncheck` gate.
- Coverage gate in CI (F52); table-driven refactors of integration tests (F67).

**Acceptance:** remote mode passes a basic abuse test (oversized body, SSRF attempt,
flood) without falling over; CI enforces a coverage floor.

---

### M7 — Proxy / live-bot inspector (PLAN M7, future)

MITM between a live bot and real Telegram, reusing `store`/`trace`/`events`/SSE.
Keep architecture compatible; no work until M2–M6 land.

---

### M8 — SaaS beta → v1.0 GA (maps to G7/G8)

- Accounts, teams, quotas, billing (separate control plane; the emulator core
  stays single-tenant and embeddable).
- Published **compatibility score** and an **SDK matrix** (telego, python-telegram-bot,
  grammY, aiogram…) run in CI against the emulator.
- Stable, versioned `/_sim/*` contract; semver for the binary.

---

## 5. Cross-cutting engineering principles

- **Keep the dependency budget tight.** New runtime deps require a one-line
  justification in the commit (per `AGENTS.md`).
- **Every fidelity claim is checked against `telego`'s typed signatures or the
  Bot API spec**, not assumed — the audit proved assumptions are unsafe (§2).
- **Coverage matrix is the source of truth** for "how real is this method"; bumping
  a method's level requires a test.
- **In-memory remains the default**; persistence, media-on-disk, and proxy are all
  opt-in and behind interfaces.
- **The trace is the product.** Correlation stays an explicit, `inferred`-labeled
  heuristic; never present it as ground truth.

## 6. Deferred / explicitly out of scope (for now)

- Full Bot API surface (only registry + stubs until a method earns statefulness).
- Real Telegram connectivity except via the future M7 proxy.
- Multi-tenant auth beyond the single remote-mode token until M8.
- WebSocket transport (SSE stays the server→browser channel).
