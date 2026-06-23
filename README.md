# Local Telegram Client

Local Telegram Client is a fake Telegram Bot API server and browser DevTools UI for bot developers.
Point your bot's Bot API base URL at the simulator, open the browser, and test messages, buttons,
callbacks, webhooks, and trace output without a phone, tunnel, or real Telegram connection.

The simulator includes a Bot API 10.1 compatibility registry generated from the official Telegram
Bot API documentation. Core chat flows are implemented with stateful behavior, while the rest of the
official methods return deterministic compatibility stubs and still appear in trace output.

## Current Status

Local Telegram Client is currently a developer-preview emulator, not a production-grade Telegram
clone. It is already useful for local bot development, demos, and regression testing of common chat
flows, but many Bot API methods are still compatibility stubs rather than full semantic
implementations.

What works today:

- Local Bot API endpoint compatible with real bot SDKs through `/bot<TOKEN>/<method>`.
- Stateful polling, webhook delivery, user message injection, callback injection, bot replies,
  edits, deletes, reply markup, media-like messages, and trace correlation.
- `GET /_sim/coverage` exposes a machine-readable Bot API coverage matrix.
- `--api-mode=compat|strict` switches between permissive compatibility stubs and explicit errors
  for non-semantic methods.
- Russian IDE-style browser UI with chat list, guide panel, console panel, light/dark theme,
  attachment upload simulation, trace copy, and trace reset.
- Showcase recipe bot for manual testing of messages, photos, buttons, callback answers, rich
  messages, typing states, trace errors, polling, and webhook mode.
- CI, release workflow, Dockerfile, systemd example, Caddy example, and local release-style builds.

Main limitations:

- Bot API coverage is broad but shallow outside the core stateful methods.
- Uploaded files are represented as URLs or fake file IDs; durable byte storage and
  `/_sim/file/{id}` are not implemented yet.
- Persistence flags are reserved; runtime state is in memory by default.
- Payments, Telegram Stars, Mini Apps, inline mode, business flows, forums, games, passport,
  gifts, and many admin methods do not yet model Telegram behavior deeply.
- Remote mode has a basic token guard, but SaaS-grade auth, tenant isolation, quotas, billing, and
  audit trails are future work.

## Quickstart

Build the web UI and Go binaries:

```sh
make build-frontend
make build
```

Start the simulator:

```sh
./bin/sim
```

Open the UI:

```text
http://127.0.0.1:8080/
```

In a second terminal, start the showcase bot:

```sh
./bin/showcase-bot --mode polling
```

Send `/start` in the browser chat. The Russian showcase bot opens a recipe catalog with photo
cards, ingredients, steps, source links, reply keyboard controls, and an `Инструменты` section for
edit, toast, temporary delete, reply keyboard, rich message, and trace error scenarios.

The browser UI works like a small Russian-localized IDE: use the top bar to hide/show `Чаты`,
`Гайд`, and `Консоль`, switch light/dark theme, and use the attachment button in the composer to
send a photo update with an optional caption. The Console header can copy the current trace history
as formatted JSON or clear only the trace history while keeping the chat state.

## Webhook Demo

Stop the polling showcase bot and start webhook mode:

```sh
./bin/showcase-bot \
  --mode webhook \
  --webhook-addr 127.0.0.1:8090 \
  --webhook-url http://127.0.0.1:8090/webhook
```

Send `/start` again in the browser. The simulator will deliver the update to the showcase bot through
`setWebhook`, and the trace panel will show the inbound update plus the bot's outgoing calls.

## Recipe Showcase Bot

