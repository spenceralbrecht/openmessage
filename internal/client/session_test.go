package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadSession(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")

	// Save
	data := &SessionData{
		AuthDataJSON: []byte(`{"session_id":"test-123"}`),
		PushKeysJSON: []byte(`{"url":"https://example.com"}`),
	}
	err := SaveSession(path, data)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}

	// Load
	loaded, err := LoadSession(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// Compare as parsed JSON (whitespace-insensitive)
	var authMap map[string]any
	if err := json.Unmarshal(loaded.AuthDataJSON, &authMap); err != nil {
		t.Fatalf("parse auth data: %v", err)
	}
	if authMap["session_id"] != "test-123" {
		t.Errorf("auth data session_id mismatch: %v", authMap)
	}

	var pushMap map[string]any
	if err := json.Unmarshal(loaded.PushKeysJSON, &pushMap); err != nil {
		t.Fatalf("parse push keys: %v", err)
	}
	if pushMap["url"] != "https://example.com" {
		t.Errorf("push keys url mismatch: %v", pushMap)
	}
}

func TestSaveSessionTightensExistingPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "session-dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(): %v", err)
	}
	path := filepath.Join(dir, "session.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	if err := SaveSession(path, &SessionData{AuthDataJSON: json.RawMessage(`{}`)}); err != nil {
		t.Fatalf("SaveSession(): %v", err)
	}

	if info, err := os.Stat(dir); err != nil {
		t.Fatalf("Stat(dir): %v", err)
	} else if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("dir mode = %04o, want 0700", got)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatalf("Stat(session): %v", err)
	} else if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("session mode = %04o, want 0600", got)
	}
}

func TestSaveSessionReplacesSymlinkWithoutFollowingIt(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "unrelated")
	if err := os.WriteFile(target, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	sessionPath := filepath.Join(dir, "session.json")
	if err := os.Symlink(target, sessionPath); err != nil {
		t.Fatal(err)
	}

	if err := SaveSession(sessionPath, &SessionData{AuthDataJSON: json.RawMessage(`{}`)}); err != nil {
		t.Fatalf("SaveSession(): %v", err)
	}
	if content, err := os.ReadFile(target); err != nil {
		t.Fatal(err)
	} else if string(content) != "preserve" {
		t.Fatalf("symlink target changed to %q", content)
	}
	if info, err := os.Lstat(sessionPath); err != nil {
		t.Fatal(err)
	} else if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		t.Fatalf("session path mode = %s, want regular file", info.Mode())
	}
}

func TestLoadSessionNotFound(t *testing.T) {
	_, err := LoadSession("/nonexistent/path/session.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestSaveSessionCreatesDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "session.json")

	data := &SessionData{
		AuthDataJSON: []byte(`{}`),
	}
	err := SaveSession(path, data)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}
