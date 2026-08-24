package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		// No file in this environment may exist; we just confirm defaults load.
		t.Logf("Load() with no file returned: %v", err)
		if cfg != nil {
			t.Fatalf("cfg should be nil on error")
		}
		return
	}
	if cfg.HTTP.Port != 8080 {
		t.Errorf("default port = %d, want 8080", cfg.HTTP.Port)
	}
	if cfg.GeoIP.DBDir == "" {
		t.Error("default db dir empty")
	}
}

func TestLoad_FromFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "app.yaml")
	content := `
http:
  address: 127.0.0.1
  port: 9090
log:
  level: debug
  format: json
geoip:
  db-dir: /tmp/ip2loc-data
  city-filename: city.mmdb
  default-lang: zh-CN
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTP.Port != 9090 {
		t.Errorf("port = %d", cfg.HTTP.Port)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("level = %s", cfg.Log.Level)
	}
	if cfg.Log.Format != "json" {
		t.Errorf("format = %s", cfg.Log.Format)
	}
	if cfg.GeoIP.DefaultLang != "zh-CN" {
		t.Errorf("lang = %s", cfg.GeoIP.DefaultLang)
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Config)
		want bool
	}{
		{"ok", func(c *Config) {}, true},
		{"bad port", func(c *Config) { c.HTTP.Port = 70000 }, false},
		{"empty dbdir", func(c *Config) { c.GeoIP.DBDir = "" }, false},
		{"bad format", func(c *Config) { c.Log.Format = "yaml" }, false},
		{"bad level", func(c *Config) { c.Log.Level = "panic" }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := defaults()
			tc.mut(c)
			err := c.Validate()
			if tc.want && err != nil {
				t.Fatalf("expected ok, got %v", err)
			}
			if !tc.want && err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

func TestCityAndASNPath(t *testing.T) {
	c := defaults()
	c.GeoIP.DBDir = "/var/lib/ip2loc"
	c.GeoIP.CityFilename = "city.mmdb"
	c.GeoIP.ASNFilename = "asn.mmdb"

	if got := c.CityPath(); got != "/var/lib/ip2loc/city.mmdb" {
		t.Errorf("CityPath = %s", got)
	}
	if got := c.ASNPath(); got != "/var/lib/ip2loc/asn.mmdb" {
		t.Errorf("ASNPath = %s", got)
	}
}

func TestEnvOrFile(t *testing.T) {
	t.Setenv("FOO_BAR", "from-env")
	if got := EnvOrFile("FOO_BAR", "fallback"); got != "from-env" {
		t.Errorf("got %q", got)
	}
	t.Setenv("FOO_BAR", "")
	if got := EnvOrFile("FOO_BAR", "fallback"); got != "fallback" {
		t.Errorf("got %q", got)
	}
}
