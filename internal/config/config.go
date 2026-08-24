// Package config loads, validates and exposes runtime configuration.
//
// Configuration sources, in increasing order of precedence:
//  1. Built-in defaults.
//  2. YAML file (path from -config / IP2LOC_CONFIG, default ./configs/app.yaml).
//  3. Environment variables (IP2LOC_<SECTION>_<KEY>).
//
// The struct shape is intentionally exhaustive: every key the program reads has a
// home here, so we never sprinkle GetStringDefault through business code.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
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
			Cron:            "0 0 */2 * * *",
			DownloadTimeout: 120 * time.Second,
			Headers:         []string{},
		},
		Defaults: DefaultsConfig{
			AllowPrivateIP: true,
		},
	}
}

// Load resolves configuration from defaults → file → env → flags.
// configPath may be empty to fall back to the search list.
func Load(configPath string) (*Config, error) {
	v := viper.NewWithOptions(viper.KeyDelimiter("."))

	// 1) Defaults.
	def := defaults()
	if err := bindDefaults(v, def); err != nil {
		return nil, fmt.Errorf("bind defaults: %w", err)
	}

	// 2) File.
	v.SetConfigType("yaml")
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("app")
		v.AddConfigPath("./configs")
		v.AddConfigPath("./conf")
		v.AddConfigPath(".")
		v.AddConfigPath("/etc/ip2loc")
	}
	if err := v.ReadInConfig(); err != nil {
		var nfErr viper.ConfigFileNotFoundError
		if !errors.As(err, &nfErr) && configPath != "" {
			return nil, fmt.Errorf("read config %q: %w", configPath, err)
		}
	}

	// 3) Env: IP2LOC_HTTP_PORT, IP2LOC_GEOIP_DB_DIR, ...
	v.SetEnvPrefix("IP2LOC")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	cfg := defaults()
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
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

// bindDefaults registers each field as a default so viper's precedence works.
func bindDefaults(v *viper.Viper, c *Config) error {
	set := func(path string, val interface{}) {
		v.SetDefault(path, val)
	}
	set("http.address", c.HTTP.Address)
	set("http.port", c.HTTP.Port)
	set("http.read-timeout", c.HTTP.ReadTimeout)
	set("http.write-timeout", c.HTTP.WriteTimeout)
	set("http.idle-timeout", c.HTTP.IdleTimeout)
	set("log.level", c.Log.Level)
	set("log.format", c.Log.Format)
	set("log.output", c.Log.Output)
	set("geoip.db-dir", c.GeoIP.DBDir)
	set("geoip.city-filename", c.GeoIP.CityFilename)
	set("geoip.asn-filename", c.GeoIP.ASNFilename)
	set("geoip.default-lang", c.GeoIP.DefaultLang)
	set("updater.enabled", c.Updater.Enabled)
	set("updater.cron", c.Updater.Cron)
	set("updater.download-timeout", c.Updater.DownloadTimeout)
	set("updater.city-url", c.Updater.CityURL)
	set("updater.asn-url", c.Updater.ASNURL)
	set("updater.headers", c.Updater.Headers)
	set("defaults.allow-private-ip", c.Defaults.AllowPrivateIP)
	return nil
}

// EnvOrFile picks the env var value if set, otherwise returns the fallback.
func EnvOrFile(envKey, fallback string) string {
	if v, ok := os.LookupEnv(envKey); ok && v != "" {
		return v
	}
	return fallback
}
