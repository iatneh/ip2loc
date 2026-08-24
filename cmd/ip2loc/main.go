// Command ip2loc serves IP geolocation lookups backed by MaxMind mmdb files.
//
// Usage:
//
//	ip2loc -config /etc/ip2loc/app.yaml
//
// Environment overrides (highest priority):
//
//	IP2LOC_HTTP_PORT, IP2LOC_LOG_LEVEL, IP2LOC_GEOIP_DB_DIR, ...
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
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
	cfgPath := flag.String("config", os.Getenv("IP2LOC_CONFIG"), "path to YAML config file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		_, _ = os.Stdout.WriteString("ip2loc " + version + "\n")
		return nil
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}

	log, err := logger.New(cfg.Log)
	if err != nil {
		return err
	}
	logger.SetDefault(log)
	log.Info("ip2loc starting", "version", version, "config", *cfgPath)

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
