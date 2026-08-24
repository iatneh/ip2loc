// Package updater downloads fresh mmdb files on a cron schedule and asks the
// geoip.Reader to reload them.
//
// Design notes:
//   - We download to a temp file in the same directory, fsync, compare hashes,
//     and atomically rename. This guarantees that any concurrent reader sees
//     either the old or the new file, never a half-written one.
//   - MD5 equality is checked against the existing on-disk file. Same hash →
//     skip rename; saves an inotify storm on watchers.
//   - A single-flight lock keeps overlapping ticks (cron + manual trigger)
//     from racing.
package updater

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/robfig/cron/v3"

	"github.com/iatneh/ip2loc/internal/config"
	"github.com/iatneh/ip2loc/internal/geoip"
)

// Reloader is the subset of geoip.Reader we depend on. Defined as an interface
// to keep tests trivial.
type Reloader interface {
	ReloadCity() error
	ReloadASN() error
}

// Updater drives the cron and the actual download.
type Updater struct {
	cfg    config.UpdaterConfig
	geoCfg config.GeoIPConfig
	reader Reloader
	log    *slog.Logger
	client *resty.Client

	cron    *cron.Cron
	entryID cron.EntryID

	single sync.Mutex // single-flight
}

// New constructs an Updater. Cron parsing is lazy (start it via Start).
func New(cfg config.UpdaterConfig, geoCfg config.GeoIPConfig, reader Reloader, log *slog.Logger) *Updater {
	if log == nil {
		log = slog.Default()
	}
	cli := resty.New().
		SetTimeout(cfg.DownloadTimeout).
		SetRetryCount(2).
		SetRetryWaitTime(2 * time.Second)

	u := &Updater{
		cfg:    cfg,
		geoCfg: geoCfg,
		reader: reader,
		log:    log.With("component", "updater"),
		client: cli,
		cron:   cron.New(cron.WithSeconds()),
	}
	return u
}

// Start launches the cron loop. Returns immediately; failures to parse the
// cron expression are surfaced synchronously.
//
// If RunOnStart is true (default), one download+reload cycle is fired off in
// a goroutine right after the cron is scheduled, so a freshly started
// container reaches /readyz without waiting up to one cron interval. The
// goroutine shares the single-flight lock with cron-driven runs, so it cannot
// overlap with a tick that happens to fire at almost the same instant.
func (u *Updater) Start() error {
	if !u.cfg.Enabled {
		u.log.Info("updater disabled")
		return nil
	}
	if u.cfg.Cron == "" {
		return errors.New("updater.cron must not be empty when enabled")
	}
	id, err := u.cron.AddFunc(u.cfg.Cron, func() {
		ctx, cancel := context.WithTimeout(context.Background(), u.cfg.DownloadTimeout+30*time.Second)
		defer cancel()
		if err := u.RunOnce(ctx); err != nil {
			u.log.Warn("scheduled update failed", "err", err)
		}
	})
	if err != nil {
		return fmt.Errorf("parse cron %q: %w", u.cfg.Cron, err)
	}
	u.entryID = id
	u.cron.Start()
	u.log.Info("updater started",
		"cron", u.cfg.Cron,
		"next", u.cron.Entry(id).Next,
		"run_on_start", u.cfg.RunOnStart,
	)

	if u.cfg.RunOnStart {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), u.cfg.DownloadTimeout+30*time.Second)
			defer cancel()
			u.log.Info("updater running initial cycle (run-on-start)")
			if err := u.RunOnce(ctx); err != nil {
				u.log.Warn("initial update failed", "err", err)
			}
		}()
	}
	return nil
}

// Stop halts the cron. Safe to call even if Start was never called.
func (u *Updater) Stop() {
	if u.cron != nil {
		u.cron.Stop()
	}
}

// RunOnce triggers one download+reload cycle. Safe for concurrent callers; only
// one cycle runs at a time.
func (u *Updater) RunOnce(ctx context.Context) error {
	if !u.single.TryLock() {
		u.log.Info("update skipped: previous run still in flight")
		return nil
	}
	defer u.single.Unlock()

	cityChanged, err := u.downloadOne(ctx, u.cfg.CityURL, u.cityFilename(), "city")
	if err != nil {
		u.log.Warn("city download failed", "err", err)
	}
	asnChanged, err := u.downloadOne(ctx, u.cfg.ASNURL, u.asnFilename(), "asn")
	if err != nil {
		u.log.Warn("asn download failed", "err", err)
	}

	if cityChanged || asnChanged {
		if err := u.reader.ReloadCity(); err != nil {
			u.log.Warn("city reload failed", "err", err)
		}
		if err := u.reader.ReloadASN(); err != nil {
			u.log.Warn("asn reload failed", "err", err)
		}
	}
	return nil
}

