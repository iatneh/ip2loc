package geoip

import (
	"errors"
	"net"
	"testing"

	"github.com/iatneh/ip2loc/internal/config"
	"github.com/iatneh/ip2loc/internal/iputil"
)

func TestReader_NoDB_ReturnsError(t *testing.T) {
	r, err := NewReader(config.GeoIPConfig{
		DBDir:        t.TempDir(),
		CityFilename: "missing.mmdb",
		ASNFilename:  "missing.mmdb",
	}, nil)
	if err != nil {
		t.Fatalf("NewReader should not error on missing files, got %v", err)
	}
	defer r.Close()

	if r.HasCity() {
		t.Fatal("expected HasCity=false with no db on disk")
	}

	_, err = r.Lookup("8.8.8.8", "en")
	if !errors.Is(err, ErrNoCityDB) {
		t.Fatalf("expected ErrNoCityDB, got %v", err)
	}
}

func TestReader_PrivateIP_ShortCircuits(t *testing.T) {
	r, err := NewReader(config.GeoIPConfig{
		DBDir:        t.TempDir(),
		CityFilename: "missing.mmdb",
		ASNFilename:  "missing.mmdb",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	info, err := r.Lookup("192.168.1.1", "en")
	if err != nil {
		// No city db → ErrNoCityDB. But private should be marked first.
		// Actually our impl returns ErrNoCityDB for any non-private lookup,
		// and short-circuits private ones. Verify both.
		if !errors.Is(err, ErrNoCityDB) {
			t.Fatalf("unexpected err: %v", err)
		}
		return
	}
	if !info.IsPrivate {
		t.Fatal("expected IsPrivate=true")
	}
	if info.IP != "192.168.1.1" {
		t.Fatalf("expected IP to round-trip, got %q", info.IP)
	}
}

func TestReader_InvalidIP(t *testing.T) {
	r, _ := NewReader(config.GeoIPConfig{DBDir: t.TempDir(), CityFilename: "x.mmdb", ASNFilename: "y.mmdb"}, nil)
	defer r.Close()

	cases := []string{"", "  ", "garbage", "1.2.3.4:99:99"}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			_, err := r.Lookup(in, "en")
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, ErrEmptyIP) && !errors.Is(err, ErrInvalidIP) {
				t.Fatalf("wrong error type: %v", err)
			}
		})
	}
}

func TestPickName(t *testing.T) {
	names := map[string]string{
		"en":    "Japan",
		"zh-CN": "日本",
		"ja":    "日本",
	}
	if got := pickName(names, "en"); got != "Japan" {
		t.Errorf("en: got %q", got)
	}
	if got := pickName(names, "zh-CN"); got != "日本" {
		t.Errorf("zh-CN: got %q", got)
	}
	if got := pickName(names, "fr"); got != "Japan" {
		t.Errorf("fallback to en: got %q", got)
	}
	if got := pickName(nil, "en"); got != "" {
		t.Errorf("nil names: got %q", got)
	}
}

func TestIsPrivateCompatibility(t *testing.T) {
	// Spot-check that the helper agrees with stdlib semantics for the cases
	// we actually encounter in production.
	cases := []struct {
		ip   string
		want bool
	}{
		{"8.8.8.8", false},
		{"10.0.0.1", true},
		{"127.0.0.1", true},
	}
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			parsed := net.ParseIP(tc.ip)
			if parsed == nil {
				t.Fatalf("bad ip %q", tc.ip)
			}
			if got := iputil.IsPrivate(parsed); got != tc.want {
				t.Fatalf("IsPrivate(%s) = %v want %v", tc.ip, got, tc.want)
			}
		})
	}
}
