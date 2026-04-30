package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

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
	flag.StringVar(&configPath, "config", getenv("OPENLIST_TVBOX_CONFIG", "config.json"), "path to config file")
	flag.StringVar(&listen, "listen", getenv("OPENLIST_TVBOX_LISTEN", ":18989"), "HTTP listen address")
	flag.StringVar(&hashPassword, "hash-password", "", "print a bcrypt hash for an access code and exit")
	flag.BoolVar(&printConfigExample, "print-config-example", false, "print a starter YAML config and exit")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if printConfigExample {
		_, _ = os.Stdout.WriteString(starterConfigYAML)
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
	cfg, err := config.Load(configPath)
	if err != nil {
		logConfigLoadError(logger, configPath, err)
		os.Exit(1)
	}

	client := openlist.NewClient(http.DefaultClient, logger)
	service := mount.NewService(cfg, client, logger)
	server := &http.Server{
		Addr:              listen,
		Handler:           gateway.NewServer(service, logger),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("openlist-tvbox listening", "addr", listen)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
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
