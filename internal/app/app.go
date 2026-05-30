package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"ilonasin/internal/config"
	"ilonasin/internal/credentials"
	"ilonasin/internal/home"
	"ilonasin/internal/provider"
	"ilonasin/internal/server"
	"ilonasin/internal/storage/sqlite"
	"ilonasin/internal/tui"
)

type Options struct {
	ConfigPath string
	Stdout     io.Writer
	Stderr     io.Writer
}

type runtime struct {
	HomeDir    string
	ConfigPath string
	Config     config.Config
	Registry   provider.Registry
	Store      *sqlite.Store
	cleanup    func()
}

func Serve(opts Options) error {
	rt, err := bootstrap(context.Background(), opts, false)
	if err != nil {
		return err
	}
	defer rt.Store.Close()

	auth := credentials.Service{Repo: rt.Store}
	upstreams := credentials.UpstreamService{Registry: rt.Registry, Repo: rt.Store}
	srv := &http.Server{
		Addr:              rt.Config.Server.Bind,
		Handler:           server.New(rt.Registry, auth, upstreams, rt.Store).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	fmt.Fprintf(opts.Stdout, "ilonasin serving on %s\n", rt.Config.Server.Bind)
	return srv.ListenAndServe()
}

func ServeCheck(opts Options) error {
	rt, err := bootstrap(context.Background(), opts, true)
	if err != nil {
		return err
	}
	defer rt.cleanup()
	defer rt.Store.Close()

	checkDBDir, err := os.MkdirTemp("", "ilonasin-serve-check-db-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(checkDBDir)
	checkStore, err := sqlite.Open(context.Background(), filepath.Join(checkDBDir, "ilonasin.sqlite"))
	if err != nil {
		return err
	}
	defer checkStore.Close()

	tokenService := credentials.Service{Repo: checkStore}
	upstreamService := credentials.UpstreamService{Registry: rt.Registry, Repo: checkStore}
	created, err := tokenService.Create(context.Background(), "serve-check")
	if err != nil {
		return err
	}

	if instance, ok := firstAPIKeyProvider(rt.Registry); ok {
		if _, err := upstreamService.AddAPIKey(context.Background(), instance.ID, "serve-check-upstream", "sk-serve-check-upstream"); err != nil {
			return err
		}
		resolved, err := upstreamService.ResolveAPIKey(context.Background(), instance.ID)
		if err != nil {
			return err
		}
		if resolved.APIKey == "" {
			return fmt.Errorf("resolved empty upstream api key")
		}
		if err := upstreamService.Disable(context.Background(), resolved.ID); err != nil {
			return err
		}
		if _, err := upstreamService.ResolveAPIKey(context.Background(), instance.ID); !errors.Is(err, credentials.ErrNoEligibleCredential) {
			return fmt.Errorf("disabled upstream credential resolved err=%v", err)
		}
	}

	handler := server.New(rt.Registry, tokenService, upstreamService, checkStore).Handler()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	errc := make(chan error, 1)
	go func() {
		errc <- srv.Serve(listener)
	}()
	defer srv.Shutdown(context.Background())

	base := "http://" + listener.Addr().String()
	if status, err := getStatus(base+"/v1/models", ""); err != nil || status != http.StatusUnauthorized {
		return fmt.Errorf("unauthenticated models status=%d err=%v", status, err)
	}
	if status, err := getStatus(base+"/v1/models", created.Token); err != nil || status != http.StatusOK {
		return fmt.Errorf("authenticated models status=%d err=%v", status, err)
	}
	if err := tokenService.Disable(context.Background(), created.Metadata.ID); err != nil {
		return err
	}
	if status, err := getStatus(base+"/v1/models", created.Token); err != nil || status != http.StatusUnauthorized {
		return fmt.Errorf("disabled token models status=%d err=%v", status, err)
	}
	body := []byte(`{"model":"deepseek/deepseek-v4-pro","messages":[{"role":"user","content":"check"}],"unsupported":true}`)
	created2, err := tokenService.Create(context.Background(), "serve-check-chat")
	if err != nil {
		return err
	}
	if status, err := postStatus(base+"/v1/chat/completions", created2.Token, body); err != nil || status != http.StatusBadRequest {
		return fmt.Errorf("unsupported chat status=%d err=%v", status, err)
	}

	_ = srv.Shutdown(context.Background())
	select {
	case err := <-errc:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
	case <-time.After(time.Second):
		return fmt.Errorf("server did not shut down")
	}
	return nil
}

func Manage(opts Options) error {
	rt, err := bootstrap(context.Background(), opts, false)
	if err != nil {
		return err
	}
	defer rt.Store.Close()
	return tui.Run(rt.Config, rt.Registry, credentials.Service{Repo: rt.Store}, credentials.UpstreamService{Registry: rt.Registry, Repo: rt.Store})
}

func ManageCheck(opts Options) error {
	rt, err := bootstrap(context.Background(), opts, true)
	if err != nil {
		return err
	}
	defer rt.cleanup()
	defer rt.Store.Close()
	if err := exerciseLocalTokenCheck(context.Background()); err != nil {
		return err
	}
	if err := exerciseUpstreamCredentialCheck(context.Background(), rt.Registry, rt.Config, opts); err != nil {
		return err
	}
	var buf bytes.Buffer
	tokenService := credentials.Service{Repo: rt.Store}
	if err := tui.Check(rt.Config, rt.Registry, tokenService, credentials.UpstreamService{Registry: rt.Registry, Repo: rt.Store}, &buf); err != nil {
		return err
	}
	if opts.Stdout != nil {
		_, _ = opts.Stdout.Write(buf.Bytes())
	}
	return nil
}

func bootstrap(ctx context.Context, opts Options, checkSafeHome bool) (*runtime, error) {
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	envHome := os.Getenv(home.EnvName)
	cleanup := func() {}
	if checkSafeHome && envHome == "" {
		tmp, err := os.MkdirTemp("", "ilonasin-check-*")
		if err != nil {
			return nil, err
		}
		envHome = tmp
		cleanup = func() { _ = os.RemoveAll(tmp) }
	}
	homeDir, err := home.Resolve(envHome)
	if err != nil {
		cleanup()
		return nil, err
	}
	if err := home.Ensure(homeDir); err != nil {
		cleanup()
		return nil, err
	}
	cfg, cfgPath, err := config.LoadOrCreate(opts.ConfigPath, homeDir, opts.ConfigPath != "")
	if err != nil {
		cleanup()
		return nil, err
	}
	for _, dir := range []string{cfg.Paths.DataDir, cfg.Paths.LogDir, cfg.Paths.CacheDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			cleanup()
			return nil, err
		}
	}
	registry, err := provider.NewRegistry(cfg)
	if err != nil {
		cleanup()
		return nil, err
	}
	store, err := sqlite.Open(ctx, filepath.Clean(cfg.Paths.Database))
	if err != nil {
		cleanup()
		return nil, err
	}
	return &runtime{HomeDir: homeDir, ConfigPath: cfgPath, Config: cfg, Registry: registry, Store: store, cleanup: cleanup}, nil
}

func exerciseUpstreamCredentialCheck(ctx context.Context, registry provider.Registry, cfg config.Config, opts Options) error {
	if _, ok := firstAPIKeyProvider(registry); !ok {
		return nil
	}
	checkDBDir, err := os.MkdirTemp("", "ilonasin-manage-check-db-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(checkDBDir)
	store, err := sqlite.Open(ctx, filepath.Join(checkDBDir, "ilonasin.sqlite"))
	if err != nil {
		return err
	}
	defer store.Close()
	service := credentials.UpstreamService{Registry: registry, Repo: store}
	if err := tui.ExerciseUpstreamCredentialLifecycle(ctx, cfg, registry, service); err != nil {
		return err
	}
	return nil
}

func exerciseLocalTokenCheck(ctx context.Context) error {
	checkDBDir, err := os.MkdirTemp("", "ilonasin-manage-check-local-db-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(checkDBDir)
	store, err := sqlite.Open(ctx, filepath.Join(checkDBDir, "ilonasin.sqlite"))
	if err != nil {
		return err
	}
	defer store.Close()
	return tui.ExerciseTokenLifecycle(ctx, credentials.Service{Repo: store})
}

func firstAPIKeyProvider(registry provider.Registry) (provider.Instance, bool) {
	for _, instance := range registry.List() {
		if instance.APIKey && !instance.Placeholder {
			return instance, true
		}
	}
	return provider.Instance{}, false
}

func getStatus(url, token string) (int, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func postStatus(url, token string, body []byte) (int, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}
