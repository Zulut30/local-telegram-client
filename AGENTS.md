# Codex Notes

## Commands

- `make build` builds the simulator and showcase bot binaries (injects version via `-ldflags`).
- `make test` runs `go vet ./... && go test ./...`.
- `go test -race ./...` is the stronger check CI runs; run it before finishing concurrency work.
- `make run` starts the simulator in local mode.
- `make run-showcase` starts the showcase bot in polling mode.
- `make run-showcase-webhook` starts the showcase bot in webhook mode.
- `make build-frontend` builds the Vite frontend into `internal/webui/dist`.
- `cd web && npm run typecheck` typechecks the frontend (also runs inside `npm run build`).

## Rules

- Build the frontend with `make build-frontend` before the final `make build`; `go:embed` reads from `internal/webui/dist`.
- Add new dependencies only with a clear reason. Keep the runtime binary light.
- The forward-looking plan is `docs/ROADMAP.md` (milestones M1–M8); `PLAN.md` is historical context.
- Verify Bot API return shapes against the `telego` typed signatures or the official spec before
  changing fidelity behavior — do not assume.
- Before moving to the next milestone, `make test` (and `go test -race ./...`) must pass and the
  current milestone should be committed.
