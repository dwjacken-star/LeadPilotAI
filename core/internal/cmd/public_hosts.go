package cmd

import (
	"net"
	"strings"
)

// An optional exact host allowlist retires old public hostnames without keeping
// customer-specific domain names in application code. Empty preserves defaults.
func publicHostAllowed(host, configured string) bool {
	if strings.TrimSpace(configured) == "" {
		return true
	}
	if h, _, e := net.SplitHostPort(host); e == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSuffix(strings.Trim(host, "[]"), "."))
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	for _, allowed := range strings.Split(configured, ",") {
		if host == strings.ToLower(strings.TrimSpace(allowed)) {
			return true
		}
	}
	return false
}
