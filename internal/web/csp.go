package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

var embeddedScriptCSPSource = mustEmbeddedScriptCSPSource()

// mustEmbeddedScriptCSPSource pins the one developer-authored inline script.
// Inline event handlers remain blocked because script-src has no unsafe-inline.
func mustEmbeddedScriptCSPSource() string {
	html, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		panic(fmt.Sprintf("read embedded UI for CSP: %v", err))
	}
	const openTag = "<script>"
	const closeTag = "</script>"
	start := bytes.Index(html, []byte(openTag))
	end := bytes.LastIndex(html, []byte(closeTag))
	if start < 0 || end <= start || bytes.Count(html, []byte(openTag)) != 1 {
		panic("embedded UI must contain exactly one inline script for CSP hashing")
	}
	script := html[start+len(openTag) : end]
	sum := sha256.Sum256(script)
	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}

func contentSecurityPolicy() string {
	return "default-src 'self'; " +
		"script-src " + embeddedScriptCSPSource + "; " +
		"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
		"font-src https://fonts.gstatic.com; " +
		"img-src 'self' data: blob: https:; " +
		"media-src 'self' data: blob:; " +
		"connect-src 'self'; object-src 'none'; base-uri 'none'; " +
		"frame-ancestors 'none'; form-action 'none'"
}
