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

func GetClientIP(req *http.Request) (ip string) {
	var ipSlice []string
	for _, header := range ipHeaders {
		ipSlice = append(ipSlice, req.Header.Get(header))
	}
	logrus.Infof("client request header check gives ips: %v", ipSlice)
	for _, v := range ipSlice {
		if v != "" {
			return v
		}
	}
	// 请求头中获取不到IP,获取RemoteAddr返回
	host, _, _ := net.SplitHostPort(strings.TrimSpace(req.RemoteAddr))

	return host

}
