package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/net/html"
)

var (
	ErrInvalidLinkPreviewURL = errors.New("invalid link preview url")
	ErrBlockedLinkPreviewURL = errors.New("link preview url is blocked")
	ErrNoLinkPreview         = errors.New("no link preview available")
)

type LinkPreview struct {
	URL         string `json:"url,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	ImageURL    string `json:"image_url,omitempty"`
	SiteName    string `json:"site_name,omitempty"`
	Domain      string `json:"domain,omitempty"`
}

type LinkPreviewFetcher func(ctx context.Context, rawURL string) (*LinkPreview, error)

type linkPreviewCacheEntry struct {
	preview   *LinkPreview
	expiresAt time.Time
	touchedAt time.Time
}

type LinkPreviewService struct {
	logger            zerolog.Logger
	client            *http.Client
	resolver          *net.Resolver
	dialer            *net.Dialer
	ttl               time.Duration
	maxEntries        int
	allowPrivateHosts bool

	mu    sync.Mutex
	cache map[string]linkPreviewCacheEntry
}

func NewLinkPreviewService(logger zerolog.Logger) *LinkPreviewService {
	service := &LinkPreviewService{
		logger:     logger,
		resolver:   net.DefaultResolver,
		dialer:     &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second},
		ttl:        6 * time.Hour,
		maxEntries: 256,
		cache:      make(map[string]linkPreviewCacheEntry),
	}
	service.client = service.newHTTPClient()
	return service
}

func (s *LinkPreviewService) newHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           s.dialPreviewContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
	}
	return &http.Client{
		Timeout:   6 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if _, _, err := normalizeLinkPreviewURL(req.URL.String()); err != nil {
				return err
			}
			if !s.allowPrivateHosts {
				if err := ensureSafePreviewPort(req.URL); err != nil {
					return err
				}
				if err := ensurePublicPreviewHostWithResolver(req.Context(), req.URL, s.resolver); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func (s *LinkPreviewService) dialPreviewContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("parse preview address: %w", err)
	}
	if s.allowPrivateHosts {
		return s.dialer.DialContext(ctx, network, address)
	}

	ips, err := publicPreviewIPs(ctx, host, s.resolver)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, ip := range ips {
		conn, dialErr := s.dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	if lastErr == nil {
		lastErr = ErrBlockedLinkPreviewURL
	}
	return nil, fmt.Errorf("dial preview host: %w", lastErr)
}

func (s *LinkPreviewService) Fetch(ctx context.Context, rawURL string) (*LinkPreview, error) {
	normalizedURL, parsedURL, err := normalizeLinkPreviewURL(rawURL)
	if err != nil {
		return nil, err
	}
	if !s.allowPrivateHosts {
		if err := ensureSafePreviewPort(parsedURL); err != nil {
			return nil, err
		}
		if err := ensurePublicPreviewHostWithResolver(ctx, parsedURL, s.resolver); err != nil {
			return nil, err
		}
	}

	if preview := s.cached(normalizedURL); preview != nil {
		return preview, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, normalizedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidLinkPreviewURL, err)
	}
	req.Header.Set("User-Agent", "OpenMessage/1.0 (+https://openmessage.ai)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch link preview: %w", err)
	}
	defer resp.Body.Close()

	if resp.Request != nil && resp.Request.URL != nil && !s.allowPrivateHosts {
		if err := ensurePublicPreviewHostWithResolver(ctx, resp.Request.URL, s.resolver); err != nil {
			return nil, err
		}
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("fetch link preview: unexpected status %d", resp.StatusCode)
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if contentType != "" && !strings.Contains(contentType, "text/html") && !strings.Contains(contentType, "application/xhtml+xml") {
		return nil, ErrNoLinkPreview
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read link preview: %w", err)
	}

	finalURL := parsedURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL
	}

	preview, err := extractLinkPreview(body, finalURL)
	if err != nil {
		return nil, err
	}
	preview.URL = finalURL.String()
	preview.Domain = finalURL.Hostname()
	if preview.SiteName == "" {
		preview.SiteName = prettifyPreviewHost(finalURL.Hostname())
	}

	s.store(normalizedURL, preview)
	return cloneLinkPreview(preview), nil
}

func (s *LinkPreviewService) cached(rawURL string) *LinkPreview {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.cache[rawURL]
	if !ok {
		return nil
	}
	if time.Now().After(entry.expiresAt) {
		delete(s.cache, rawURL)
		return nil
	}
	entry.touchedAt = time.Now()
	s.cache[rawURL] = entry
	return cloneLinkPreview(entry.preview)
}

func (s *LinkPreviewService) store(rawURL string, preview *LinkPreview) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.pruneExpiredLocked(now)
	s.cache[rawURL] = linkPreviewCacheEntry{
		preview:   cloneLinkPreview(preview),
		expiresAt: now.Add(s.ttl),
		touchedAt: now,
	}
	s.evictIfNeededLocked()
}

func (s *LinkPreviewService) pruneExpiredLocked(now time.Time) {
	for rawURL, entry := range s.cache {
		if now.After(entry.expiresAt) {
			delete(s.cache, rawURL)
		}
	}
}

func (s *LinkPreviewService) evictIfNeededLocked() {
	if s.maxEntries <= 0 {
		return
	}
	for len(s.cache) > s.maxEntries {
		oldestURL := ""
		var oldestTouched time.Time
		for rawURL, entry := range s.cache {
			if oldestURL == "" || entry.touchedAt.Before(oldestTouched) {
				oldestURL = rawURL
				oldestTouched = entry.touchedAt
			}
		}
		if oldestURL == "" {
			return
		}
		delete(s.cache, oldestURL)
	}
}

func cloneLinkPreview(preview *LinkPreview) *LinkPreview {
	if preview == nil {
		return nil
	}
	copy := *preview
	return &copy
}

func normalizeLinkPreviewURL(raw string) (string, *url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil, ErrInvalidLinkPreviewURL
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrInvalidLinkPreviewURL, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", nil, ErrInvalidLinkPreviewURL
	}
	if parsed.Hostname() == "" {
		return "", nil, ErrInvalidLinkPreviewURL
	}
	if parsed.User != nil {
		return "", nil, ErrInvalidLinkPreviewURL
	}
	parsed.Fragment = ""
	return parsed.String(), parsed, nil
}

func ensureSafePreviewPort(target *url.URL) error {
	port := target.Port()
	if port == "" || (target.Scheme == "http" && port == "80") || (target.Scheme == "https" && port == "443") {
		return nil
	}
	return ErrBlockedLinkPreviewURL
}

func ensurePublicPreviewHost(ctx context.Context, target *url.URL) error {
	return ensurePublicPreviewHostWithResolver(ctx, target, net.DefaultResolver)
}

func ensurePublicPreviewHostWithResolver(ctx context.Context, target *url.URL, resolver *net.Resolver) error {
	host := strings.ToLower(target.Hostname())
	if host == "" {
		return ErrInvalidLinkPreviewURL
	}
	if host == "localhost" || strings.HasSuffix(host, ".local") {
		return ErrBlockedLinkPreviewURL
	}

	_, err := publicPreviewIPs(ctx, host, resolver)
	return err
}

func publicPreviewIPs(ctx context.Context, host string, resolver *net.Resolver) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if isPrivatePreviewIP(ip) {
			return nil, ErrBlockedLinkPreviewURL
		}
		return []net.IP{ip}, nil
	}
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve preview host: %w", err)
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		if isPrivatePreviewIP(addr.IP) {
			return nil, ErrBlockedLinkPreviewURL
		}
		ips = append(ips, addr.IP)
	}
	if len(ips) == 0 {
		return nil, ErrBlockedLinkPreviewURL
	}
	return ips, nil
}

func isPrivatePreviewIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return !ip.IsGlobalUnicast() ||
		ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsUnspecified()
}

func extractLinkPreview(body []byte, baseURL *url.URL) (*LinkPreview, error) {
	meta := map[string]string{}
	title := ""
	inTitle := false
	tokenizer := html.NewTokenizer(bytes.NewReader(body))
	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			if err := tokenizer.Err(); err != nil && err != io.EOF {
				return nil, fmt.Errorf("parse link preview: %w", err)
			}
			goto parsed
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			switch strings.ToLower(token.Data) {
			case "body":
				goto parsed
			case "title":
				inTitle = true
			case "meta":
				key, content := "", ""
				for _, attr := range token.Attr {
					switch strings.ToLower(attr.Key) {
					case "property", "name", "itemprop":
						if key == "" {
							key = strings.ToLower(strings.TrimSpace(attr.Val))
						}
					case "content":
						content = collapsePreviewWhitespace(attr.Val)
					}
				}
				if key != "" && content != "" {
					if _, exists := meta[key]; !exists {
						meta[key] = content
					}
				}
			}
		case html.EndTagToken:
			if strings.EqualFold(tokenizer.Token().Data, "title") {
				inTitle = false
			}
		case html.TextToken:
			if inTitle && title == "" {
				title = collapsePreviewWhitespace(string(tokenizer.Text()))
			}
		}
	}

parsed:

	preview := &LinkPreview{
		Title:       firstNonEmpty(meta["og:title"], meta["twitter:title"], meta["title"], title),
		Description: firstNonEmpty(meta["og:description"], meta["twitter:description"], meta["description"]),
		// External preview images are intentionally omitted. Letting the browser
		// fetch arbitrary metadata URLs would bypass the server's IP filtering.
		ImageURL: "",
		SiteName: firstNonEmpty(meta["og:site_name"], meta["application-name"]),
	}

	preview.Title = truncatePreviewText(preview.Title, 180)
	preview.Description = truncatePreviewText(preview.Description, 320)
	if preview.Title == "" && preview.Description == "" && preview.ImageURL == "" && preview.SiteName == "" {
		return nil, ErrNoLinkPreview
	}
	return preview, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func collapsePreviewWhitespace(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func truncatePreviewText(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 1 {
		return value[:limit]
	}
	return strings.TrimSpace(value[:limit-1]) + "…"
}

func resolvePreviewURL(baseURL *url.URL, raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "//") {
		if baseURL == nil || baseURL.Scheme == "" {
			return "https:" + trimmed
		}
		return baseURL.Scheme + ":" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	if parsed.IsAbs() {
		return parsed.String()
	}
	if baseURL == nil {
		return ""
	}
	return baseURL.ResolveReference(parsed).String()
}

func prettifyPreviewHost(host string) string {
	trimmed := strings.ToLower(strings.TrimSpace(host))
	trimmed = strings.TrimPrefix(trimmed, "www.")
	trimmed = strings.TrimSpace(trimmed)
	trimmed = strings.TrimSuffix(trimmed, path.Ext(trimmed))
	if trimmed == "" {
		return host
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) == 0 {
		return host
	}
	label := strings.ReplaceAll(parts[0], "-", " ")
	if label == "" {
		return host
	}
	return strings.ToUpper(label[:1]) + label[1:]
}
