# Codex Notes

## Commands

- `make build` builds the simulator and showcase bot binaries.
- `make test` runs `go vet ./... && go test ./...`.
- `make run` starts the simulator in local mode.
- `make run-showcase` starts the showcase bot in polling mode.
- `make run-showcase-webhook` starts the showcase bot in webhook mode.
- `make build-frontend` builds the Vite frontend into `internal/webui/dist`.

## Rules

- Build the frontend with `make build-frontend` before the final `make build`; `go:embed` reads from `internal/webui/dist`.
- Add new dependencies only with a clear reason. Keep the runtime binary light.
- Work milestone-by-milestone from `PLAN.md`.
- Before moving to the next milestone, `make test` must pass and the current milestone should be committed.
