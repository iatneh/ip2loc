package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iatneh/ip2loc/internal/config"
)

// stubReloader records reload calls.
type stubReloader struct {
	city atomic.Int32
	asn  atomic.Int32
	fail error
}

func (s *stubReloader) ReloadCity() error {
	if s.fail != nil {
		return s.fail
	}
	s.city.Add(1)
	return nil
}
func (s *stubReloader) ReloadASN() error {
	if s.fail != nil {
		return s.fail
	}
	s.asn.Add(1)
	return nil
}

func TestRunOnce_EmptyURLs_Noop(t *testing.T) {
	dir := t.TempDir()
	cfg := config.UpdaterConfig{
		CityURL: "",
		ASNURL:  "",
	}
	g := config.GeoIPConfig{
		DBDir:        dir,
		CityFilename: "city.mmdb",
		ASNFilename:  "asn.mmdb",
	}
	r := &stubReloader{}
	u := New(cfg, g, r, nil)

	if err := u.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if r.city.Load() != 0 || r.asn.Load() != 0 {
		t.Fatalf("expected no reloads on empty urls")
	}
}

func TestFileMD5(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.bin")
	if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := fileMD5(p)
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Fatal("expected md5")
	}
	// Missing file → empty string, no error.
	if got, err := fileMD5(filepath.Join(dir, "nope")); err != nil || got != "" {
		t.Errorf("missing file: got=%q err=%v", got, err)
	}
}

func TestFilenameFromURL(t *testing.T) {
	cases := map[string]string{
		"https://example.com/GeoLite2-City.mmdb?x=1": "GeoLite2-City.mmdb",
		"https://example.com/page":                   "fallback.mmdb",
	}
	for in, want := range cases {
		if got := filenameFromURL(in, "fallback.mmdb"); got != want {
			t.Errorf("filenameFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStart_DisabledIsNoop(t *testing.T) {
	cfg := config.UpdaterConfig{Enabled: false, Cron: "0 0 * * * *"}
	g := config.GeoIPConfig{DBDir: t.TempDir()}
	u := New(cfg, g, &stubReloader{}, nil)
	if err := u.Start(); err != nil {
		t.Fatalf("Start when disabled: %v", err)
	}
	u.Stop()
}

func TestStart_BadCron(t *testing.T) {
	cfg := config.UpdaterConfig{Enabled: true, Cron: "not a cron"}
	u := New(cfg, config.GeoIPConfig{DBDir: t.TempDir()}, &stubReloader{}, nil)
	if err := u.Start(); err == nil {
		t.Fatal("expected cron parse error")
	}
	u.Stop()
}

func TestRunOnce_ContextCanceled(t *testing.T) {
	cfg := config.UpdaterConfig{
		Enabled:         true,
		CityURL:         "http://127.0.0.1:1/will-fail.mmdb",
		DownloadTimeout: 50 * time.Millisecond,
	}
	g := config.GeoIPConfig{
		DBDir:        t.TempDir(),
		CityFilename: "city.mmdb",
	}
	u := New(cfg, g, &stubReloader{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Should not panic, may log warn.
	if err := u.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
		t.Logf("got err (expected nil or canceled): %v", err)
	}
}
