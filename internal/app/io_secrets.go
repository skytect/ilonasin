package app

import (
	"context"

	"ilonasin/internal/logging"
	"ilonasin/internal/storage/sqlite"
)

func refreshIOConfiguredSecrets(ctx context.Context, ioLogger *logging.IOLogger, store *sqlite.Store) error {
	if ioLogger == nil || store == nil {
		return nil
	}
	secrets, err := store.ListCredentialSecretMaterial(ctx)
	if err != nil {
		return err
	}
	ioLogger.ReplaceConfiguredSecrets(secrets)
	return nil
}

func ioSecretRefreshHook(ctx context.Context, ioLogger *logging.IOLogger, store *sqlite.Store) func(context.Context, ...string) {
	if ioLogger == nil || store == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return func(_ context.Context, secrets ...string) {
		for _, secret := range secrets {
			ioLogger.AddEphemeralSecret(secret)
		}
		_ = refreshIOConfiguredSecrets(ctx, ioLogger, store)
	}
}
