package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mymmrac/telego"

	"github.com/Zulut30/local-telegram-client/internal/showcase"
)

type config struct {
	APIBase     string
	BotToken    string
	Mode        string
	WebhookAddr string
	WebhookURL  string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "showcase-bot: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	client := showcase.NewAPIClient(cfg.APIBase, cfg.BotToken)
	app := showcase.New(client, showcase.NewTraceErrorTrigger(cfg.APIBase, cfg.BotToken))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch cfg.Mode {
	case "polling":
		return runPolling(ctx, logger, client, app)
	case "webhook":
		return runWebhook(ctx, logger, client, app, cfg)
	default:
		return fmt.Errorf("invalid --mode %q: expected polling or webhook", cfg.Mode)
	}
}

func parseConfig(args []string) (config, error) {
	cfg := config{
		APIBase:     envString("SHOWCASE_API_BASE", "http://127.0.0.1:8080"),
		BotToken:    envString("SHOWCASE_BOT_TOKEN", "dev-bot-token"),
		Mode:        envString("SHOWCASE_MODE", "polling"),
		WebhookAddr: envString("SHOWCASE_WEBHOOK_ADDR", "127.0.0.1:8090"),
		WebhookURL:  envString("SHOWCASE_WEBHOOK_URL", "http://127.0.0.1:8090/webhook"),
	}

	fs := flag.NewFlagSet("showcase-bot", flag.ContinueOnError)
	fs.StringVar(&cfg.APIBase, "api-base", cfg.APIBase, "Telegram Bot API base URL exposed by the simulator")
	fs.StringVar(&cfg.BotToken, "bot-token", cfg.BotToken, "fake bot token accepted by the simulator")
	fs.StringVar(&cfg.Mode, "mode", cfg.Mode, "update mode: polling or webhook")
	fs.StringVar(&cfg.WebhookAddr, "webhook-addr", cfg.WebhookAddr, "local HTTP address for webhook mode")
	fs.StringVar(&cfg.WebhookURL, "webhook-url", cfg.WebhookURL, "public URL registered through setWebhook")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}

	cfg.APIBase = strings.TrimRight(cfg.APIBase, "/")
	cfg.Mode = strings.ToLower(strings.TrimSpace(cfg.Mode))
	if cfg.APIBase == "" {
		return config{}, errors.New("--api-base must not be empty")
	}
	if cfg.BotToken == "" {
		return config{}, errors.New("--bot-token must not be empty")
	}
	return cfg, nil
}

func runPolling(ctx context.Context, logger *slog.Logger, client *showcase.APIClient, app *showcase.Bot) error {
	_ = client.DeleteWebhook(&telego.DeleteWebhookParams{})
	logger.Info("showcase bot polling started")
	offset := 0
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		updates, err := client.GetUpdates(&telego.GetUpdatesParams{
			Offset:         offset,
			Limit:          50,
			Timeout:        1,
			AllowedUpdates: []string{telego.MessageUpdates, telego.CallbackQueryUpdates},
		})
		if err != nil {
			logger.Error("get updates", "error", err)
			time.Sleep(time.Second)
			continue
		}
		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			if err := app.Handle(update); err != nil {
				logger.Error("handle update", "update_id", update.UpdateID, "error", err)
			}
		}
	}
}

func runWebhook(ctx context.Context, logger *slog.Logger, client *showcase.APIClient, app *showcase.Bot, cfg config) error {
	if err := client.SetWebhook(&telego.SetWebhookParams{
		URL:                cfg.WebhookURL,
		DropPendingUpdates: true,
		AllowedUpdates:     []string{telego.MessageUpdates, telego.CallbackQueryUpdates},
	}); err != nil {
		return fmt.Errorf("set webhook: %w", err)
	}
	defer func() {
		if err := client.DeleteWebhook(&telego.DeleteWebhookParams{}); err != nil {
			logger.Warn("delete webhook", "error", err)
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhook", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
		var update telego.Update
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			http.Error(w, "decode update", http.StatusBadRequest)
			return
		}
		if err := app.Handle(update); err != nil {
			logger.Error("handle webhook update", "update_id", update.UpdateID, "error", err)
			http.Error(w, "handle update", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	srv := &http.Server{
		Addr:         cfg.WebhookAddr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 20 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		logger.Info("showcase bot webhook started", "addr", cfg.WebhookAddr, "url", cfg.WebhookURL)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown webhook server: %w", err)
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("webhook server: %w", err)
	}
}

func envString(key, fallback string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	return value
}
