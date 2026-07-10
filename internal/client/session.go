package client

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type SessionData struct {
	AuthDataJSON json.RawMessage `json:"auth_data"`
	PushKeysJSON json.RawMessage `json:"push_keys,omitempty"`
}

func SaveSession(path string, data *SessionData) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("inspect dir: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("secure dir: must be a regular directory")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("secure dir: %w", err)
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".session-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary session: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return fmt.Errorf("secure temporary session: %w", err)
	}
	if _, err := temp.Write(b); err != nil {
		temp.Close()
		return fmt.Errorf("write temporary session: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync temporary session: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary session: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace session: %w", err)
	}
	return nil
}

func LoadSession(path string) (*SessionData, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	var data SessionData
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return &data, nil
}
