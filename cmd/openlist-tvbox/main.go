package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"openlist-tvbox/internal/admin"
	"openlist-tvbox/internal/auth"
	"openlist-tvbox/internal/config"
	"openlist-tvbox/internal/gateway"
	"openlist-tvbox/internal/mount"
	"openlist-tvbox/internal/openlist"
)

func main() {
	var configPath string
	var listen string
	var hashPassword string
	var printConfigExample bool
	var printConfigJSON bool
	flag.StringVar(&configPath, "config", getenv("OPENLIST_TVBOX_CONFIG", "config.json"), "path to config file")
	flag.StringVar(&listen, "listen", getenv("OPENLIST_TVBOX_LISTEN", ":18989"), "HTTP listen address")
	flag.StringVar(&hashPassword, "hash-password", "", "print a bcrypt hash for an access code and exit")
	flag.BoolVar(&printConfigExample, "print-config-example", false, "print a starter YAML config and exit")
	flag.BoolVar(&printConfigJSON, "print-config-json", false, "print the config as declarative JSON and exit")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if printConfigExample {
		_, _ = os.Stdout.WriteString(starterConfigYAML)
		return
	}
	if printConfigJSON {
		cfg, err := config.LoadEditable(configPath)
		if err != nil {
			logConfigLoadError(logger, configPath, err)
			os.Exit(1)
		}
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			logger.Error("marshal config json failed", "error", err)
			os.Exit(1)
		}
		data = append(data, '\n')
		_, _ = os.Stdout.Write(data)
		return
	}
	if hashPassword != "" {
		hash, err := auth.HashPassword(hashPassword)
		if err != nil {
			logger.Error("hash password failed", "error", err)
			os.Exit(1)
		}
		_, _ = os.Stdout.WriteString(hash + "\n")
		return
	}
	if err := config.EnsureEditableJSON(configPath); err != nil {
		logger.Error("initialize editable json config failed", "path", configPath, "error", err)
		os.Exit(1)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		logConfigLoadError(logger, configPath, err)
		os.Exit(1)
	}

	client := openlist.NewClient(http.DefaultClient, logger)
	service := mount.NewService(cfg, client, logger)
	gatewayHandler := gateway.NewServer(service, logger)
	var adminHandler *admin.Server
	applyConfig := func(cfg *config.Config) {
		reloadedClient := openlist.NewClient(http.DefaultClient, logger)
		gatewayHandler.SetService(mount.NewService(cfg, reloadedClient, logger))
		if adminHandler != nil {
			adminHandler.ApplyConfig(cfg)
		}
	}
	var handler http.Handler = gatewayHandler
	if config.IsJSONPath(configPath) {
		var err error
		adminHandler, err = admin.NewServer(admin.Options{
			ConfigPath: configPath,
			Listen:     listen,
			Logger:     logger,
			OnSaved:    applyConfig,
		})
		if err != nil {
			logger.Error("admin setup failed", "error", err)
			os.Exit(1)
		}
		handler = routeAdmin(adminHandler.Handler(), gatewayHandler)
	}
	server := &http.Server{
		Addr:              listen,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	reloadCtx, stopReload := context.WithCancel(context.Background())
	defer stopReload()
	go watchConfig(reloadCtx, configPath, logger, applyConfig)

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("openlist-tvbox listening", "addr", listen)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	signalCtx, stopSignal := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignal()

	select {
	case err := <-serverErr:
		if err != nil {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
		return
	case <-signalCtx.Done():
	}
	stopSignal()

	logger.Info("shutdown signal received")
	stopReload()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("server graceful shutdown failed", "error", err)
		if closeErr := server.Close(); closeErr != nil {
			logger.Error("server close failed", "error", closeErr)
		}
	} else {
		logger.Info("server graceful shutdown completed")
	}
	if err := <-serverErr; err != nil {
		logger.Error("server stopped with error", "error", err)
	}
}

func routeAdmin(adminHandler, gatewayHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/admin/") || r.URL.Path == "/admin" {
			adminHandler.ServeHTTP(w, r)
			return
		}
		gatewayHandler.ServeHTTP(w, r)
	})
}

type configFileState struct {
	modTime time.Time
	size    int64
	sum     uint64
}

