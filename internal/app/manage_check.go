package app

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"ilonasin/internal/management"
	"ilonasin/internal/tui"
)

func ManageCheck(opts Options) error {
	rt, err := bootstrap(context.Background(), opts, true)
	if err != nil {
		return err
	}
	defer rt.cleanup()
	defer rt.Store.Close()
	rt.Logger.InfoContext(context.Background(), "manage check starting", "event", "app_command_start", "command", "manage_check")
	beforeSnapshot, err := selectedHomeSnapshot(context.Background(), rt.Store, rt.ConfigPath)
	if err != nil {
		return err
	}
	mgmt, err := startManagementServer(context.Background(), rt.HomeDir, rt.ConfigPath, rt.Config.Paths.Database, rt.Registry, rt.Store, rt.Logger)
	if err != nil {
		return err
	}
	defer mgmt.Close(context.Background())
	if err := exerciseLocalTokenCheck(context.Background(), rt.HomeDir, rt.ConfigPath); err != nil {
		return err
	}
	if err := exerciseManagementSnapshotTUIReload(context.Background()); err != nil {
		return err
	}
	if err := exerciseManagementSnapshotSanitization(context.Background()); err != nil {
		return err
	}
	if err := exerciseManagementSnapshotHTTPRoute(context.Background(), rt.HomeDir, rt.ConfigPath); err != nil {
		return err
	}
	if err := assertProductionUpstreamMutationWiring(); err != nil {
		return err
	}
	if err := exerciseUpstreamCredentialCheck(context.Background(), rt.Registry, rt.Config); err != nil {
		return err
	}
	if err := exerciseFallbackPolicyCheck(context.Background(), rt.Registry, rt.Config); err != nil {
		return err
	}
	if err := exerciseModelCacheCheck(context.Background(), rt.Registry, rt.Config); err != nil {
		return err
	}
	if err := exerciseObservabilityCheck(context.Background(), rt.Registry, rt.Config); err != nil {
		return err
	}
	if err := exerciseOAuthCheck(context.Background(), rt.Registry, rt.Config); err != nil {
		return err
	}
	if err := exerciseOAuthDeviceLoginCheck(context.Background(), rt.Config, rt.Logger); err != nil {
		return err
	}
	if err := exerciseOAuthRefreshCheck(context.Background(), rt.Config); err != nil {
		return err
	}
	if err := exerciseTelemetryPruneCheck(context.Background(), rt.Registry, rt.Config); err != nil {
		return err
	}
	var buf bytes.Buffer
	tokenClient := management.NewUnixLocalTokenClient(management.SocketPath(rt.HomeDir, rt.ConfigPath, rt.Config.Paths.Database))
	if err := tui.Check(rt.Config, rt.Registry, tokenClient, tokenClient, tokenClient, tokenClient, nil, nil, rt.Store, &buf, rt.Logger); err != nil {
		return err
	}
	afterSnapshot, err := selectedHomeSnapshot(context.Background(), rt.Store, rt.ConfigPath)
	if err != nil {
		return err
	}
	if afterSnapshot != beforeSnapshot {
		return fmt.Errorf("manage check mutated selected home metadata")
	}
	if opts.Stdout != nil {
		_, _ = opts.Stdout.Write(buf.Bytes())
	}
	return nil
}

func assertProductionUpstreamMutationWiring() error {
	root, err := sourceRoot()
	if err != nil {
		return err
	}
	if err := assertTUIUpstreamArg(filepath.Join(root, "internal/app/commands.go"), "Run"); err != nil {
		return err
	}
	if err := assertTUIUpstreamArg(filepath.Join(root, "internal/app/manage_check.go"), "Check"); err != nil {
		return err
	}
	checks := []struct {
		path      string
		forbidden string
	}{
		{path: "internal/tui/tui.go", forbidden: "func Run(cfg config.Config, registry provider.Registry, snapshot management.SnapshotClient, tokens management.LocalTokenClient, upstreams " + "credentials.UpstreamCredentialManager"},
		{path: "internal/tui/tui.go", forbidden: "func Check(cfg config.Config, registry provider.Registry, snapshot management.SnapshotClient, tokens management.LocalTokenClient, upstreams " + "credentials.UpstreamCredentialManager"},
		{path: "internal/tui/tui.go", forbidden: "upstreams        " + "credentials.UpstreamCredentialManager"},
		{path: "internal/tui/tui.go", forbidden: "oauthRefresh     " + "credentials.OAuthRefreshController"},
		{path: "internal/tui/tui.go", forbidden: "oauthLogin       " + "credentials.OAuthDeviceLoginController"},
		{path: "internal/tui/tui.go", forbidden: "func Run(cfg config.Config, registry provider.Registry, snapshot management.SnapshotClient, tokens management.LocalTokenClient, upstreams management.UpstreamCredentialClient, oauth " + "credentials.OAuthMetadataReader"},
		{path: "internal/tui/tui.go", forbidden: "func Check(cfg config.Config, registry provider.Registry, snapshot management.SnapshotClient, tokens management.LocalTokenClient, upstreams management.UpstreamCredentialClient, oauth " + "credentials.OAuthMetadataReader"},
	}
	for _, check := range checks {
		body, err := os.ReadFile(filepath.Join(root, check.path))
		if err != nil {
			return err
		}
		if strings.Contains(string(body), check.forbidden) {
			return fmt.Errorf("production upstream mutation wiring retained legacy dependency in %s", check.path)
		}
	}
	return nil
}

func sourceRoot() (string, error) {
	_, file, _, ok := goruntime.Caller(0)
	if !ok {
		return "", fmt.Errorf("source root unavailable")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..")), nil
}

func assertTUIUpstreamArg(path, name string) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return err
	}
	var found bool
	var invalid bool
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != name {
			return true
		}
		pkg, ok := selector.X.(*ast.Ident)
		if !ok || pkg.Name != "tui" {
			return true
		}
		found = true
		if len(call.Args) < 6 || !identName(call.Args[4], "tokenClient") || !identName(call.Args[5], "tokenClient") {
			invalid = true
		}
		return true
	})
	if !found {
		return fmt.Errorf("production tui.%s call missing in %s", name, path)
	}
	if invalid {
		return fmt.Errorf("production tui.%s mutation arguments are not the management client", name)
	}
	return nil
}

func identName(expr ast.Expr, name string) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == name
}
