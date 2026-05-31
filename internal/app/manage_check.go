package app

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"ilonasin/internal/home"
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
	if err := exerciseManageDaemonUnavailableCheck(); err != nil {
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
	if err := tui.Check(rt.Config, rt.Registry, tokenClient, tokenClient, tokenClient, tokenClient, tokenClient, &buf, rt.Logger); err != nil {
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
	if err := assertProductionManageClientBootstrap(root); err != nil {
		return err
	}
	if err := assertTUIUpstreamArg(filepath.Join(root, "internal/app/commands.go"), "Run"); err != nil {
		return err
	}
	if err := assertTUIUpstreamArg(filepath.Join(root, "internal/app/manage_check.go"), "Check"); err != nil {
		return err
	}
	if err := assertTUIViewRowTypes(filepath.Join(root, "internal/tui/tui.go")); err != nil {
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
		{path: "internal/tui/tui.go", forbidden: "UpstreamMetadataReader"},
		{path: "internal/tui/tui.go", forbidden: "OAuthMetadataReader"},
		{path: "internal/tui/tui.go", forbidden: "upstreamReader"},
		{path: "internal/tui/tui.go", forbidden: "oauthReader"},
		{path: "internal/tui/tui.go", forbidden: "reloadDirect"},
		{path: "internal/tui/tui.go", forbidden: "ListLocalTokens(context.Background())"},
		{path: "internal/tui/tui.go", forbidden: "ListOAuthCredentials(context.Background())"},
		{path: "internal/tui/tui.go", forbidden: "ListProviderAccounts(context.Background())"},
		{path: "internal/tui/tui.go", forbidden: "ListFallbackPolicies(context.Background())"},
		{path: "internal/tui/tui.go", forbidden: ".List(context.Background())"},
		{path: "internal/tui/tui.go", forbidden: "UpstreamCredentialResolver"},
		{path: "internal/tui/tui.go", forbidden: "directUpstreamMutations"},
		{path: "internal/tui/tui.go", forbidden: "directOAuthMutations"},
		{path: "internal/tui/tui.go", forbidden: "failingOAuthRefreshController"},
		{path: "internal/tui/tui.go", forbidden: "func Run(cfg config.Config, registry provider.Registry, snapshot management.SnapshotClient, tokens management.LocalTokenClient, upstreams management.UpstreamCredentialClient, oauth " + "credentials.OAuthMetadataReader"},
		{path: "internal/tui/tui.go", forbidden: "func Check(cfg config.Config, registry provider.Registry, snapshot management.SnapshotClient, tokens management.LocalTokenClient, upstreams management.UpstreamCredentialClient, oauth " + "credentials.OAuthMetadataReader"},
		{path: "internal/tui/tui.go", forbidden: "func Run(cfg config.Config, registry provider.Registry, snapshot management.SnapshotClient, tokens management.LocalTokenClient, upstreams management.UpstreamCredentialClient, oauth management.OAuthClient, modelCache ModelCacheReader, observability ObservabilityReader, pruner " + "TelemetryPruner"},
		{path: "internal/tui/tui.go", forbidden: "func Check(cfg config.Config, registry provider.Registry, snapshot management.SnapshotClient, tokens management.LocalTokenClient, upstreams management.UpstreamCredentialClient, oauth management.OAuthClient, modelCache ModelCacheReader, observability ObservabilityReader, pruner " + "TelemetryPruner"},
		{path: "internal/tui/tui.go", forbidden: "ModelCacheReader"},
		{path: "internal/tui/tui.go", forbidden: "ObservabilityReader"},
		{path: "internal/tui/tui.go", forbidden: "modelCache       "},
		{path: "internal/tui/tui.go", forbidden: "observability    "},
		{path: "internal/tui/tui.go", forbidden: "m.modelCache"},
		{path: "internal/tui/tui.go", forbidden: "m.observability"},
		{path: "internal/tui/tui.go", forbidden: "ListModelCache(context.Background())"},
		{path: "internal/tui/tui.go", forbidden: "RecentRequests(context.Background()"},
		{path: "internal/tui/tui.go", forbidden: "UsageByProvider(context.Background())"},
		{path: "internal/tui/tui.go", forbidden: "LatencyByProvider(context.Background())"},
		{path: "internal/tui/tui.go", forbidden: "StreamSummary(context.Background())"},
		{path: "internal/tui/tui.go", forbidden: "LatestHealth(context.Background())"},
		{path: "internal/tui/tui.go", forbidden: "RecentFallbacks(context.Background()"},
		{path: "internal/tui/tui.go", forbidden: "QuotaByProvider(context.Background())"},
		{path: "internal/tui/tui.go", forbidden: "credentials.UpstreamCredentialMetadata"},
		{path: "internal/tui/tui.go", forbidden: "credentials.FallbackPolicyMetadata"},
		{path: "internal/tui/tui.go", forbidden: "credentials.OAuthCredentialMetadata"},
		{path: "internal/tui/tui.go", forbidden: "credentials.ProviderAccountMetadata"},
		{path: "internal/tui/tui.go", forbidden: "upstreamCredentialsFromSnapshot"},
		{path: "internal/tui/tui.go", forbidden: "fallbackPoliciesFromSnapshot"},
		{path: "internal/tui/tui.go", forbidden: "oauthCredentialsFromSnapshot"},
		{path: "internal/tui/tui.go", forbidden: "providerAccountsFromSnapshot"},
		{path: "internal/tui/tui.go", forbidden: "\"ilonasin/internal/metadata\""},
		{path: "internal/tui/tui.go", forbidden: "provider.ModelMetadata"},
		{path: "internal/tui/tui.go", forbidden: "metadata.RequestSummary"},
		{path: "internal/tui/tui.go", forbidden: "metadata.UsageSummary"},
		{path: "internal/tui/tui.go", forbidden: "metadata.LatencySummary"},
		{path: "internal/tui/tui.go", forbidden: "metadata.StreamSummary"},
		{path: "internal/tui/tui.go", forbidden: "metadata.HealthSummary"},
		{path: "internal/tui/tui.go", forbidden: "metadata.FallbackSummary"},
		{path: "internal/tui/tui.go", forbidden: "metadata.QuotaSummary"},
		{path: "internal/tui/tui.go", forbidden: "metadata.PruneResult"},
		{path: "internal/tui/tui.go", forbidden: "modelMetadataFromSnapshot"},
		{path: "internal/tui/tui.go", forbidden: "requestSummariesFromSnapshot"},
		{path: "internal/tui/tui.go", forbidden: "usageSummariesFromSnapshot"},
		{path: "internal/tui/tui.go", forbidden: "latencySummariesFromSnapshot"},
		{path: "internal/tui/tui.go", forbidden: "streamSummariesFromSnapshot"},
		{path: "internal/tui/tui.go", forbidden: "healthSummariesFromSnapshot"},
		{path: "internal/tui/tui.go", forbidden: "fallbackSummariesFromSnapshot"},
		{path: "internal/tui/tui.go", forbidden: "quotaSummariesFromSnapshot"},
		{path: "internal/tui/tui.go", forbidden: "type TelemetryPruner interface"},
		{path: "internal/tui/tui.go", forbidden: "directTelemetryPruner"},
		{path: "internal/tui/tui.go", forbidden: "PruneTelemetryBefore"},
		{path: "internal/app/manage_check_exercises.go", forbidden: "tui.ExerciseModelCacheSummary(ctx, cfg, registry, store)"},
		{path: "internal/app/manage_check_exercises.go", forbidden: "tui.ExerciseObservabilitySummary(ctx, cfg, registry, store)"},
		{path: "internal/app/manage_check_exercises.go", forbidden: "tui.ExerciseTelemetryPrune(ctx, cfg, registry, store,"},
		{path: "internal/app/manage_check_exercises.go", forbidden: "tui.ExerciseTokenLifecycle(ctx, client)"},
		{path: "internal/app/manage_check_exercises.go", forbidden: "tui.ExerciseUpstreamCredentialLifecycle(ctx, cfg, registry, client, service)"},
		{path: "internal/app/manage_check_exercises.go", forbidden: "tui.ExerciseFallbackPolicyLifecycle(ctx, cfg, registry, client, service"},
		{path: "internal/app/manage_check_exercises.go", forbidden: "tui.ExerciseFallbackPolicyLifecycle(ctx, cfg, registry, client, client, service"},
		{path: "internal/app/manage_check_exercises.go", forbidden: "tui.ExerciseOAuthFallbackPolicySummary(ctx, cfg, registry, service)"},
		{path: "internal/app/manage_check_exercises.go", forbidden: "tui.ExerciseOAuthSummary(ctx, cfg, registry, service)"},
		{path: "internal/app/manage_check_exercises.go", forbidden: "tui.ExerciseOAuthRefresh(ctx, cfg, registry, service"},
		{path: "internal/app/manage_check_exercises.go", forbidden: "tui.ExerciseOAuthDeviceLogin(ctx, loginCfg, registry, service"},
		{path: "internal/app/manage_check_exercises.go", forbidden: "tui.ExerciseOAuthDeviceLoginFailure(ctx, cfg, service.Registry, service"},
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

func assertTUIViewRowTypes(path string) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return err
	}
	if err := assertNoProviderModelMetadata(file); err != nil {
		return err
	}
	wantArrays := map[string]string{
		"credentials":      "UpstreamCredential",
		"fallbackPolicies": "FallbackPolicy",
		"oauthRows":        "OAuthCredential",
		"accountRows":      "ProviderAccount",
		"modelRows":        "ModelMetadata",
		"requestRows":      "RequestSummary",
		"usageRows":        "UsageSummary",
		"latencyRows":      "LatencySummary",
		"streamRows":       "StreamSummary",
		"healthRows":       "HealthSummary",
		"fallbackRows":     "FallbackSummary",
		"quotaRows":        "QuotaSummary",
	}
	wantPointers := map[string]string{
		"pruneResult": "PruneResult",
	}
	seen := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		decl, ok := n.(*ast.TypeSpec)
		if !ok || decl.Name.Name != "Model" {
			return true
		}
		st, ok := decl.Type.(*ast.StructType)
		if !ok {
			return false
		}
		for _, field := range st.Fields.List {
			for _, name := range field.Names {
				if wantType, ok := wantArrays[name.Name]; ok {
					if arraySelectorType(field.Type, "management", wantType) {
						seen[name.Name] = true
					}
					continue
				}
				if wantType, ok := wantPointers[name.Name]; ok {
					if pointerSelectorType(field.Type, "management", wantType) {
						seen[name.Name] = true
					}
				}
			}
		}
		return false
	})
	for field, wantType := range wantArrays {
		if !seen[field] {
			return fmt.Errorf("tui Model.%s is not []management.%s", field, wantType)
		}
	}
	for field, wantType := range wantPointers {
		if !seen[field] {
			return fmt.Errorf("tui Model.%s is not *management.%s", field, wantType)
		}
	}
	return nil
}