The bundled showcase bot is a small food recipe bot backed by static demo data from
[TheMealDB](https://www.themealdb.com/). It exercises:

- `sendMessage` for menus, ingredients, steps, echo, and photo acknowledgements.
- `sendPhoto` for recipe cards with remote image URLs.
- `sendChatAction` for realistic bot activity states such as `upload_photo`.
- `sendRichMessage`, custom emoji entities, HTML formatting, and rich tables.
- `answerCallbackQuery`, `editMessageText`, `deleteMessage`, reply keyboards, polling, webhook, and trace errors.
- User photo injection through the UI attachment button.

## Bot API Coverage

The method registry follows the official [Telegram Bot API](https://core.telegram.org/bots/api)
documentation for Bot API 10.1, released on June 11, 2026.

Coverage levels:

| Level | Meaning | Current examples |
|---|---|---|
| Stateful | The method changes simulator state, is visible in UI, and is covered by integration tests. | `getUpdates`, webhook methods, `sendMessage`, `sendPhoto`, `editMessageText`, `editMessageReplyMarkup`, `deleteMessage`, `answerCallbackQuery`, `sendChatAction`, `sendMessageDraft`, `sendRichMessage`, `getCustomEmojiStickers` |
| UI-rendered | The UI can display the result, but Telegram semantics may still be simplified. | entities, custom/premium emoji placeholders, HTML parse mode, rich-message tables, media chips, live typing status, streaming draft previews |
| Compatibility stub | The official method name is accepted and returns deterministic success data so local bots keep running. | lower-priority admin, reaction, profile, sticker-management, and settings methods |
| Not yet semantic | The method is recognized but does not yet emulate Telegram-side validation, state transitions, events, or edge cases. | payments, invoices, Stars, Mini Apps, inline mode, file downloads, business accounts, gifts, forum topics, games |

The goal is to move high-value methods from `compatibility stub` to `stateful` in small,
well-tested slices.

Inspect the current matrix:

```sh
curl http://127.0.0.1:8080/_sim/coverage
```

## Roadmap / Goals To SaaS

This README is the canonical public roadmap. The old `PLAN.md` is kept as historical
implementation context, but the goals below are the source of truth for future work.

### G0: Publish Roadmap Goals

Definition of Done:

- README clearly states current product status, coverage levels, and limitations.
- Roadmap goals are committed and pushed to `main`.
- No GitHub Issues, milestones, or project board are created for this docs pass.

Still incomplete until:

- Later implementation goals are split into issues or milestones if project management moves out of
  README.

### G1: v0.1 Public OSS Release Hardening

Progress on `main`:

- Done: `GET /_sim/coverage`.
- Done: `--api-mode=compat|strict`.

Definition of Done:

- Release tag `v0.1.0` builds Linux and macOS archives with `sim`, `showcase-bot`, checksums,
  README, and license.
- README quickstart, webhook demo, Docker, systemd, and Caddy docs are verified from a clean clone.
- `GET /_sim/coverage` exposes method coverage as `stateful`, `ui_rendered`,
  `compatibility_stub`, or `not_yet_semantic`.
- `--api-mode=compat|strict` is documented and implemented: compat keeps stable stubs, strict makes
  unsupported semantic behavior explicit.
- CI runs Go vet/test, frontend build, Go build, and release-style binary build.

Still incomplete until:

- Browser end-to-end smoke tests run against the built binary.
- README has a release badge, coverage summary, and a clear versioning policy.

### G2: Bot API Fidelity

Definition of Done:

- Real media/file handling exists: multipart uploads are stored, bot file IDs resolve through
  `getFile`, and bytes are served through `GET /_sim/file/{id}`.
- Payments are stateful enough for local testing: `sendInvoice`, shipping queries, pre-checkout
  queries, successful payment updates, refunds, and failure injection.
- Telegram Stars and paid media have realistic local fixtures and trace events.
- Mini Apps and Web App flows support `web_app_data`, `answerWebAppQuery`, init-data fixtures, and
  button-driven UI testing.
- Inline mode supports injected inline queries, `answerInlineQuery`, selected results, and visible
  trace output.

Still incomplete until:

- SDK compatibility is tested with at least Go, Node.js, Python, and PHP bot libraries.
- High-value Bot API methods have acceptance tests that compare response shape against official
  examples where practical.

### G3: Testing IDE

Definition of Done:

- Users can record a manual session as a scenario.
- `POST /_sim/scenarios` saves scenarios with updates, expected bot calls, and optional UI-visible
  assertions.
- `POST /_sim/scenarios/{id}/run` replays scenarios deterministically.
- The browser UI has a scenario runner, API explorer, fixtures panel, and error-injection controls.
- Assertions can check sent messages, reply markup, callbacks, traces, webhook delivery, and
  expected failures.

Still incomplete until:

- Scenarios can run headlessly in CI with exported JUnit or JSON reports.
- Scenario files are stable enough to commit into bot projects.

### G4: Persistence And Session Portability

Definition of Done:

- `--persist` enables SQLite-backed sessions while in-memory mode remains the default.
- `--media-dir` stores uploaded media bytes separately from the SQLite database.
- `GET /_sim/export` and `POST /_sim/import` move chats, messages, updates, traces, media metadata,
  webhooks, and scenarios between machines.
- Reset supports full reset, trace-only reset, and scenario/session reset.

Still incomplete until:

- Migration tests protect old session files.
- Large media handling has size limits, cleanup policy, and backup guidance.

### G5: Observability And Debugging

Definition of Done:

- Trace console supports search, filters, status grouping, method grouping, and JSONL export.
- Structured logs include request IDs, trace IDs, bot token hash, method, latency, and outcome.
- `/metrics` exposes safe local metrics for request counts, errors, trace counts, webhook delivery,
  and queue depth.
- Webhook failures show last error, retry behavior, delivery latency, and pending count in the UI.

Still incomplete until:

- Long traces are easy to inspect without overwhelming the browser.
- Sensitive values are redacted consistently in UI, logs, exports, and copied trace payloads.

### G6: Self-Hosted Production

Definition of Done:

- `--auth-mode` supports local-only, shared-token, and reverse-proxy header modes.
- Remote mode has documented CORS behavior, request limits, body-size limits, and safe defaults.
- Docker image can run with mounted persistence and media directories.
- systemd and Caddy examples include health checks, restart policy, TLS reverse proxy, and backup
  notes.
- Admin docs explain update strategy, data retention, threat model, and disaster recovery.

Still incomplete until:

- A self-hosted smoke test runs the released binary behind the documented reverse proxy.
- The project has a security policy and vulnerability reporting path.

### G7: SaaS Beta

Definition of Done:

- Hosted workspaces support accounts, teams, projects, per-project bot tokens, and isolated
  sessions.
- Tenant isolation protects chats, traces, media, scenarios, exports, and logs.
- Quotas cover requests, stored media, trace retention, scenario runs, and webhook deliveries.
- Billing supports a free developer tier and paid team tier.
- SaaS UI includes project switcher, invite flow, audit log, usage page, and billing page.

Still incomplete until:

- Production monitoring, backups, incident process, abuse prevention, and data deletion flows are
  operational.
- Legal pages, privacy policy, and terms match the hosted product behavior.

### G8: v1.0 General Availability

Definition of Done:

- Public compatibility score shows which Bot API methods are stateful, stubbed, or unsupported.
- Stable public simulator APIs have versioning and migration policy.
- SDK matrix is green for the most common Telegram bot libraries.
- Scenario runner is reliable enough for bot CI pipelines.
- Documentation covers local development, team usage, self-hosting, and SaaS usage.

Still incomplete until:

- Breaking-change policy, deprecation windows, and upgrade guides are in place.
- Real users have validated the product with non-trivial bots before the `v1.0.0` tag.

## Simulator Interfaces

Implemented:

```text
GET  /_sim/coverage                 # Bot API coverage matrix
```

Planned:

```text
GET  /_sim/file/{id}                # stored media bytes
POST /_sim/scenarios                # save a scenario
POST /_sim/scenarios/{id}/run       # replay a scenario
GET  /_sim/export                   # export session data
POST /_sim/import                   # import session data
```

Planned flags:

```text
--persist=/path/to/session.sqlite
--media-dir=/path/to/media
--auth-mode=local|token|proxy-header
```

Included recipe sources:

- [Spicy Arrabiata Penne](https://www.themealdb.com/meal/52771-spicy-arrabiata-penne-recipe)
- [Chicken Handi](https://www.themealdb.com/meal/52795-chicken-handi-recipe)
- [Beef and Mustard Pie](https://www.themealdb.com/meal/52874-beef-and-mustard-pie-recipe)

## Make Targets

```sh
make run                    # run the simulator
make run-showcase           # run showcase bot in polling mode
make run-showcase-webhook   # run showcase bot in webhook mode
make demo                   # print the two-terminal demo flow
make test                   # go vet ./... && go test ./...
make build-frontend         # build React UI into internal/webui/dist
make build                  # build bin/sim and bin/showcase-bot
```

## Bot Configuration

The simulator accepts a fake bot token through `--bot-token` or `SIM_BOT_TOKEN`.
The showcase bot must use the same value.

Defaults:

```text
sim:          --bot-token dev-bot-token --addr 127.0.0.1:8080 --api-mode compat
showcase-bot: --bot-token dev-bot-token --api-base http://127.0.0.1:8080
```

API modes:

```text
compat # default; recognized Bot API methods can fall back to deterministic stubs
strict # non-semantic methods return HTTP 501 with an explicit simulator error
```

To point your own bot at the simulator, set its Bot API base URL to:

```text
http://127.0.0.1:8080
```

and use the configured fake token in `/bot<TOKEN>/<method>` calls.

## Remote Mode

For a self-hosted dev server:

```sh
./bin/sim --mode remote --token "$SIM_TOKEN" --addr 0.0.0.0:8080
```

UI and `/_sim/*` endpoints require the token in `Authorization: Bearer ...`, `X-Sim-Token`, or
`?token=...`. Bot API paths stay authenticated by the bot token in the path.

Prefer running remote mode behind Tailscale, Cloudflare Tunnel, Caddy, or another HTTPS reverse proxy.

## Control Plane

Useful simulator endpoints:

```text
POST /_sim/inject   # inject message or callback_query
GET  /_sim/state    # chats and messages
GET  /_sim/traces   # trace ring snapshot
GET  /_sim/coverage # Bot API coverage matrix and current api mode
GET  /_sim/events   # SSE stream
POST /_sim/reset    # clear chats, messages, pending updates, traces, and webhook state
POST /_sim/traces/reset # clear traces only, keep chat state
```

## Release Build

GitHub Actions builds and tests every push. Tags matching `v*` create release archives with:

```text
sim
showcase-bot
README.md
LICENSE
```

Local release-style build:

```sh
make build-frontend
CGO_ENABLED=0 make build
```
