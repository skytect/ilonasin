package management

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	SocketFileNamePrefix = "manage-"
	SocketFileNameSuffix = ".sock"
)

type SocketOwner struct {
	path string
	info os.FileInfo
}

func SocketPath(homeDir, configPath, databasePath string) string {
	configPath = resolveIdentityPath(configPath)
	databasePath = resolveIdentityPath(databasePath)
	sum := sha256.Sum256([]byte(configPath + "\x00" + databasePath))
	return filepath.Join(homeDir, "run", SocketFileNamePrefix+hex.EncodeToString(sum[:8])+SocketFileNameSuffix)
}

func PrepareUnixListener(ctx context.Context, socketPath string) (net.Listener, SocketOwner, error) {
	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, SocketOwner{}, err
	}
	if err := secureDir(dir); err != nil {
		return nil, SocketOwner{}, err
	}
	if st, err := os.Lstat(socketPath); err == nil {
		if st.Mode()&os.ModeSocket == 0 {
			return nil, SocketOwner{}, fmt.Errorf("management socket path is not a socket")
		}
		if socketAccepts(ctx, socketPath) {
			return nil, SocketOwner{}, fmt.Errorf("management daemon is already running")
		}
		current, err := os.Lstat(socketPath)
		if err != nil {
			return nil, SocketOwner{}, err
		}
		if current.Mode()&os.ModeSocket == 0 || !os.SameFile(current, st) {
			return nil, SocketOwner{}, fmt.Errorf("management socket changed while preparing listener")
		}
		if err := os.Remove(socketPath); err != nil {
			return nil, SocketOwner{}, err
		}
	} else if !os.IsNotExist(err) {
		return nil, SocketOwner{}, err
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, SocketOwner{}, err
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		_ = listener.Close()
		return nil, SocketOwner{}, err
	}
	info, err := os.Lstat(socketPath)
	if err != nil {
		_ = listener.Close()
		return nil, SocketOwner{}, err
	}
	return listener, SocketOwner{path: socketPath, info: info}, nil
}

func CleanupSocket(owner SocketOwner) {
	if owner.path == "" || owner.info == nil {
		return
	}
	st, err := os.Lstat(owner.path)
	if err != nil || st.Mode()&os.ModeSocket == 0 {
		return
	}
	if !os.SameFile(st, owner.info) {
		return
	}
	_ = os.Remove(owner.path)
}

func socketAccepts(ctx context.Context, socketPath string) bool {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func HTTPClient(socketPath string) *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socketPath)
			},
		},
	}
}

func resolveIdentityPath(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	eval, err := filepath.EvalSymlinks(path)
	if err == nil {
		path = eval
	}
	return filepath.Clean(path)
}

func secureDir(path string) error {
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("management runtime path must not be a symlink")
	}
	if !st.IsDir() {
		return fmt.Errorf("management runtime path is not a directory")
	}
	return os.Chmod(path, 0o700)
}