func assertNoProviderModelMetadata(file *ast.File) error {
	providerNames := map[string]bool{}
	for _, imp := range file.Imports {
		if strings.Trim(imp.Path.Value, "\"") != "ilonasin/internal/provider" {
			continue
		}
		if imp.Name == nil {
			providerNames["provider"] = true
			continue
		}
		switch imp.Name.Name {
		case ".", "_":
			return fmt.Errorf("tui provider import must not use alias %q", imp.Name.Name)
		default:
			providerNames[imp.Name.Name] = true
		}
	}
	var found bool
	ast.Inspect(file, func(n ast.Node) bool {
		selector, ok := n.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "ModelMetadata" {
			return true
		}
		ident, ok := selector.X.(*ast.Ident)
		if ok && providerNames[ident.Name] {
			found = true
			return false
		}
		return true
	})
	if found {
		return fmt.Errorf("tui must not use provider.ModelMetadata")
	}
	return nil
}

func arraySelectorType(expr ast.Expr, pkg, typ string) bool {
	array, ok := expr.(*ast.ArrayType)
	if !ok {
		return false
	}
	selector, ok := array.Elt.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != typ {
		return false
	}
	ident, ok := selector.X.(*ast.Ident)
	return ok && ident.Name == pkg
}

func pointerSelectorType(expr ast.Expr, pkg, typ string) bool {
	pointer, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := pointer.X.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != typ {
		return false
	}
	ident, ok := selector.X.(*ast.Ident)
	return ok && ident.Name == pkg
}

