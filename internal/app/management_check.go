package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"ilonasin/internal/management"
)

func exerciseManagementRouteIsolation(ctx context.Context, client management.HTTPTokenClient) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.BaseURL+"/v1/models", nil)
	if err != nil {
		return err
	}
	resp, err := client.Client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("management socket served public models status=%d", resp.StatusCode)
	}
	return nil
}

func assertManagementSocketDirMode(socketPath string) error {
	info, err := os.Stat(filepath.Dir(socketPath))
	if err != nil {
		return err
	}
	if info.Mode().Perm() != 0o700 {
		return fmt.Errorf("management socket dir mode=%#o", info.Mode().Perm())
	}
	return nil
}
