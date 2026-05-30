package home

import (
	"os"
	"path/filepath"
	"strings"
)

const EnvName = "ILONASIN_HOME"

func Resolve(envHome string) (string, error) {
	if envHome != "" {
		return filepath.Abs(expand(envHome))
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(userHome, ".ilonasin"), nil
}

func Ensure(dir string) error {
	return os.MkdirAll(dir, 0o700)
}

func ExpandPath(path, homeDir string) string {
	switch {
	case path == "":
		return ""
	case path == "~":
		userHome, err := os.UserHomeDir()
		if err != nil {
			return homeDir
		}
		return userHome
	case strings.HasPrefix(path, "~/"):
		userHome, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(homeDir, strings.TrimPrefix(path, "~/"))
		}
		return filepath.Join(userHome, strings.TrimPrefix(path, "~/"))
	default:
		return path
	}
}

func SecureFile(path string) {
	_ = os.Chmod(path, 0o600)
}

func expand(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		userHome, err := os.UserHomeDir()
		if err == nil {
			if path == "~" {
				return userHome
			}
			return filepath.Join(userHome, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}
