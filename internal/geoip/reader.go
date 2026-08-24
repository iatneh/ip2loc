// Package geoip is the core of the service: it owns the MaxMind .mmdb readers
// and turns IP addresses into structured IPInfo records.
//
// Two readers live here (City + ASN), each behind an atomic pointer so an
// in-flight query never sees a half-mmap'd file. Reloading is a swap of the
// pointer; old readers close on a goroutine so a slow lookup cannot stall a
// reload.
package geoip

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oschwald/geoip2-golang"

	"github.com/iatneh/ip2loc/internal/config"
	"github.com/iatneh/ip2loc/internal/iputil"
)

// IPInfo is the public response shape.
//
// Tags match the original project; omitempty trims noise on partial lookups
// (private IPs, missing ASN, etc).
type IPInfo struct {
	IP                string  `json:"ip"`
	Latitude          float64 `json:"latitude"`
	Longitude         float64 `json:"longitude"`
	ContinentCode     string  `json:"continentCode,omitempty"`
	ContinentName     string  `json:"continentName,omitempty"`
	CountryCode       string  `json:"countryCode"`
	CountryName       string  `json:"countryName"`
	RegionCode        string  `json:"regionCode,omitempty"`
	RegionName        string  `json:"regionName,omitempty"`
	CityName          string  `json:"cityName,omitempty"`
	PostalCode        string  `json:"postalCode,omitempty"`
	TimeZone          string  `json:"timeZone,omitempty"`
	IsInEuropeanUnion bool    `json:"isInEuropeanUnion"`
	AccuracyRadius    uint16  `json:"accuracyRadius,omitempty"`
	ASN               uint    `json:"asn,omitempty"`
	ASOrg             string  `json:"asOrg,omitempty"`
	IsPrivate         bool    `json:"isPrivate"`
}

// Lookup errors. Mapped to HTTP status codes by the server layer.
var (
	ErrNoCityDB  = errors.New("city mmdb is not loaded")
	ErrNoRecord  = errors.New("no record for ip")
	ErrInvalidIP = errors.New("invalid ip")
	ErrEmptyIP   = errors.New("empty ip")
)

// Reader is the public API. Construct via NewReader; it is safe for concurrent
// use.
type Reader struct {
	cfg config.GeoIPConfig

	city atomic.Pointer[geoip2.Reader]
	asn  atomic.Pointer[geoip2.Reader]

	// closeQueue hands old readers to a background goroutine that closes them.
	// maxmind readers hold an mmap; closing from the query path would briefly
	// fault active lookups.
	closeCh   chan *geoip2.Reader
	closeOnce sync.Once
	wg        sync.WaitGroup
	log       *slog.Logger
}

