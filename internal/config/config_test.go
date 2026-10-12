package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRepoConfigIsBaseline(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "config.yaml"))
	if err != nil {
		t.Fatalf("config.yaml must load cleanly: %v", err)
	}
	if cfg != Default() {
		t.Fatalf("config.yaml must equal the step-0 defaults\n got: %+v\nwant: %+v", cfg, Default())
	}
}

func TestRejectsSwitchFromLaterStep(t *testing.T) {
	path := writeTemp(t, "cache: redis\n")
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "until step 4") {
		t.Fatalf("want a 'until step 4' error, got %v", err)
	}
}

func TestRejectsUnknownKey(t *testing.T) {
	path := writeTemp(t, "db:\n  pool_sise: 20\n")
	if _, err := Load(path); err == nil {
		t.Fatal("a misspelled key must be an error, not a silent default")
	}
}

func TestOverridesPoolSizeAndTTL(t *testing.T) {
	path := writeTemp(t, "db:\n  pool_size: 20\nauth:\n  token_ttl: 30m\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DB.PoolSize != 20 || cfg.Auth.TokenTTL != 30*time.Minute {
		t.Fatalf("overrides not applied: %+v", cfg)
	}
}

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
