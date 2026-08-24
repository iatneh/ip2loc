package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iatneh/ip2loc/internal/config"
	"github.com/iatneh/ip2loc/internal/geoip"
)

// fakeReader lets us exercise the HTTP handlers without mmdb on disk.
type fakeReader struct {
	hasCity bool
	hasASN  bool
	lookup  func(ip, lang string) (*geoip.IPInfo, error)
}

func (f *fakeReader) HasCity() bool { return f.hasCity }
func (f *fakeReader) HasASN() bool  { return f.hasASN }
func (f *fakeReader) Lookup(ip, lang string) (*geoip.IPInfo, error) {
	if f.lookup != nil {
		return f.lookup(ip, lang)
	}
	return &geoip.IPInfo{IP: ip, CountryCode: "JP"}, nil
}

// We can't pass a *fakeReader directly because *Server.reader is typed
// *geoip.Reader. To keep tests honest we test the lower-level handler helpers
// by constructing a real Server with a real Reader pointing at a non-existent
// dir (which means HasCity=false → 503, exercises that branch).

func newTestServer(t *testing.T) *Server {
	t.Helper()
	// Reader with no DBs: city db missing → HasCity=false.
	r, err := geoip.NewReader(config.GeoIPConfig{
		DBDir:        t.TempDir(),
		CityFilename: "missing-city.mmdb",
		ASNFilename:  "missing-asn.mmdb",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })

	return New(config.HTTPConfig{Address: "127.0.0.1", Port: 0}, r, nil)
}

func TestHandlePublicIP(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	w := httptest.NewRecorder()
	s.srv.Handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got != "1.2.3.4" {
		t.Fatalf("body = %q", got)
	}
}

func TestHandleIp2Loc_InvalidIP(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/ip2loc?ip=not-an-ip", nil)
	w := httptest.NewRecorder()
	s.srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var env envelope
	if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if env.Code == 0 {
		t.Fatalf("expected non-zero error code, got %+v", env)
	}
}

func TestHandleIp2Loc_NoIP(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/ip2loc", nil)
	// Public IP via RemoteAddr — won't short-circuit as private.
	req.RemoteAddr = "8.8.8.8:5000"
	w := httptest.NewRecorder()
	s.srv.Handler.ServeHTTP(w, req)

	// Reader has no city db → 503 with code 5001.
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestHandleReadyz_NotReady(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	s.srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestHandleHealthz(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	s.srv.Handler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
}