// NewReader opens whatever mmdb files it can find under cfg.DBDir and starts
// the close-helper goroutine. Missing files are logged at WARN level — the
// service can still answer "I have no DB" rather than crash.
func NewReader(cfg config.GeoIPConfig, log *slog.Logger) (*Reader, error) {
	if log == nil {
		log = slog.Default()
	}
	r := &Reader{
		cfg:     cfg,
		closeCh: make(chan *geoip2.Reader, 8),
		log:     log.With("component", "geoip"),
	}
	r.wg.Add(1)
	go r.closeLoop()

	if err := os.MkdirAll(cfg.DBDir, 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	if err := r.openCity(); err != nil {
		log.Warn("city mmdb not loaded", "path", r.cityPath(), "err", err)
	}
	if err := r.openASN(); err != nil {
		log.Warn("asn mmdb not loaded", "path", r.asnPath(), "err", err)
	}
	return r, nil
}

// Close releases all mmdb mappings and stops the helper goroutine. Safe to
// call multiple times.
func (r *Reader) Close() error {
	r.closeOnce.Do(func() { close(r.closeCh) })
	r.wg.Wait()
	var firstErr error
	if c := r.city.Load(); c != nil {
		if err := c.Close(); err != nil {
			firstErr = err
		}
	}
	if a := r.asn.Load(); a != nil {
		if err := a.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// HasCity reports whether a City db is loaded.
func (r *Reader) HasCity() bool { return r.city.Load() != nil }

// HasASN reports whether an ASN db is loaded.
func (r *Reader) HasASN() bool { return r.asn.Load() != nil }

// Lookup resolves ip → IPInfo using the configured DBs. lang is the preferred
// language for localised names; pass "" for default (en).
func (r *Reader) Lookup(rawIP, lang string) (*IPInfo, error) {
	ipStr, err := iputil.Normalise(rawIP)
	if err != nil {
		if errors.Is(err, iputil.ErrEmpty) {
			return nil, ErrEmptyIP
		}
		return nil, ErrInvalidIP
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, ErrInvalidIP
	}

	info := &IPInfo{
		IP:        ipStr,
		IsPrivate: iputil.IsPrivate(ip),
	}

	if lang == "" {
		lang = r.cfg.DefaultLang
	}

	// Private IPs: skip DB lookup entirely, return the marker.
	if info.IsPrivate {
		return info, nil
	}

	city := r.city.Load()
	if city == nil {
		// No DB → degrade gracefully: at least we know it's public.
		return nil, ErrNoCityDB
	}

	record, err := city.City(ip)
	if err != nil {
		return nil, fmt.Errorf("city lookup: %w", err)
	}

	fillFromCity(info, record, lang)

	if asn := r.asn.Load(); asn != nil {
		if rec, err := asn.ASN(ip); err == nil && rec != nil {
			info.ASN = rec.AutonomousSystemNumber
			info.ASOrg = rec.AutonomousSystemOrganization
		}
	}
	return info, nil
}

// ReloadCity re-opens the city mmdb file. Returns ErrNotLoaded if the file is
// missing. The old reader (if any) is closed asynchronously.
func (r *Reader) ReloadCity() error {
	return r.swapCity()
}

// ReloadASN re-opens the ASN mmdb file.
func (r *Reader) ReloadASN() error {
	return r.swapASN()
}

// ReloadAll reloads both readers; used by the file updater after a successful
// download.
func (r *Reader) ReloadAll() error {
	var errs []string
	if err := r.ReloadCity(); err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, "city: "+err.Error())
	}
	if err := r.ReloadASN(); err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, "asn: "+err.Error())
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// --- internals ---------------------------------------------------------------

func (r *Reader) cityPath() string { return filepath.Join(r.cfg.DBDir, r.cfg.CityFilename) }
func (r *Reader) asnPath() string  { return filepath.Join(r.cfg.DBDir, r.cfg.ASNFilename) }

func (r *Reader) openCity() error {
	return r.openReader(r.cityPath(), &r.city, true)
}

func (r *Reader) openASN() error {
	return r.openReader(r.asnPath(), &r.asn, false)
}

// openReader opens path, swaps the pointer atomically, and queues the previous
// reader for deferred close.
func (r *Reader) openReader(path string, slot *atomic.Pointer[geoip2.Reader], required bool) error {
	if _, err := os.Stat(path); err != nil {
		if required {
			return fmt.Errorf("stat %s: %w", path, err)
		}
		return err
	}
	newR, err := geoip2.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	old := slot.Swap(newR)
	if old != nil {
		select {
		case r.closeCh <- old:
		default:
			// channel full → close inline; better than leaking.
			_ = old.Close()
		}
	}
	r.log.Info("mmdb loaded", "path", path)
	return nil
}

func (r *Reader) swapCity() error { return r.openReader(r.cityPath(), &r.city, true) }
func (r *Reader) swapASN() error  { return r.openReader(r.asnPath(), &r.asn, false) }

// closeLoop drains replaced readers on a goroutine so the hot path never
// blocks on Close().
func (r *Reader) closeLoop() {
	defer r.wg.Done()
	for rdr := range r.closeCh {
		if rdr == nil {
			continue
		}
		if err := rdr.Close(); err != nil {
			r.log.Warn("close old reader failed", "err", err)
		}
	}
}

// fillFromCity copies a City record into the public IPInfo, picking localised
// names according to lang.
func fillFromCity(dst *IPInfo, rec *geoip2.City, lang string) {
	dst.Latitude = rec.Location.Latitude
	dst.Longitude = rec.Location.Longitude
	dst.ContinentCode = rec.Continent.Code
	dst.ContinentName = pickName(rec.Continent.Names, lang)
	dst.CountryCode = rec.Country.IsoCode
	dst.CountryName = pickName(rec.Country.Names, lang)
	dst.TimeZone = rec.Location.TimeZone
	dst.IsInEuropeanUnion = rec.Country.IsInEuropeanUnion
	dst.AccuracyRadius = rec.Location.AccuracyRadius

	if len(rec.Subdivisions) > 0 {
		dst.RegionCode = rec.Subdivisions[0].IsoCode
		dst.RegionName = pickName(rec.Subdivisions[0].Names, lang)
	}
	if len(rec.City.Names) > 0 {
		dst.CityName = pickName(rec.City.Names, lang)
	}
	if rec.Postal.Code != "" {
		dst.PostalCode = rec.Postal.Code
	}
}

// pickName returns the best name for lang, falling back to en → zh-CN → the
// first non-empty value. Mirrors the original project.
func pickName(names map[string]string, lang string) string {
	if len(names) == 0 {
		return ""
	}
	if lang != "" {
		if v := names[lang]; v != "" {
			return v
		}
		if i := strings.Index(lang, "-"); i > 0 {
			if v := names[lang[:i]]; v != "" {
				return v
			}
		}
	}
	if v := names["en"]; v != "" {
		return v
	}
	if v := names["zh-CN"]; v != "" {
		return v
	}
	for _, v := range names {
		if v != "" {
			return v
		}
	}
	return ""
}

// isAddrNotFound matches the upstream "no record for ip" outcome by string
// matching. Newer maxminddb-golang versions don't expose a typed error; the
// underlying Lookup returns nil for "no entry", but we keep this helper for
// resilience against future changes.
func isAddrNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no data") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "AddressNotFound")
}