func watchConfig(ctx context.Context, configPath string, logger *slog.Logger, apply func(*config.Config)) {
	watchConfigWithTimings(ctx, configPath, logger, 2*time.Second, 700*time.Millisecond, apply)
}

func watchConfigWithTimings(ctx context.Context, configPath string, logger *slog.Logger, interval, debounce time.Duration, apply func(*config.Config)) {
	state, err := statConfigFile(configPath)
	if err != nil {
		logger.Warn("config watcher disabled", "error", err)
		return
	}
	absPath, absErr := filepath.Abs(configPath)
	if absErr != nil {
		absPath = configPath
	}
	logger.Info("config watcher started", "path", absPath)

	reloadRequests := make(chan struct{}, 1)
	go runConfigReloadWorker(ctx, configPath, absPath, logger, debounce, reloadRequests, apply)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			next, err := statConfigFile(configPath)
			if err != nil {
				logger.Warn("config stat failed", "error", err)
				continue
			}
			if next == state {
				continue
			}
			state = next
			queueConfigReload(reloadRequests)
		}
	}
}

func runConfigReloadWorker(ctx context.Context, configPath, absPath string, logger *slog.Logger, debounce time.Duration, reloadRequests <-chan struct{}, apply func(*config.Config)) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-reloadRequests:
		}
		if !waitForConfigDebounce(ctx, debounce, reloadRequests) {
			return
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			logger.Error("config reload failed; keeping current config", "error", err)
			continue
		}
		apply(cfg)
		logger.Info("config reloaded", "path", absPath)
	}
}

func waitForConfigDebounce(ctx context.Context, debounce time.Duration, reloadRequests <-chan struct{}) bool {
	timer := time.NewTimer(debounce)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-reloadRequests:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(debounce)
		case <-timer.C:
			return true
		}
	}
}

func queueConfigReload(reloadRequests chan<- struct{}) {
	select {
	case reloadRequests <- struct{}{}:
	default:
	}
}

func statConfigFile(path string) (configFileState, error) {
	info, err := os.Stat(path)
	if err != nil {
		return configFileState{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return configFileState{}, err
	}
	return configFileState{modTime: info.ModTime(), size: info.Size(), sum: checksum(data)}, nil
}

func checksum(data []byte) uint64 {
	const (
		offset uint64 = 14695981039346656037
		prime  uint64 = 1099511628211
	)
	sum := offset
	for _, b := range data {
		sum ^= uint64(b)
		sum *= prime
	}
	return sum
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func logConfigLoadError(logger *slog.Logger, configPath string, err error) {
	absPath, absErr := filepath.Abs(configPath)
	if absErr != nil {
		absPath = configPath
	}
	logger.Error("load config failed", "path", absPath, "error", err)
	if !errors.Is(err, os.ErrNotExist) {
		return
	}
	logger.Info("config hint", "message", "use -config to specify a config file or set OPENLIST_TVBOX_CONFIG")
	for _, candidate := range existingConfigCandidates() {
		logger.Info("config hint", "message", "found config candidate", "path", candidate)
	}
	logger.Info("config hint", "message", "run openlist-tvbox -print-config-example to print a starter YAML config")
}

func existingConfigCandidates() []string {
	names := []string{"config.yaml", "config.yml", "config.example.yaml", "config.example.yml", "config.example.json"}
	out := make([]string, 0, len(names))
	for _, name := range names {
		if _, err := os.Stat(name); err == nil {
			if abs, absErr := filepath.Abs(name); absErr == nil {
				out = append(out, abs)
			} else {
				out = append(out, name)
			}
		}
	}
	return out
}

const starterConfigYAML = `public_base_url: http://127.0.0.1:18989
trust_x_forwarded_for: false
tvbox:
  site_key: openlist_tvbox
  site_name: OpenList
  timeout: 15
  searchable: 1
  quick_search: 0
  changeable: 0
backends:
  - id: main
    server: https://openlist.example.com
    auth_type: api_key
    api_key_env: OPENLIST_MAIN_API_KEY
subs:
  - id: all
    path: /sub
    site_key: openlist_tvbox
    site_name: OpenList
    mounts:
      - id: movies
        name: Movies
        backend: main
        path: /Movies
        search: true
        refresh: false
        hidden: false
`
