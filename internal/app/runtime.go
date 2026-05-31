package app

import (
	"context"
	"log/slog"
	"path/filepath"

	"ilonasin/internal/storage/sqlite"
)

type runtime struct {
	*coreRuntime
	Store *sqlite.Store
}

func bootstrap(ctx context.Context, opts Options, checkSafeHome bool) (*runtime, error) {
	core, err := bootstrapCore(ctx, opts, checkSafeHome)
	if err != nil {
		return nil, err
	}
	store, err := sqlite.Open(ctx, filepath.Clean(core.Config.Paths.Database))
	if err != nil {
		core.cleanup()
		return nil, err
	}
	store.Logger = core.Logger
	core.Logger.InfoContext(ctx, "storage open complete",
		slog.String("event", "storage_open"),
		slog.String("database", "configured"),
	)
	return &runtime{coreRuntime: core, Store: store}, nil
}
