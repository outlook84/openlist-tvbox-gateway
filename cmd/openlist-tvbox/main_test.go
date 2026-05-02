package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"openlist-tvbox/internal/config"
)

func TestWatchConfigReloadsChangedFile(t *testing.T) {
	path := writeTestConfig(t, "Movies")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reloaded := make(chan string, 1)
	go watchConfigWithTimings(ctx, path, slog.New(slog.NewTextHandler(os.Stdout, nil)), 20*time.Millisecond, 20*time.Millisecond, func(cfg *config.Config) {
		reloaded <- cfg.Subs[0].Mounts[0].Name
	})

	time.Sleep(40 * time.Millisecond)
	writeFile(t, path, testConfigYAML("Shows"))

	select {
	case got := <-reloaded:
		if got != "Shows" {
			t.Fatalf("reloaded mount name = %q, want Shows", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for config reload")
	}
}

func TestWatchConfigKeepsCurrentConfigWhenReloadFails(t *testing.T) {
	path := writeTestConfig(t, "Movies")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reloaded := make(chan string, 1)
	go watchConfigWithTimings(ctx, path, slog.New(slog.NewTextHandler(os.Stdout, nil)), 20*time.Millisecond, 20*time.Millisecond, func(cfg *config.Config) {
		reloaded <- cfg.Subs[0].Mounts[0].Name
	})

	time.Sleep(40 * time.Millisecond)
	writeFile(t, path, "backends: []\nsubs: []\n")

	select {
	case got := <-reloaded:
		t.Fatalf("unexpected reload with invalid config: %q", got)
	case <-time.After(150 * time.Millisecond):
	}
}

func writeTestConfig(t *testing.T, mountName string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, testConfigYAML(mountName))
	return path
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func testConfigYAML(mountName string) string {
	return `backends:
  - id: b1
    server: https://openlist.example.com
subs:
  - id: default
    mounts:
      - id: media
        name: ` + mountName + `
        backend: b1
        path: /Media
`
}
