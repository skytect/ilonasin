package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
		Handler:           server.New(rt.Registry, auth, upstreams, chatAdapters(nil), rt.Store).Handler(),
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

	instances := apiKeyProviders(rt.Registry)
	if len(instances) > 0 {
		instance := instances[0]
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

	fakeUpstream := newServeCheckUpstream()
	defer fakeUpstream.server.Close()
	checkRegistry := baseURLOverrideRegistry{Registry: rt.Registry, baseURL: fakeUpstream.server.URL}
	handler := server.New(checkRegistry, tokenService, upstreamService, chatAdapters(fakeUpstream.server.Client()), checkStore).Handler()
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
	for _, instance := range instances {
		if _, err := upstreamService.AddAPIKey(context.Background(), instance.ID, "serve-check-adapter", "sk-serve-check-adapter"); err != nil {
			return err
		}
		if err := exerciseChatAdapterCheck(context.Background(), base, created2.Token, instance, fakeUpstream, checkStore); err != nil {
			return err
		}
	}
	if len(instances) > 0 {
		if err := assertHomeCredentialCountsZero(context.Background(), rt.Store); err != nil {
			return err
		}
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
	instances := apiKeyProviders(registry)
	if len(instances) == 0 {
		return provider.Instance{}, false
	}
	return instances[0], true
}

func apiKeyProviders(registry provider.Registry) []provider.Instance {
	var out []provider.Instance
	for _, instance := range registry.List() {
		if instance.APIKey && !instance.Placeholder {
			out = append(out, instance)
		}
	}
	return out
}

func chatAdapters(client *http.Client) provider.StaticChatAdapters {
	adapter := provider.NewHTTPChatAdapter(client)
	return provider.StaticChatAdapters{
		"deepseek":   adapter,
		"openrouter": adapter,
	}
}

type baseURLOverrideRegistry struct {
	provider.Registry
	baseURL string
}

func (r baseURLOverrideRegistry) Get(id string) (provider.Instance, bool) {
	instance, ok := r.Registry.Get(id)
	if ok && r.baseURL != "" {
		instance.BaseURL = r.baseURL
		if instance.Type == "openrouter" {
			instance.BaseURL += "/api/v1"
		}
	}
	return instance, ok
}

type serveCheckUpstream struct {
	server   *httptest.Server
	mu       sync.Mutex
	observed map[string]bool
}

func newServeCheckUpstream() *serveCheckUpstream {
	up := &serveCheckUpstream{observed: map[string]bool{}}
	up.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.URL.Path != "/chat/completions" && r.URL.Path != "/api/v1/chat/completions") || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer sk-serve-check-adapter" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "bad content type", http.StatusBadRequest)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		model, _ := body["model"].(string)
		if model == "invalid-json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"not-chat"}`))
			return
		}
		if model == "malformed-chat" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"chat.completion"}`))
			return
		}
		if model == "too-large" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(strings.Repeat("x", int(provider.MaxUpstreamChatBodyBytes)+1)))
			return
		}
		if body["stream"] == nil && (model == "deepseek-v4-pro" || model == "deepseek/deepseek-v4-pro") {
			up.mu.Lock()
			up.observed[r.URL.Path+" "+model] = true
			up.mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_check","object":"chat.completion","created":1,"model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2,"completion_tokens_details":{"reasoning_tokens":0}}}`))
	}))
	return up
}

func (u *serveCheckUpstream) sawExpected(path, model string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.observed[path+" "+model]
}

func looksLikeChatCompletion(body []byte) bool {
	var resp struct {
		Object  string `json:"object"`
		Choices []any  `json:"choices"`
		Usage   any    `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return false
	}
	return resp.Object == "chat.completion" && len(resp.Choices) > 0 && resp.Usage != nil
}

func exerciseChatAdapterCheck(ctx context.Context, base, token string, instance provider.Instance, fakeUpstream *serveCheckUpstream, store *sqlite.Store) error {
	modelID := "deepseek-v4-pro"
	if instance.Type == "openrouter" {
		modelID = "deepseek/deepseek-v4-pro"
	}
	model := instance.ID + "/" + modelID
	successBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"max_tokens":1}`, model))
	status, respBody, err := postJSON(base+"/v1/chat/completions", token, successBody)
	if err != nil || status != http.StatusOK {
		return fmt.Errorf("chat adapter success provider=%s status=%d err=%v", instance.ID, status, err)
	}
	if !looksLikeChatCompletion(respBody) {
		return fmt.Errorf("chat adapter success response was not OpenAI-compatible")
	}
	expectedPath := "/chat/completions"
	if instance.Type == "openrouter" {
		expectedPath = "/api/v1/chat/completions"
	}
	if !fakeUpstream.sawExpected(expectedPath, modelID) {
		return fmt.Errorf("chat adapter did not send expected upstream request for provider=%s", instance.ID)
	}
	if err := assertRecordedCredentialID(ctx, store); err != nil {
		return err
	}
	streamBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"stream":true}`, model))
	if status, err := postStatus(base+"/v1/chat/completions", token, streamBody); err != nil || status != http.StatusBadRequest {
		return fmt.Errorf("stream chat provider=%s status=%d err=%v", instance.ID, status, err)
	}
	toolsBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"tools":[]}`, model))
	if status, err := postStatus(base+"/v1/chat/completions", token, toolsBody); err != nil || status != http.StatusBadRequest {
		return fmt.Errorf("unsupported tools provider=%s status=%d err=%v", instance.ID, status, err)
	}
	providerOptionsBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}],"provider_options":null}`, model))
	if status, err := postStatus(base+"/v1/chat/completions", token, providerOptionsBody); err != nil || status != http.StatusBadRequest {
		return fmt.Errorf("unsupported provider_options provider=%s status=%d err=%v", instance.ID, status, err)
	}
	invalidBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}]}`, instance.ID+"/invalid-json"))
	if status, err := postStatus(base+"/v1/chat/completions", token, invalidBody); err != nil || status != http.StatusBadGateway {
		return fmt.Errorf("invalid upstream provider=%s status=%d err=%v", instance.ID, status, err)
	}
	malformedBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}]}`, instance.ID+"/malformed-chat"))
	if status, err := postStatus(base+"/v1/chat/completions", token, malformedBody); err != nil || status != http.StatusBadGateway {
		return fmt.Errorf("malformed upstream provider=%s status=%d err=%v", instance.ID, status, err)
	}
	tooLargeBody := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"check"}]}`, instance.ID+"/too-large"))
	if status, err := postStatus(base+"/v1/chat/completions", token, tooLargeBody); err != nil || status != http.StatusBadGateway {
		return fmt.Errorf("too-large upstream provider=%s status=%d err=%v", instance.ID, status, err)
	}
	return nil
}

func assertHomeCredentialCountsZero(ctx context.Context, store *sqlite.Store) error {
	for _, table := range []string{"client_tokens", "provider_credentials", "credential_secrets"} {
		var count int
		if err := store.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("selected home table %s has %d check-created rows", table, count)
		}
	}
	return nil
}

func assertRecordedCredentialID(ctx context.Context, store *sqlite.Store) error {
	var count int
	err := store.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM request_metadata
		WHERE http_status = 200
			AND credential_id IS NOT NULL
			AND prompt_tokens = 1
			AND completion_tokens = 1
			AND total_tokens = 2
	`).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("chat adapter metadata did not record credential and usage")
	}
	return nil
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

func postJSON(url, token string, body []byte) (int, []byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, respBody, nil
}
