package iputil

import (
	"errors"
	"net"
	"net/http"
	"testing"
)

func TestNormalise(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		err  error
	}{
		{"empty", "", "", ErrEmpty},
		{"spaces", "   ", "", ErrEmpty},
		{"unknown", "unknown", "", ErrInvalid},
		{"plain v4", "1.2.3.4", "1.2.3.4", nil},
		{"v4 with port", "1.2.3.4:80", "1.2.3.4", nil},
		{"comma list", "1.2.3.4, 5.6.7.8", "1.2.3.4", nil},
		{"brackets v6", "[2001:db8::1]", "2001:db8::1", nil},
		{"plain v6", "2001:db8::1", "2001:db8::1", nil},
		{"garbage", "not-an-ip", "", ErrInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Normalise(tc.in)
			if !errors.Is(err, tc.err) {
				t.Fatalf("err = %v, want %v", err, tc.err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClientIP(t *testing.T) {
	r := &http.Request{
		Header:     http.Header{},
		RemoteAddr: "9.9.9.9:5000",
	}
	// no headers → fall back to RemoteAddr
	ip, err := ClientIP(r)
	if err != nil {
		t.Fatal(err)
	}
	if ip != "9.9.9.9" {
		t.Fatalf("expected fallback to RemoteAddr, got %s", ip)
	}

	r.Header.Set("X-Forwarded-For", "1.1.1.1, 2.2.2.2")
	ip, _ = ClientIP(r)
	if ip != "1.1.1.1" {
		t.Fatalf("expected first hop from XFF, got %s", ip)
	}

	// XFF already present ⇒ X-Real-IP is not consulted.
	r.Header.Set("X-Real-IP", "3.3.3.3")
	ip, _ = ClientIP(r)
	if ip != "1.1.1.1" {
		t.Fatalf("expected XFF to keep priority, got %s", ip)
	}

	// Drop XFF; X-Real-IP now wins.
	r.Header.Del("X-Forwarded-For")
	ip, _ = ClientIP(r)
	if ip != "3.3.3.3" {
		t.Fatalf("expected X-Real-IP, got %s", ip)
	}
}

func TestIsPrivate(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"10.0.0.1", true},
		{"192.168.1.1", true},
		{"127.0.0.1", true},
		{"169.254.0.1", true},
		{"8.8.8.8", false},
		{"2606:4700:4700::1111", false},
	}
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("bad test ip %s", tc.ip)
			}
			if got := IsPrivate(ip); got != tc.want {
				t.Fatalf("IsPrivate(%s) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}
