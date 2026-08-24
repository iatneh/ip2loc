// Package iputil handles IP-address parsing, normalisation and client-IP
// extraction from HTTP requests.
//
// Normalise() is intentionally permissive: people paste "1.2.3.4:80",
// "[2001:db8::1]", "1.2.3.4, 10.0.0.1", and the empty string. We return the
// first usable IP or "".
package iputil

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// Common errors returned by Normalise.
var (
	ErrEmpty   = errors.New("ip is empty")
	ErrInvalid = errors.New("ip is invalid")
)

// Headers inspected, in order, when extracting the originating client IP.
// Reverse proxies and CDNs each have their own convention; we cover the usual
// suspects so the operator can deploy behind any of them.
var clientIPHeaders = []string{
	"X-Forwarded-For",
	"X-Real-IP",
	"X-Client-IP",
	"CF-Connecting-IP",
	"True-Client-IP",
	"Fastly-Client-IP",
	"X-Cluster-Client-IP",
	"X-Original-Forwarded-For",
	"Proxy-Client-IP",
	"WL-Proxy-Client-IP",
	"HTTP_X_FORWARDED_FOR",
	"HTTP_X_FORWARDED",
	"HTTP_CLIENT_IP",
}

// Normalise takes a raw string and returns a canonical IP literal, or an empty
// string. Errors are returned as ErrEmpty / ErrInvalid so callers can map them
// to HTTP status codes without parsing strings.
func Normalise(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrEmpty
	}
	// "1.2.3.4, 5.6.7.8" → take the leftmost
	if strings.Contains(raw, ",") {
		raw = strings.TrimSpace(strings.SplitN(raw, ",", 2)[0])
	}
	raw = strings.Trim(raw, "[]")

	if ip := net.ParseIP(raw); ip != nil {
		return ip.String(), nil
	}
	if host, _, err := net.SplitHostPort(raw); err == nil {
		host = strings.TrimSpace(strings.Trim(host, "[]"))
		if ip := net.ParseIP(host); ip != nil {
			return ip.String(), nil
		}
	}
	return "", fmt.Errorf("%w: %q", ErrInvalid, raw)
}

// ClientIP returns the best-effort originating IP for an HTTP request. It
// inspects forwarded headers in priority order and falls back to RemoteAddr.
func ClientIP(r *http.Request) (string, error) {
	for _, h := range clientIPHeaders {
		v := strings.TrimSpace(r.Header.Get(h))
		if v == "" || strings.EqualFold(v, "unknown") {
			continue
		}
		if ip, err := Normalise(v); err == nil {
			return ip, nil
		}
	}
	return Normalise(r.RemoteAddr)
}

// IsPrivate reports whether the IP is in a non-routable range (RFC1918,
// loopback, link-local, unspecified, multicast, etc).
func IsPrivate(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return ip.IsPrivate() ||
		ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() ||
		ip.IsMulticast()
}

// Parse is the strict variant of Normalise: empty / invalid → error.
func Parse(raw string) (net.IP, error) {
	s, err := Normalise(raw)
	if err != nil {
		return nil, err
	}
	return net.ParseIP(s), nil
}
