package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
)

const (
	ModeLocal  = "local"
	ModeRemote = "remote"
	ModeProxy  = "proxy"

	APIModeCompat = "compat"
	APIModeStrict = "strict"

	defaultLocalAddr  = "127.0.0.1:8080"
	defaultRemoteAddr = "0.0.0.0:8080"
	defaultBotToken   = "dev-bot-token"
	defaultBufferSize = 1000
)

type Config struct {
	Mode       string
	Addr       string
	Token      string
	BotToken   string
	APIMode    string
	BufferSize int
	Persist    string
	LogFormat  string
	LogFile    string
}

func Load(args []string) (Config, error) {
	addrFromEnv := envString("SIM_ADDR", "") != ""
	cfg := Config{
		Mode:       envString("SIM_MODE", ModeLocal),
		Token:      envString("SIM_TOKEN", ""),
		BotToken:   envString("SIM_BOT_TOKEN", defaultBotToken),
		APIMode:    envString("SIM_API_MODE", APIModeCompat),
		BufferSize: envInt("SIM_BUFFER_SIZE", defaultBufferSize),
		Persist:    envString("SIM_PERSIST", ""),
		LogFormat:  envString("SIM_LOG_FORMAT", "json"),
		LogFile:    envString("SIM_LOG_FILE", ""),
	}
	cfg.Addr = envString("SIM_ADDR", "")
	if cfg.Addr == "" {
		cfg.Addr = defaultAddr(cfg.Mode)
	}

	fs := flag.NewFlagSet("sim", flag.ContinueOnError)
	fs.StringVar(&cfg.Mode, "mode", cfg.Mode, "run mode: local, remote, or proxy")
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "HTTP bind address")
	fs.StringVar(&cfg.Token, "token", cfg.Token, "access token for remote UI and /_sim endpoints")
	fs.StringVar(&cfg.BotToken, "bot-token", cfg.BotToken, "fake Telegram bot token accepted in /bot<TOKEN>/ paths")
	fs.StringVar(&cfg.APIMode, "api-mode", cfg.APIMode, "Bot API behavior: compat or strict")
	fs.IntVar(&cfg.BufferSize, "buffer-size", cfg.BufferSize, "trace/event ring buffer size")
	fs.StringVar(&cfg.Persist, "persist", cfg.Persist, "optional SQLite persistence path")
	fs.StringVar(&cfg.LogFormat, "log-format", cfg.LogFormat, "log format: json or text")
	fs.StringVar(&cfg.LogFile, "log-file", cfg.LogFile, "optional log file path")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	addrFromFlag := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "addr" {
			addrFromFlag = true
		}
	})
	if !addrFromEnv && !addrFromFlag {
		cfg.Addr = defaultAddr(cfg.Mode)
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (cfg *Config) validate() error {
	switch cfg.Mode {
	case ModeLocal, ModeRemote, ModeProxy:
	default:
		return fmt.Errorf("invalid mode %q: expected local, remote, or proxy", cfg.Mode)
	}

	if cfg.Mode == ModeRemote && cfg.Token == "" {
		return errors.New("remote mode requires --token or SIM_TOKEN")
	}
	if cfg.Mode == ModeProxy {
		return errors.New("proxy mode is reserved for a future milestone")
	}
	if cfg.BotToken == "" {
		return errors.New("bot token must not be empty")
	}
	switch cfg.APIMode {
	case "", APIModeCompat:
		cfg.APIMode = APIModeCompat
	case APIModeStrict:
	default:
		return fmt.Errorf("invalid api mode %q: expected compat or strict", cfg.APIMode)
	}
	if cfg.BufferSize <= 0 {
		return errors.New("buffer size must be greater than zero")
	}
	switch cfg.LogFormat {
	case "json", "text":
	default:
		return fmt.Errorf("invalid log format %q: expected json or text", cfg.LogFormat)
	}
	if cfg.LogFile != "" {
		return errors.New("log-file support is reserved for a future milestone")
	}
	return nil
}

func (cfg Config) EffectiveAPIMode() string {
	if cfg.APIMode == APIModeStrict {
		return APIModeStrict
	}
	return APIModeCompat
}

func defaultAddr(mode string) string {
	if mode == ModeRemote {
		return defaultRemoteAddr
	}
	return defaultLocalAddr
}

func envString(key, fallback string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
