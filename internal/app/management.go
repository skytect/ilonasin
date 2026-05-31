package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/management"
	"ilonasin/internal/storage/sqlite"
)

type managementRuntime struct {
	socketPath string
	owner      management.SocketOwner
	server     *http.Server
}

func startManagementServer(ctx context.Context, homeDir, configPath, databasePath string, store *sqlite.Store) (managementRuntime, error) {
	socketPath := management.SocketPath(homeDir, configPath, databasePath)
	listener, owner, err := management.PrepareUnixListener(ctx, socketPath)
	if err != nil {
		return managementRuntime{}, err
	}
	srv := &http.Server{
		Handler:           management.Handler(management.Service{Tokens: credentials.Service{Repo: store}}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		errc <- srv.Serve(listener)
	}()
	select {
	case err := <-errc:
		management.CleanupSocket(owner)
		if err == nil || err == http.ErrServerClosed {
			return managementRuntime{}, fmt.Errorf("management server stopped")
		}
		return managementRuntime{}, err
	default:
	}
	return managementRuntime{socketPath: socketPath, owner: owner, server: srv}, nil
}

func (m managementRuntime) Close(ctx context.Context) {
	if m.server != nil {
		_ = m.server.Shutdown(ctx)
	}
	management.CleanupSocket(m.owner)
}
