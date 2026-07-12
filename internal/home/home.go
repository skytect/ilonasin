package home

import (
	"os"
	"path/filepath"
	"strings"
)

const EnvName = "ILONASIN_HOME"

func Resolve(envHome string) (string, error) {
	if envHome != "" {
		return canonicalPath(expand(envHome))
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return canonicalPath(filepath.Join(userHome, ".ilonasin"))
}

func Ensure(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700)
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

func SecureFile(path string) error {
	return os.Chmod(path, 0o600)
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

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	eval, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return eval, nil
	}
	if os.IsNotExist(err) {
		return filepath.Clean(abs), nil
	}
	return "", err
}
