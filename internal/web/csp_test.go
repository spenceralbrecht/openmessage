package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestEmbeddedUIHasNoInlineEventHandlers(t *testing.T) {
	content, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("ReadFile(): %v", err)
	}
	tokenizer := html.NewTokenizer(bytes.NewReader(content))
	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			if err := tokenizer.Err(); err != nil && err != io.EOF {
				t.Fatalf("tokenize embedded UI: %v", err)
			}
			return
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			for _, attr := range token.Attr {
				if strings.HasPrefix(strings.ToLower(attr.Key), "on") {
					t.Fatalf("inline event handler %q remains on <%s>", attr.Key, token.Data)
				}
			}
		}
	}
}

func TestEmbeddedScriptCSPSourceMatchesUI(t *testing.T) {
	content, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("ReadFile(): %v", err)
	}
	start := bytes.Index(content, []byte("<script>"))
	end := bytes.LastIndex(content, []byte("</script>"))
	if start < 0 || end <= start {
		t.Fatal("inline script not found")
	}
	sum := sha256.Sum256(content[start+len("<script>") : end])
	want := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	if embeddedScriptCSPSource != want {
		t.Fatalf("CSP source = %q, want %q", embeddedScriptCSPSource, want)
	}
	policy := contentSecurityPolicy()
	if strings.Contains(policy, "script-src 'unsafe-inline'") || strings.Contains(policy, "script-src 'unsafe-eval'") {
		t.Fatalf("unsafe script CSP: %s", policy)
	}
}

func TestEmbeddedUINeverInterpolatesMessageIDIntoEmojiHTML(t *testing.T) {
	content, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("ReadFile(): %v", err)
	}
	if bytes.Contains(content, []byte(`emoji-grid-${messageId}`)) {
		t.Fatal("untrusted message ID is interpolated into hydrated emoji HTML")
	}
}