func exerciseManageDaemonUnavailableCheck() error {
	tmp, err := os.MkdirTemp("", "ilonasin-manage-daemon-unavailable-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	previous, hadPrevious := os.LookupEnv(home.EnvName)
	if err := os.Setenv(home.EnvName, tmp); err != nil {
		return err
	}
	defer func() {
		if hadPrevious {
			_ = os.Setenv(home.EnvName, previous)
		} else {
			_ = os.Unsetenv(home.EnvName)
		}
	}()
	err = Manage(Options{Stdout: io.Discard, Stderr: io.Discard})
	if err == nil {
		return fmt.Errorf("manage without daemon succeeded")
	}
	if !strings.Contains(err.Error(), "management daemon unavailable") {
		return fmt.Errorf("manage without daemon failed with unexpected error: %s", err)
	}
	if _, statErr := os.Stat(filepath.Join(tmp, "ilonasin.sqlite")); !os.IsNotExist(statErr) {
		if statErr != nil {
			return statErr
		}
		return fmt.Errorf("manage without daemon created sqlite database")
	}
	return nil
}

func assertProductionManageClientBootstrap(root string) error {
	commandsPath := filepath.Join(root, "internal/app/commands.go")
	manageBody, err := functionBodySource(commandsPath, "Manage")
	if err != nil {
		return err
	}
	compactManage := compactSource(manageBody)
	forbidden := []string{
		"bootstrap(",
		"sqlite.Open",
		".Store",
		"credentials.Service",
		"credentials.UpstreamService",
	}
	for _, value := range forbidden {
		if strings.Contains(manageBody, value) {
			return fmt.Errorf("production Manage retained storage dependency %q", value)
		}
	}
	if !strings.Contains(manageBody, "bootstrapClient(") {
		return fmt.Errorf("production Manage does not use client bootstrap")
	}
	if !strings.Contains(manageBody, "defer rt.cleanup()") {
		return fmt.Errorf("production Manage does not cleanup client runtime")
	}
	if !strings.Contains(compactManage, "management.SocketPath(rt.HomeDir,rt.ConfigPath,rt.Config.Paths.Database)") {
		return fmt.Errorf("production Manage does not use resolved client runtime paths for management socket")
	}

	corePath := filepath.Join(root, "internal/app/runtime_core.go")
	coreBody, err := os.ReadFile(corePath)
	if err != nil {
		return err
	}
	core := string(coreBody)
	for _, value := range []string{
		"internal/storage/sqlite",
		"sqlite.Open",
		"Store",
		"bootstrap(",
		"credentials.Service",
		"credentials.UpstreamService",
	} {
		if strings.Contains(core, value) {
			return fmt.Errorf("client runtime retained storage dependency %q", value)
		}
	}
	return nil
}

func functionBodySource(path, name string) (string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return "", err
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != name || fn.Body == nil {
			continue
		}
		start := fset.Position(fn.Body.Pos()).Offset
		end := fset.Position(fn.Body.End()).Offset
		return string(src[start:end]), nil
	}
	return "", fmt.Errorf("function %s missing in %s", name, path)
}

func compactSource(value string) string {
	value = strings.ReplaceAll(value, " ", "")
	value = strings.ReplaceAll(value, "\n", "")
	value = strings.ReplaceAll(value, "\t", "")
	return value
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
		if len(call.Args) < 7 ||
			!identName(call.Args[2], "tokenClient") ||
			!identName(call.Args[3], "tokenClient") ||
			!identName(call.Args[4], "tokenClient") ||
			!identName(call.Args[5], "tokenClient") ||
			!identName(call.Args[6], "tokenClient") {
			invalid = true
		}
		return true
	})
	if !found {
		return fmt.Errorf("production tui.%s call missing in %s", name, path)
	}
	if invalid {
		return fmt.Errorf("production tui.%s management arguments are not the management client", name)
	}
	return nil
}

func identName(expr ast.Expr, name string) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == name
}