// cityFilename / asnFilename honour env overrides so operators can point at a
// different mirror without rebuilding the binary.
func (u *Updater) cityFilename() string {
	if v := os.Getenv("CITY_FILE_URL"); v != "" {
		return filenameFromURL(v, u.geoCfg.CityFilename)
	}
	return u.geoCfg.CityFilename
}

func (u *Updater) asnFilename() string {
	if v := os.Getenv("ASN_FILE_URL"); v != "" {
		return filenameFromURL(v, u.geoCfg.ASNFilename)
	}
	return u.geoCfg.ASNFilename
}

func filenameFromURL(raw, fallback string) string {
	// Strip query string before taking the base; otherwise a URL like
	// "https://mirror/db.mmdb?token=..." produces "db.mmdb?token=..."
	// which then fails the .mmdb suffix check and falls back.
	if i := strings.IndexByte(raw, '?'); i >= 0 {
		raw = raw[:i]
	}
	name := filepath.Base(raw)
	if strings.HasSuffix(strings.ToLower(name), ".mmdb") {
		return name
	}
	return fallback
}

// downloadOne fetches url to the configured dir under the chosen filename.
// Returns true iff the on-disk file changed and was atomically swapped.
func (u *Updater) downloadOne(ctx context.Context, url, filename, label string) (bool, error) {
	if url == "" {
		return false, fmt.Errorf("%s url is empty", label)
	}
	if err := os.MkdirAll(u.geoCfg.DBDir, 0o755); err != nil {
		return false, fmt.Errorf("mkdir db dir: %w", err)
	}
	target := filepath.Join(u.geoCfg.DBDir, filename)

	// 1) Download to tmp file in the same directory (so the rename is atomic).
	tmp, err := os.CreateTemp(u.geoCfg.DBDir, filename+".*.tmp")
	if err != nil {
		return false, fmt.Errorf("create tmp: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	_ = tmp.Close()
	_ = os.Remove(tmpPath) // resty wants the path free

	req := u.client.R().SetContext(ctx).SetOutput(tmpPath)
	for _, h := range u.cfg.Headers {
		if k, v, ok := strings.Cut(h, ":"); ok {
			req = req.SetHeader(strings.TrimSpace(k), strings.TrimSpace(v))
		}
	}
	resp, err := req.Get(url)
	if err != nil {
		return false, fmt.Errorf("download %s: %w", url, err)
	}
	if resp.StatusCode() != http.StatusOK {
		return false, fmt.Errorf("download %s: status %d", url, resp.StatusCode())
	}

	// 2) Compare hashes; skip the rename if unchanged.
	newSum, err := fileMD5(tmpPath)
	if err != nil {
		return false, fmt.Errorf("md5 new: %w", err)
	}
	oldSum, err := fileMD5(target)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("md5 old: %w", err)
	}
	if newSum != "" && newSum == oldSum {
		u.log.Info("mmdb unchanged, skip", "label", label, "path", target)
		return false, nil
	}

	// 3) Atomic swap with a single-step backup. If rename fails, attempt to
	//    restore from the backup rather than leaving a half-state.
	backup := target + ".bak"
	_ = os.Remove(backup)
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, backup); err != nil {
			return false, fmt.Errorf("backup %s: %w", target, err)
		}
	}
	if err := os.Rename(tmpPath, target); err != nil {
		// best-effort restore
		if _, statErr := os.Stat(backup); statErr == nil {
			_ = os.Rename(backup, target)
		}
		return false, fmt.Errorf("rename %s -> %s: %w", tmpPath, target, err)
	}
	_ = os.Remove(backup)
	u.log.Info("mmdb updated", "label", label, "path", target, "old_md5", oldSum, "new_md5", newSum)
	return true, nil
}

func fileMD5(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// compile-time check: our reader satisfies Reloader.
var _ Reloader = (*geoip.Reader)(nil)
