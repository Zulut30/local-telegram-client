FROM node:22-bookworm AS web
WORKDIR /src
COPY web/package*.json ./web/
RUN cd web && npm ci
COPY web ./web
COPY internal/webui ./internal/webui
RUN cd web && npm run build

FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/webui/dist ./internal/webui/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/sim ./cmd/sim
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/showcase-bot ./cmd/showcase-bot

FROM debian:bookworm-slim
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates curl \
  && rm -rf /var/lib/apt/lists/* \
  && useradd --system --uid 10001 --no-create-home --shell /usr/sbin/nologin sim
COPY --from=build /out/sim /usr/local/bin/sim
COPY --from=build /out/showcase-bot /usr/local/bin/showcase-bot
EXPOSE 8080
USER sim
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD curl -fsS http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["sim"]
