package config

import "testing"

func TestLoadDefaultsToLocalMode(t *testing.T) {
	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Mode != ModeLocal {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, ModeLocal)
	}
	if cfg.Addr != defaultLocalAddr {
		t.Fatalf("Addr = %q, want %q", cfg.Addr, defaultLocalAddr)
	}
	if cfg.BufferSize != defaultBufferSize {
		t.Fatalf("BufferSize = %d, want %d", cfg.BufferSize, defaultBufferSize)
	}
	if cfg.APIMode != APIModeCompat {
		t.Fatalf("APIMode = %q, want %q", cfg.APIMode, APIModeCompat)
	}
}

func TestLoadRemoteModeDefaultsAddressAndRequiresToken(t *testing.T) {
	if _, err := Load([]string{"--mode", "remote"}); err == nil {
		t.Fatal("Load remote mode without token returned nil error")
	}

	cfg, err := Load([]string{"--mode", "remote", "--token", "secret"})
	if err != nil {
		t.Fatalf("Load remote mode with token returned error: %v", err)
	}
	if cfg.Addr != defaultRemoteAddr {
		t.Fatalf("Addr = %q, want %q", cfg.Addr, defaultRemoteAddr)
	}
}

func TestLoadFlagOverridesEnv(t *testing.T) {
	t.Setenv("SIM_MODE", "remote")
	t.Setenv("SIM_TOKEN", "env-secret")
	t.Setenv("SIM_ADDR", "0.0.0.0:9000")

	cfg, err := Load([]string{"--mode", "local", "--addr", "127.0.0.1:9999"})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Mode != ModeLocal {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, ModeLocal)
	}
	if cfg.Addr != "127.0.0.1:9999" {
		t.Fatalf("Addr = %q, want explicit flag value", cfg.Addr)
	}
}

func TestLoadAPIMode(t *testing.T) {
	t.Setenv("SIM_API_MODE", APIModeStrict)

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.APIMode != APIModeStrict {
		t.Fatalf("APIMode from env = %q, want %q", cfg.APIMode, APIModeStrict)
	}

	cfg, err = Load([]string{"--api-mode", APIModeCompat})
	if err != nil {
		t.Fatalf("Load with api mode flag returned error: %v", err)
	}
	if cfg.APIMode != APIModeCompat {
		t.Fatalf("APIMode from flag = %q, want %q", cfg.APIMode, APIModeCompat)
	}

	if _, err := Load([]string{"--api-mode", "loose"}); err == nil {
		t.Fatal("Load with invalid api mode returned nil error")
	}
}

func TestEffectiveAPIModeDefaultsToCompat(t *testing.T) {
	if got := (Config{}).EffectiveAPIMode(); got != APIModeCompat {
		t.Fatalf("EffectiveAPIMode = %q, want %q", got, APIModeCompat)
	}
}
