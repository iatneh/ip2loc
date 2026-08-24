// Package config loads, validates and exposes runtime configuration.
//
// Configuration sources, in increasing order of precedence:
//  1. Built-in defaults.
//  2. Environment variables (IP2LOC_<SECTION>_<KEY>).
//
// The service does **not** read any configuration file. All knobs are set
// via env vars at process start, or fall back to the defaults baked into this
// package. The struct shape is intentionally exhaustive: every key the
// program reads has a home here, so we never sprinkle GetStringDefault
// through business code.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config is the root configuration object.
type Config struct {
	HTTP     HTTPConfig     `mapstructure:"http"`
	Log      LogConfig      `mapstructure:"log"`
	GeoIP    GeoIPConfig    `mapstructure:"geoip"`
	Updater  UpdaterConfig  `mapstructure:"updater"`
	Defaults DefaultsConfig `mapstructure:"defaults"`
}

// HTTPConfig describes how the HTTP listener is bound.
type HTTPConfig struct {
	Address      string        `mapstructure:"address"`
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read-timeout"`
	WriteTimeout time.Duration `mapstructure:"write-timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle-timeout"`
}

// LogConfig configures the structured logger.
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"` // "json" or "text"
	Output string `mapstructure:"output"` // stdout, stderr, or file path
}

// GeoIPConfig tells the loader where mmdb files live.
type GeoIPConfig struct {
	DBDir        string `mapstructure:"db-dir"`
	CityFilename string `mapstructure:"city-filename"`
	ASNFilename  string `mapstructure:"asn-filename"`
	DefaultLang  string `mapstructure:"default-lang"`
}

// UpdaterConfig drives the periodic download job.
type UpdaterConfig struct {
	Enabled         bool          `mapstructure:"enabled"`
	// RunOnStart triggers a single download+reload cycle immediately after
	// the process boots (in addition to the cron schedule). Default true so
	// a freshly started container reaches /readyz without waiting up to one
	// cron interval. Set to false to keep the legacy "first download only
	// on the next cron tick" behaviour.
	RunOnStart      bool          `mapstructure:"run-on-start"`
	Cron            string        `mapstructure:"cron"`
	DownloadTimeout time.Duration `mapstructure:"download-timeout"`
	CityURL         string        `mapstructure:"city-url"`
	ASNURL          string        `mapstructure:"asn-url"`
	// Headers to send on every download request; useful for private mirrors.
	Headers []string `mapstructure:"headers"`
}

// DefaultsConfig controls fallback behaviour when an mmdb is missing.
type DefaultsConfig struct {
	// AllowPrivateIP makes the service return IsPrivate=true results even when
	// the database has no record for the address.
	AllowPrivateIP bool `mapstructure:"allow-private-ip"`
}

// defaults returns the program defaults; called before unmarshalling so a missing
// key does not blow up.
func defaults() *Config {
	return &Config{
		HTTP: HTTPConfig{
			Address:      "0.0.0.0",
			Port:         8080,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "text",
			Output: "stdout",
		},
		GeoIP: GeoIPConfig{
			DBDir:        "./data",
			CityFilename: "GeoLite2-City.mmdb",
			ASNFilename:  "GeoLite2-ASN.mmdb",
			DefaultLang:  "en",
		},
		Updater: UpdaterConfig{
			Enabled:         false,
			RunOnStart:      true,
			Cron:            "0 0 */2 * * *",
			DownloadTimeout: 120 * time.Second,
			// Default mmdb download URLs. These are short-URL aliases hosted
			// at git.io and resolve to the P3TERX/GeoLite.mmdb GitHub raw
			// mirror. Override via IP2LOC_UPDATER_CITY_URL /
			// IP2LOC_UPDATER_ASN_URL when needed.
			CityURL:         "https://git.io/GeoLite2-City.mmdb",
			ASNURL:          "https://git.io/GeoLite2-ASN.mmdb",
			Headers:         []string{},
		},
		Defaults: DefaultsConfig{
			AllowPrivateIP: true,
		},
	}
}

