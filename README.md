# Local Telegram Client

Local Telegram Client is a fake Telegram Bot API server and browser DevTools UI for bot developers.
Point your bot's Bot API base URL at the simulator, open the browser, and test messages, buttons,
callbacks, webhooks, and trace output without a phone, tunnel, or real Telegram connection.

The simulator includes a Bot API 10.1 compatibility registry generated from the official Telegram
Bot API documentation. Core chat flows are implemented with stateful behavior, while the rest of the
official methods return deterministic compatibility stubs and still appear in trace output.

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

- Stateful methods: `getUpdates`, webhook methods, `sendMessage`, `sendPhoto`, `sendRichMessage`,
  `sendChatAction`, `sendMessageDraft`, `sendRichMessageDraft`, edits, deletes, callback answers,
  media groups, generic send methods, and custom emoji sticker lookup.
- UI rendering: message entities, custom/premium emoji placeholders, HTML parse mode, rich-message
  tables, media chips, live typing status, and streaming draft previews.
- Compatibility stubs: every official Bot API 10.1 method is recognized case-insensitively; methods
  that are not stateful yet return a stable success shape for local bot testing.

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
sim:          --bot-token dev-bot-token --addr 127.0.0.1:8080
showcase-bot: --bot-token dev-bot-token --api-base http://127.0.0.1:8080
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
