package app

import (
	"context"

	"ilonasin/internal/logging"
)

type credentialSecretSource interface {
	ListCredentialSecretMaterial(context.Context) ([]string, error)
}

func refreshIOConfiguredSecrets(ctx context.Context, ioLogger *logging.IOLogger, source credentialSecretSource) error {
	if ioLogger == nil || source == nil {
		return nil
	}
	values, err := source.ListCredentialSecretMaterial(ctx)
	if err != nil {
		return err
	}
	ioLogger.ReplaceConfiguredSecrets(values)
	return nil
}

func ioSecretRefreshHook(ctx context.Context, ioLogger *logging.IOLogger, source credentialSecretSource) func(context.Context, ...string) {
	if ioLogger == nil || source == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return func(_ context.Context, secrets ...string) {
		for _, secret := range secrets {
			ioLogger.AddEphemeralSecret(secret)
		}
		_ = refreshIOConfiguredSecrets(ctx, ioLogger, source)
	}
}