// be cleaner but adds another dep; mtime polling at 30s is cheap and matches
// the cron cadence.
type Watcher struct {
	reader   *Reader
	interval time.Duration
	log      *slog.Logger
	stop     chan struct{}
}

// NewWatcher constructs a poller; call Start to run.
func NewWatcher(r *Reader, interval time.Duration, log *slog.Logger) *Watcher {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if log == nil {
		log = slog.Default()
	}
	return &Watcher{
		reader:   r,
		interval: interval,
		log:      log.With("component", "watcher"),
		stop:     make(chan struct{}),
	}
}

// Start launches the polling loop in a goroutine.
func (w *Watcher) Start() {
	go w.run()
}

// Stop signals the loop to exit. Blocks until the goroutine returns.
func (w *Watcher) Stop() {
	close(w.stop)
}

func (w *Watcher) run() {
	cityM, asnM := w.snapshot()
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-w.stop:
			return
		case <-t.C:
			if newCity, err := os.Stat(w.reader.cityPath()); err == nil {
				if !newCity.ModTime().Equal(cityM) {
					w.log.Info("city mmdb changed, reloading")
					if err := w.reader.ReloadCity(); err != nil {
						w.log.Warn("city reload failed", "err", err)
					} else {
						cityM = newCity.ModTime()
					}
				}
			}
			if newASN, err := os.Stat(w.reader.asnPath()); err == nil {
				if !newASN.ModTime().Equal(asnM) {
					w.log.Info("asn mmdb changed, reloading")
					if err := w.reader.ReloadASN(); err != nil {
						w.log.Warn("asn reload failed", "err", err)
					} else {
						asnM = newASN.ModTime()
					}
				}
			}
		}
	}
}

func (w *Watcher) snapshot() (time.Time, time.Time) {
	cityM := time.Time{}
	asnM := time.Time{}
	if fi, err := os.Stat(w.reader.cityPath()); err == nil {
		cityM = fi.ModTime()
	}
	if fi, err := os.Stat(w.reader.asnPath()); err == nil {
		asnM = fi.ModTime()
	}
	return cityM, asnM
}
