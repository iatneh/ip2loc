package config

import (
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTP.Port != 8080 {
		t.Errorf("default port = %d, want 8080", cfg.HTTP.Port)
	}
	if cfg.GeoIP.DBDir == "" {
		t.Error("default db dir empty")
	}
	if cfg.Updater.CityURL == "" {
		t.Error("default city url empty")
	}
	if cfg.Updater.ASNURL == "" {
		t.Error("default asn url empty")
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("IP2LOC_HTTP_PORT", "9090")
	t.Setenv("IP2LOC_HTTP_ADDRESS", "127.0.0.1")
	t.Setenv("IP2LOC_LOG_LEVEL", "debug")
	t.Setenv("IP2LOC_LOG_FORMAT", "json")
	t.Setenv("IP2LOC_GEOIP_DB_DIR", "/var/lib/ip2loc")
	t.Setenv("IP2LOC_GEOIP_DEFAULT_LANG", "zh-CN")
	t.Setenv("IP2LOC_UPDATER_ENABLED", "true")
	t.Setenv("IP2LOC_UPDATER_CITY_URL", "https://mirror/city.mmdb")
	t.Setenv("IP2LOC_UPDATER_ASN_URL", "https://mirror/asn.mmdb")
	t.Setenv("IP2LOC_UPDATER_HEADERS", "X-A: 1,X-B: 2")
	t.Setenv("IP2LOC_DEFAULTS_ALLOW_PRIVATE_IP", "false")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTP.Port != 9090 {
		t.Errorf("port = %d", cfg.HTTP.Port)
	}
	if cfg.HTTP.Address != "127.0.0.1" {
		t.Errorf("address = %s", cfg.HTTP.Address)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("level = %s", cfg.Log.Level)
	}
	if cfg.Log.Format != "json" {
		t.Errorf("format = %s", cfg.Log.Format)
	}
	if cfg.GeoIP.DBDir != "/var/lib/ip2loc" {
		t.Errorf("dbdir = %s", cfg.GeoIP.DBDir)
	}
	if cfg.GeoIP.DefaultLang != "zh-CN" {
		t.Errorf("lang = %s", cfg.GeoIP.DefaultLang)
	}
	if !cfg.Updater.Enabled {
		t.Error("updater should be enabled")
	}
	if cfg.Updater.CityURL != "https://mirror/city.mmdb" {
		t.Errorf("city url = %s", cfg.Updater.CityURL)
	}
	if cfg.Updater.ASNURL != "https://mirror/asn.mmdb" {
		t.Errorf("asn url = %s", cfg.Updater.ASNURL)
	}
	if len(cfg.Updater.Headers) != 2 {
		t.Fatalf("headers len = %d", len(cfg.Updater.Headers))
	}
	if cfg.Updater.Headers[0] != "X-A: 1" || cfg.Updater.Headers[1] != "X-B: 2" {
		t.Errorf("headers = %v", cfg.Updater.Headers)
	}
	if cfg.Defaults.AllowPrivateIP {
		t.Error("allow-private-ip should be false")
	}
}

func TestLoad_EmptyEnvKeepsDefault(t *testing.T) {
	t.Setenv("IP2LOC_HTTP_PORT", "")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTP.Port != 8080 {
		t.Errorf("empty env should not override default, got %d", cfg.HTTP.Port)
	}
}

func TestLoad_BadBool(t *testing.T) {
	t.Setenv("IP2LOC_UPDATER_ENABLED", "definitely-not-a-bool")
	if _, err := Load(""); err == nil {
		t.Fatal("expected error for invalid bool")
	}
}

func TestLoad_BadDuration(t *testing.T) {
	t.Setenv("IP2LOC_HTTP_READ_TIMEOUT", "ten seconds")
	if _, err := Load(""); err == nil {
		t.Fatal("expected error for invalid duration")
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
