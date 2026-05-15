// Package handler provides HTTP request handling for the ladder proxy service.
// It fetches remote URLs on behalf of clients, stripping paywalls and tracking.
package handler

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultUserAgent mimics a common browser to avoid bot detection
	DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	// MaxResponseSize limits the size of proxied responses to 10MB
	MaxResponseSize = 10 * 1024 * 1024
	// RequestTimeout is the maximum time allowed for a proxied request
	// Increased from 30s to 45s to better handle slow news sites
	RequestTimeout = 45 * time.Second
	// MaxRedirects is the maximum number of redirects to follow before giving up
	MaxRedirects = 5
)

// Handler holds configuration and dependencies for the proxy handler.
type Handler struct {
	client    *http.Client
	userAgent string
	rulesFile string
}

// New creates a new Handler with the given options.
func New(userAgent, rulesFile string) *Handler {
	if userAgent == "" {
		userAgent = DefaultUserAgent
	}
	return &Handler{
		client: &http.Client{
			Timeout: RequestTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= MaxRedirects {
					return fmt.Errorf("too many redirects (max %d)", MaxRedirects)
				}
				return nil
			},
		},
		userAgent: userAgent,
		rulesFile: rulesFile,
	}
}

// ServeHTTP handles incoming proxy requests.
// The target URL is extracted from the request path.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract target URL from the path (e.g. /https://example.com/article)
	rawTarget := strings.TrimPrefix(r.URL.Path, "/")
	if rawTarget == "" {
		http.Error(w, "missing target URL", http.StatusBadRequest)
		return
	}

	// Preserve query string if present
	if r.URL.RawQuery != "" {
		rawTarget += "?" + r.URL.RawQuery
	}

	target, err := url.ParseRequestURI(rawTarget)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid target URL: %v", err), http.StatusBadRequest)
		return
	}

	if target.Scheme != "http" && target.Scheme != "https" {
		http.Error(w, "only http and https schemes are supported", http.StatusBadRequest)
		return
	}

	log.Printf("proxying request: %s", target.String())

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target.String(), nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create request: %v", err), http.StatusInternalServerError)
		return
	}

	req.Header.Set("User-Agent", h.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := h.client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to fetch URL: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy relevant response headers
	for _, header := range []string{"Content-Type", "Content-Language", "Last-Modified"} {
		if val := resp.Header.Get(header); val != "" {
			w.Header().Set(header, val)
		}
	}

