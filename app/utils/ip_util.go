package utils

import (
	"github.com/sirupsen/logrus"
	"net"
	"net/http"
	"strings"
)

var (
	ipHeaders = []string{
		"X-Realip-For-Api-Gateway",
		"X-Forwarded-For",
		"x-real-ip",
		"Proxy-Client-IP",
		"WL-Proxy-Client-IP",
		"HTTP_X_FORWARDED_FOR",
		"HTTP_X_FORWARDED",
		"HTTP_X_CLUSTER_CLIENT_IP",
		"HTTP_CLIENT_IP",
		"REMOTE_ADDR"}
)

func NormalizeIP(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.EqualFold(raw, "unknown") {
		return ""
	}
	if strings.Contains(raw, ",") {
		parts := strings.Split(raw, ",")
		if len(parts) > 0 {
			raw = strings.TrimSpace(parts[0])
		}
	}
	raw = strings.Trim(raw, "[]")
	if ip := net.ParseIP(raw); ip != nil {
		return raw
	}
	host, _, err := net.SplitHostPort(raw)
	if err == nil {
		host = strings.Trim(host, "[]")
		if ip := net.ParseIP(host); ip != nil {
			return host
		}
	}
	return ""
}

func GetClientIP(req *http.Request) (ip string) {
	var ipSlice []string
	for _, header := range ipHeaders {
		ipSlice = append(ipSlice, req.Header.Get(header))
	}
	logrus.Infof("client request header check gives ips: %v", ipSlice)
	for _, v := range ipSlice {
		if normalized := NormalizeIP(v); normalized != "" {
			return normalized
		}
	}
	// 请求头中获取不到IP,获取RemoteAddr返回
	return NormalizeIP(req.RemoteAddr)

}