// Load resolves configuration from defaults → env overrides.
// The path argument is preserved for backwards compatibility but is ignored —
// the service no longer reads any configuration file.
func Load(_ string) (*Config, error) {
	cfg := defaults()
	if err := applyEnv(cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// applyEnv walks every tagged Config field, looks up the corresponding
// IP2LOC_<UPPER_SNAKE> env var, and overlays the value onto cfg if set.
// Slice fields (e.g. updater.headers) are comma-separated on the env side.
// Numeric and time.Duration fields are parsed via the standard library.
func applyEnv(cfg *Config) error {
	type field struct {
		env   string
		path  []string
		apply func(*Config, string) error
	}
	fields := []field{
		// HTTP
		{"IP2LOC_HTTP_ADDRESS", nil, func(c *Config, v string) error { c.HTTP.Address = v; return nil }},
		{"IP2LOC_HTTP_PORT", nil, func(c *Config, v string) error {
			p, err := parseInt(v)
			if err != nil {
				return fmt.Errorf("IP2LOC_HTTP_PORT: %w", err)
			}
			c.HTTP.Port = p
			return nil
		}},
		{"IP2LOC_HTTP_READ_TIMEOUT", nil, func(c *Config, v string) error {
			d, err := time.ParseDuration(v)
			if err != nil {
				return fmt.Errorf("IP2LOC_HTTP_READ_TIMEOUT: %w", err)
			}
			c.HTTP.ReadTimeout = d
			return nil
		}},
		{"IP2LOC_HTTP_WRITE_TIMEOUT", nil, func(c *Config, v string) error {
			d, err := time.ParseDuration(v)
			if err != nil {
				return fmt.Errorf("IP2LOC_HTTP_WRITE_TIMEOUT: %w", err)
			}
			c.HTTP.WriteTimeout = d
			return nil
		}},
		{"IP2LOC_HTTP_IDLE_TIMEOUT", nil, func(c *Config, v string) error {
			d, err := time.ParseDuration(v)
			if err != nil {
				return fmt.Errorf("IP2LOC_HTTP_IDLE_TIMEOUT: %w", err)
			}
			c.HTTP.IdleTimeout = d
			return nil
		}},
		// Log
		{"IP2LOC_LOG_LEVEL", nil, func(c *Config, v string) error { c.Log.Level = v; return nil }},
		{"IP2LOC_LOG_FORMAT", nil, func(c *Config, v string) error { c.Log.Format = v; return nil }},
		{"IP2LOC_LOG_OUTPUT", nil, func(c *Config, v string) error { c.Log.Output = v; return nil }},
		// GeoIP
		{"IP2LOC_GEOIP_DB_DIR", nil, func(c *Config, v string) error { c.GeoIP.DBDir = v; return nil }},
		{"IP2LOC_GEOIP_CITY_FILENAME", nil, func(c *Config, v string) error { c.GeoIP.CityFilename = v; return nil }},
		{"IP2LOC_GEOIP_ASN_FILENAME", nil, func(c *Config, v string) error { c.GeoIP.ASNFilename = v; return nil }},
		{"IP2LOC_GEOIP_DEFAULT_LANG", nil, func(c *Config, v string) error { c.GeoIP.DefaultLang = v; return nil }},
		// Updater
		{"IP2LOC_UPDATER_ENABLED", nil, func(c *Config, v string) error {
			b, err := parseBool(v)
			if err != nil {
				return fmt.Errorf("IP2LOC_UPDATER_ENABLED: %w", err)
			}
			c.Updater.Enabled = b
			return nil
		}},
		{"IP2LOC_UPDATER_RUN_ON_START", nil, func(c *Config, v string) error {
			b, err := parseBool(v)
			if err != nil {
				return fmt.Errorf("IP2LOC_UPDATER_RUN_ON_START: %w", err)
			}
			c.Updater.RunOnStart = b
			return nil
		}},
		{"IP2LOC_UPDATER_CRON", nil, func(c *Config, v string) error { c.Updater.Cron = v; return nil }},
		{"IP2LOC_UPDATER_DOWNLOAD_TIMEOUT", nil, func(c *Config, v string) error {
			d, err := time.ParseDuration(v)
			if err != nil {
				return fmt.Errorf("IP2LOC_UPDATER_DOWNLOAD_TIMEOUT: %w", err)
			}
			c.Updater.DownloadTimeout = d
			return nil
		}},
		{"IP2LOC_UPDATER_CITY_URL", nil, func(c *Config, v string) error { c.Updater.CityURL = v; return nil }},
		{"IP2LOC_UPDATER_ASN_URL", nil, func(c *Config, v string) error { c.Updater.ASNURL = v; return nil }},
		{"IP2LOC_UPDATER_HEADERS", nil, func(c *Config, v string) error {
			c.Updater.Headers = splitCSV(v)
			return nil
		}},
		// Defaults
		{"IP2LOC_DEFAULTS_ALLOW_PRIVATE_IP", nil, func(c *Config, v string) error {
			b, err := parseBool(v)
			if err != nil {
				return fmt.Errorf("IP2LOC_DEFAULTS_ALLOW_PRIVATE_IP: %w", err)
			}
			c.Defaults.AllowPrivateIP = b
			return nil
		}},
	}
	for _, f := range fields {
		v, ok := os.LookupEnv(f.env)
		if !ok || v == "" {
			continue
		}
		if err := f.apply(cfg, v); err != nil {
			return err
		}
	}
	return nil
}

func parseInt(s string) (int, error) {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0, fmt.Errorf("invalid int %q: %w", s, err)
	}
	return n, nil
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "t", "true", "yes", "on":
		return true, nil
	case "0", "f", "false", "no", "off":
		return false, nil
	}
	return false, fmt.Errorf("invalid bool %q", s)
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// Validate enforces invariants we cannot express in struct tags.
func (c *Config) Validate() error {
	if c.HTTP.Port <= 0 || c.HTTP.Port > 65535 {
		return fmt.Errorf("http.port out of range: %d", c.HTTP.Port)
	}
	if c.GeoIP.DBDir == "" {
		return errors.New("geoip.db-dir must not be empty")
	}
	if c.GeoIP.CityFilename == "" {
		return errors.New("geoip.city-filename must not be empty")
	}
	switch c.Log.Format {
	case "json", "text":
	default:
		return fmt.Errorf("log.format must be json|text, got %q", c.Log.Format)
	}
	switch strings.ToLower(c.Log.Level) {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("log.level invalid: %q", c.Log.Level)
	}
	return nil
}

// CityPath returns the absolute path to the City mmdb (resolved against DBDir).
func (c *Config) CityPath() string {
	return filepath.Join(c.GeoIP.DBDir, c.GeoIP.CityFilename)
}

// ASNPath returns the absolute path to the ASN mmdb.
func (c *Config) ASNPath() string {
	return filepath.Join(c.GeoIP.DBDir, c.GeoIP.ASNFilename)
}

// EnvOrFile picks the env var value if set, otherwise returns the fallback.
// Kept for any callers that still expect it; the service itself no longer
// reads files.
func EnvOrFile(envKey, fallback string) string {
	if v, ok := os.LookupEnv(envKey); ok && v != "" {
		return v
	}
	return fallback
}
