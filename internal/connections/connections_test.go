package connections

import (
	"context"
	"infuse/internal/config"
	"infuse/internal/core"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func testHome(t *testing.T) {
	t.Helper()
	t.Setenv("INFUSE_HOME", t.TempDir())
	prev := config.SecretBackend
	config.SecretBackend = config.MemSecrets{}
	t.Cleanup(func() { config.SecretBackend = prev })
}

func TestConnectProviderStoresKeyInVaultNotSettings(t *testing.T) {
	testHome(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("authorization") != "Bearer sk-test-1234" {
			w.WriteHeader(401)
			_, _ = io.WriteString(w, `{"error":{"message":"bad key"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"deepseek-chat"},{"id":"deepseek-reasoner"}]}`)
	}))
	defer srv.Close()
	if _, err := ConnectProvider(context.Background(), "deepseek", "wrong", srv.URL); err == nil {
		t.Fatal("bad key accepted")
	}
	c, err := ConnectProvider(context.Background(), "deepseek", "sk-test-1234", srv.URL)
	if err != nil || !c.Connected || len(c.Models) != 2 || c.KeyHint != "…1234" {
		t.Fatalf("%+v %v", c, err)
	}
	raw, _ := os.ReadFile(config.SettingsFile())
	if strings.Contains(string(raw), "sk-test-1234") {
		t.Fatal("key written to settings.json")
	}
	if k, _ := config.SecretGet(config.SecretName("deepseek")); k != "sk-test-1234" {
		t.Fatal("key not in the vault")
	}
	s := config.LoadSettings()
	if s.Provider != "deepseek" || s.Model != "deepseek-chat" {
		t.Fatalf("default not set: %s %s", s.Provider, s.Model)
	}
	kind, cfg := ConnectionConfig(s, "deepseek")
	if kind != "openai" || cfg.APIKey != "sk-test-1234" || cfg.BaseURL != srv.URL {
		t.Fatalf("%s %+v", kind, cfg)
	}
	if ms := AvailableModels(s); len(ms) != 2 || ms[0].Provider != "deepseek" {
		t.Fatalf("%+v", ms)
	}
	_ = DisconnectProvider("deepseek")
	if k, _ := config.SecretGet(config.SecretName("deepseek")); k != "" {
		t.Fatal("key left in the vault")
	}
}

func TestLegacyKeyMigrates(t *testing.T) {
	testHome(t)
	_ = core.WriteJSON(config.SettingsFile(), map[string]any{"provider": "openai", "apiKey": "sk-legacy-9999", "baseUrl": "https://api.deepseek.com", "model": "deepseek-chat"})
	s := config.LoadSettings()
	if s.APIKey != "" || s.Provider != "deepseek" || !s.Connections["deepseek"].Connected {
		t.Fatalf("%+v", s)
	}
	raw, _ := os.ReadFile(config.SettingsFile())
	if strings.Contains(string(raw), "sk-legacy-9999") {
		t.Fatal("legacy key still in settings.json")
	}
	if k, _ := config.SecretGet(config.SecretName("deepseek")); k != "sk-legacy-9999" {
		t.Fatal("legacy key lost")
	}
}
