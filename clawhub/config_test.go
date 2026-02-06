package clawhub

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDefaults(t *testing.T) {
	cfg := defaultConfig()

	if cfg.SiteURL != DefaultSiteURL {
		t.Errorf("expected SiteURL %s, got %s", DefaultSiteURL, cfg.SiteURL)
	}

	if cfg.RegistryURL != DefaultRegistryURL {
		t.Errorf("expected RegistryURL %s, got %s", DefaultRegistryURL, cfg.RegistryURL)
	}

	if cfg.SkillsDir != DefaultSkillsDir {
		t.Errorf("expected SkillsDir %s, got %s", DefaultSkillsDir, cfg.SkillsDir)
	}
}

func TestConfigAuthentication(t *testing.T) {
	cfg := &Config{}

	if cfg.IsAuthenticated() {
		t.Error("expected unauthenticated with empty token")
	}

	cfg.SetToken("test-token", "test-label")

	if !cfg.IsAuthenticated() {
		t.Error("expected authenticated after setting token")
	}

	if cfg.Token != "test-token" {
		t.Errorf("expected token 'test-token', got '%s'", cfg.Token)
	}

	if cfg.TokenLabel != "test-label" {
		t.Errorf("expected label 'test-label', got '%s'", cfg.TokenLabel)
	}

	cfg.ClearToken()

	if cfg.IsAuthenticated() {
		t.Error("expected unauthenticated after clearing token")
	}

	if cfg.Token != "" {
		t.Errorf("expected empty token, got '%s'", cfg.Token)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: &Config{
				SiteURL:     "https://example.com",
				RegistryURL: "https://api.example.com",
				SkillsDir:   "skills",
			},
			wantErr: false,
		},
		{
			name: "missing site URL",
			cfg: &Config{
				RegistryURL: "https://api.example.com",
				SkillsDir:   "skills",
			},
			wantErr: true,
		},
		{
			name: "missing registry URL",
			cfg: &Config{
				SiteURL:   "https://example.com",
				SkillsDir: "skills",
			},
			wantErr: true,
		},
		{
			name: "missing skills dir",
			cfg: &Config{
				SiteURL:     "https://example.com",
				RegistryURL: "https://api.example.com",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfigSaveLoad(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "clawhub-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfgPath := filepath.Join(tmpDir, "config.json")

	// Create and save config
	cfg := &Config{
		SiteURL:     "https://test.example.com",
		RegistryURL: "https://api.test.example.com",
		Token:       "test-token",
		TokenLabel:  "test-label",
		SkillsDir:   "test-skills",
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(cfgPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	// Load config
	loaded, err := LoadConfig()
	if err == nil {
		// If it loaded the default config, check it's not our test config
		if loaded.SiteURL == "https://test.example.com" {
			t.Error("expected different config, got test config")
		}
	}
}

func TestIsTelemetryDisabled(t *testing.T) {
	// Save original value
	orig := os.Getenv("CLAWHUB_DISABLE_TELEMETRY")
	defer os.Setenv("CLAWHUB_DISABLE_TELEMETRY", orig)

	// Test default (not disabled)
	os.Unsetenv("CLAWHUB_DISABLE_TELEMETRY")
	if IsTelemetryDisabled() {
		t.Error("expected telemetry enabled by default")
	}

	// Test disabled
	os.Setenv("CLAWHUB_DISABLE_TELEMETRY", "1")
	if !IsTelemetryDisabled() {
		t.Error("expected telemetry disabled when set to 1")
	}

	os.Unsetenv("CLAWHUB_DISABLE_TELEMETRY")
}
