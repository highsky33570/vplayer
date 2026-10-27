package maccms

import (
	"net/url"
	"strings"
)

// IsOleHdTvHost reports whether host is a trusted OLEHDTV site host eligible
// for optional Cookie authentication. Subdomains of olehdtv.com are included.
func IsOleHdTvHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	host = strings.TrimSuffix(host, ".")
	if host == "olehdtv.com" {
		return true
	}
	return strings.HasSuffix(host, ".olehdtv.com")
}

// HostFromBaseURL returns the hostname of a configured public base URL, or "".
func HostFromBaseURL(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return ""
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Hostname() == "" {
		// Allow bare host values.
		if !strings.Contains(baseURL, "/") {
			return strings.ToLower(strings.TrimSpace(baseURL))
		}
		return ""
	}
	return strings.ToLower(u.Hostname())
}
