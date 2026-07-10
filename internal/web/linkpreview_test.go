package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestLinkPreviewServiceFetchParsesMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html>
<html>
  <head>
    <title>Ignored Title</title>
    <meta property="og:title" content="Preview Title">
    <meta property="og:description" content="Preview Description">
    <meta property="og:image" content="/card.png">
    <meta property="og:site_name" content="Preview Site">
  </head>
  <body>Hello</body>
</html>`))
	}))
	defer srv.Close()

	service := NewLinkPreviewService(zerolog.Nop())
	service.allowPrivateHosts = true

	preview, err := service.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Title != "Preview Title" {
		t.Fatalf("got title %q", preview.Title)
	}
	if preview.Description != "Preview Description" {
		t.Fatalf("got description %q", preview.Description)
	}
	if preview.SiteName != "Preview Site" {
		t.Fatalf("got site name %q", preview.SiteName)
	}
	if preview.ImageURL != "" {
		t.Fatalf("got image URL %q, want external preview images disabled", preview.ImageURL)
	}
}

func TestNormalizeLinkPreviewURLRejectsCredentials(t *testing.T) {
	if _, _, err := normalizeLinkPreviewURL("https://user:password@example.com/path"); !errors.Is(err, ErrInvalidLinkPreviewURL) {
		t.Fatalf("normalize error = %v, want ErrInvalidLinkPreviewURL", err)
	}
}

func TestSafePreviewPortRejectsNonWebPorts(t *testing.T) {
	for _, rawURL := range []string{"http://example.com:8080/path", "https://example.com:8443/path"} {
		_, parsed, err := normalizeLinkPreviewURL(rawURL)
		if err != nil {
			t.Fatal(err)
		}
		if err := ensureSafePreviewPort(parsed); !errors.Is(err, ErrBlockedLinkPreviewURL) {
			t.Fatalf("port check for %q = %v, want ErrBlockedLinkPreviewURL", rawURL, err)
		}
	}
}

func TestLinkPreviewRedirectRevalidatesDestination(t *testing.T) {
	privateReached := false
	service := NewLinkPreviewService(zerolog.Nop())
	redirectURL := "https://example.com/start"
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Hostname() == "127.0.0.1" {
				privateReached = true
			}
			if req.URL.String() == redirectURL {
				return &http.Response{
					StatusCode: http.StatusFound,
					Header:     http.Header{"Location": []string{"http://127.0.0.1/secret"}},
					Body:       http.NoBody,
					Request:    req,
				}, nil
			}
			return http.DefaultTransport.RoundTrip(req)
		}),
		CheckRedirect: service.newHTTPClient().CheckRedirect,
	}

	_, err := service.Fetch(context.Background(), redirectURL)
	if !errors.Is(err, ErrBlockedLinkPreviewURL) {
		t.Fatalf("fetch error = %v, want ErrBlockedLinkPreviewURL", err)
	}
	if privateReached {
		t.Fatal("private redirect destination received a request")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestLinkPreviewServiceBlocksPrivateHostsByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	service := NewLinkPreviewService(zerolog.Nop())
	_, err := service.Fetch(context.Background(), srv.URL)
	if !errors.Is(err, ErrBlockedLinkPreviewURL) {
		t.Fatalf("got err %v, want ErrBlockedLinkPreviewURL", err)
	}
}

func TestLinkPreviewServiceEvictsLeastRecentlyUsedEntries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<!doctype html><html><head><meta property="og:title" content="%s"></head></html>`, r.URL.Path)
	}))
	defer srv.Close()

	service := NewLinkPreviewService(zerolog.Nop())
	service.allowPrivateHosts = true
	service.maxEntries = 2
	service.ttl = time.Hour

	urlA := srv.URL + "/a"
	urlB := srv.URL + "/b"
	urlC := srv.URL + "/c"

	if _, err := service.Fetch(context.Background(), urlA); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Fetch(context.Background(), urlB); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Fetch(context.Background(), urlA); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Fetch(context.Background(), urlC); err != nil {
		t.Fatal(err)
	}

	if len(service.cache) != 2 {
		t.Fatalf("cache size = %d, want 2", len(service.cache))
	}
	if _, ok := service.cache[urlA]; !ok {
		t.Fatalf("expected %s to remain cached", urlA)
	}
	if _, ok := service.cache[urlC]; !ok {
		t.Fatalf("expected %s to remain cached", urlC)
	}
	if _, ok := service.cache[urlB]; ok {
		t.Fatalf("expected %s to be evicted", urlB)
	}
}
