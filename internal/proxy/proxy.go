package proxy

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

const proxyPath = "/api/futur/itemavailability"

type Config struct {
	// FuturAPIURL is the base Futur API URL (e.g. "https://api.salhydro.fi/api/futur")
	// The scheme+host is extracted as the proxy target.
	FuturAPIURL string
	// Port is the HTTP port to listen on (e.g. "80")
	Port string
}

func Start(cfg *Config) error {
	parsed, err := url.Parse(cfg.FuturAPIURL)
	if err != nil {
		return fmt.Errorf("invalid FUTUR_API_URL: %w", err)
	}

	target := &url.URL{
		Scheme: parsed.Scheme,
		Host:   parsed.Host,
	}
	// Force HTTPS for the upstream
	if target.Scheme == "http" && !strings.Contains(target.Host, "localhost") {
		target.Scheme = "https"
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	// Override the Director to set the correct Host header and forward client IP
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
		// X-Forwarded-For is added automatically by ReverseProxy.
		// Add X-Real-IP with the client's address.
		if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
			req.Header.Set("X-Real-IP", clientIP)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != proxyPath {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}

		log.Printf("[HTTP Proxy] %s %s -> %s%s", r.Method, r.URL.Path, target.String(), r.URL.Path)
		proxy.ServeHTTP(w, r)
	})

	addr := ":" + cfg.Port
	log.Printf("HTTP proxy listening on %s, forwarding %s to %s", addr, proxyPath, target.String())
	return http.ListenAndServe(addr, mux)
}
