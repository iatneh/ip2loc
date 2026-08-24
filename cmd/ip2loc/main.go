// Command ip2loc serves IP geolocation lookups backed by MaxMind mmdb files.
//
// All configuration is via environment variables (IP2LOC_*); the service does
// not read any configuration file. Run with no flags to start with defaults.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/iatneh/ip2loc/internal/config"
	"github.com/iatneh/ip2loc/internal/geoip"
	"github.com/iatneh/ip2loc/internal/logger"
	"github.com/iatneh/ip2loc/internal/server"
	"github.com/iatneh/ip2loc/internal/updater"
)

// version is overridden via -ldflags at build time.
var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	showVersion := flag.Bool("version", false, "print version and exit")
	printDefaults := flag.Bool("print-defaults", false, "print the baked-in defaults and exit")
	flag.Parse()

	if *showVersion {
		_, _ = os.Stdout.WriteString("ip2loc " + version + "\n")
		return nil
	}

	cfg, err := config.Load("")
	if err != nil {
		return err
	}

	if *printDefaults {
		_, _ = os.Stdout.WriteString(formatDefaults(cfg))
		return nil
	}

	log, err := logger.New(cfg.Log)
	if err != nil {
		return err
	}
	logger.SetDefault(log)
	log.Info("ip2loc starting", "version", version)

	// Build dependencies in dependency order.
	reader, err := geoip.NewReader(cfg.GeoIP, log)
	if err != nil {
		return err
	}
	defer func() {
		if err := reader.Close(); err != nil {
			log.Warn("reader close failed", "err", err)
		}
	}()

	upd := updater.New(cfg.Updater, cfg.GeoIP, reader, log)
	if err := upd.Start(); err != nil {
		return err
	}
	defer upd.Stop()

	watcher := geoip.NewWatcher(reader, 30_000_000_000, log) // 30s
	watcher.Start()
	defer watcher.Stop()

	srv := server.New(cfg.HTTP, reader, log)

	// Run until SIGINT/SIGTERM, with a graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	wg.Add(1)
	errCh := make(chan error, 1)
	go func() {
		defer wg.Done()
		errCh <- srv.Run(ctx)
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	}
	wg.Wait()
	log.Info("ip2loc stopped")
	return nil
}

// formatDefaults dumps every config field as `KEY = value`. It is used by
// `-print-defaults` so operators can see the baked-in values without reading
// source.
func formatDefaults(c *config.Config) string {
	var b strings.Builder
	b.WriteString("ip2loc defaults:\n")
	fmt.Fprintf(&b, "  IP2LOC_HTTP_ADDRESS                  = %q\n", c.HTTP.Address)
	fmt.Fprintf(&b, "  IP2LOC_HTTP_PORT                     = %d\n", c.HTTP.Port)
	fmt.Fprintf(&b, "  IP2LOC_HTTP_READ_TIMEOUT             = %s\n", c.HTTP.ReadTimeout)
	fmt.Fprintf(&b, "  IP2LOC_HTTP_WRITE_TIMEOUT            = %s\n", c.HTTP.WriteTimeout)
	fmt.Fprintf(&b, "  IP2LOC_HTTP_IDLE_TIMEOUT             = %s\n", c.HTTP.IdleTimeout)
	fmt.Fprintf(&b, "  IP2LOC_LOG_LEVEL                     = %q\n", c.Log.Level)
	fmt.Fprintf(&b, "  IP2LOC_LOG_FORMAT                    = %q\n", c.Log.Format)
	fmt.Fprintf(&b, "  IP2LOC_LOG_OUTPUT                    = %q\n", c.Log.Output)
	fmt.Fprintf(&b, "  IP2LOC_GEOIP_DB_DIR                  = %q\n", c.GeoIP.DBDir)
	fmt.Fprintf(&b, "  IP2LOC_GEOIP_CITY_FILENAME           = %q\n", c.GeoIP.CityFilename)
	fmt.Fprintf(&b, "  IP2LOC_GEOIP_ASN_FILENAME            = %q\n", c.GeoIP.ASNFilename)
	fmt.Fprintf(&b, "  IP2LOC_GEOIP_DEFAULT_LANG            = %q\n", c.GeoIP.DefaultLang)
	fmt.Fprintf(&b, "  IP2LOC_UPDATER_ENABLED               = %t\n", c.Updater.Enabled)
	fmt.Fprintf(&b, "  IP2LOC_UPDATER_RUN_ON_START          = %t\n", c.Updater.RunOnStart)
	fmt.Fprintf(&b, "  IP2LOC_UPDATER_CRON                  = %q\n", c.Updater.Cron)
	fmt.Fprintf(&b, "  IP2LOC_UPDATER_DOWNLOAD_TIMEOUT      = %s\n", c.Updater.DownloadTimeout)
	fmt.Fprintf(&b, "  IP2LOC_UPDATER_CITY_URL              = %q\n", c.Updater.CityURL)
	fmt.Fprintf(&b, "  IP2LOC_UPDATER_ASN_URL               = %q\n", c.Updater.ASNURL)
	fmt.Fprintf(&b, "  IP2LOC_UPDATER_HEADERS               = %q\n", strings.Join(c.Updater.Headers, ","))
	fmt.Fprintf(&b, "  IP2LOC_DEFAULTS_ALLOW_PRIVATE_IP     = %t\n", c.Defaults.AllowPrivateIP)
	return b.String()
}
