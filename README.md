# Local Telegram Client

Local Telegram Client is a fake Telegram Bot API server and browser DevTools UI for bot developers.
Point your bot's Bot API base URL at the simulator, open the browser, and test messages, buttons,
callbacks, webhooks, and trace output without a phone, tunnel, or real Telegram connection.

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

Send `/start` in the browser chat. The showcase bot will respond with inline buttons for edit,
toast, temporary delete, reply keyboard, and trace error scenarios.

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
