package logging

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestBufferKeepsRecentEntries(t *testing.T) {
	buffer := NewBuffer(2)
	buffer.Append(Entry{Time: time.Unix(1, 0), Level: "INFO", Message: "one"})
	buffer.Append(Entry{Time: time.Unix(2, 0), Level: "WARN", Message: "two"})
	buffer.Append(Entry{Time: time.Unix(3, 0), Level: "ERROR", Message: "three"})

	got := buffer.Snapshot(10, slog.LevelInfo)
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Message != "two" || got[1].Message != "three" {
		t.Fatalf("snapshot = %#v", got)
	}
}

func TestBufferAppliesLevelFilterBeforeLimit(t *testing.T) {
	buffer := NewBuffer(10)
	buffer.Append(Entry{Time: time.Unix(1, 0), Level: "WARN", Message: "warn"})
	buffer.Append(Entry{Time: time.Unix(2, 0), Level: "INFO", Message: "info one"})
	buffer.Append(Entry{Time: time.Unix(3, 0), Level: "INFO", Message: "info two"})

	got := buffer.Snapshot(1, slog.LevelWarn)
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Message != "warn" {
		t.Fatalf("snapshot = %#v", got)
	}
}

func TestHandlerBuffersSlogRecords(t *testing.T) {
	buffer := NewBuffer(10)
	logger := slog.New(NewHandler(nil, buffer)).With("component", "test")

	logger.Warn("hello", "count", 3)

	got := buffer.Snapshot(10, slog.LevelInfo)
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Level != "WARN" || got[0].Message != "hello" {
		t.Fatalf("entry = %#v", got[0])
	}
	if got[0].Attrs["component"] != "test" || got[0].Attrs["count"] != int64(3) {
		t.Fatalf("attrs = %#v", got[0].Attrs)
	}
}

func TestSubscribeReceivesNewEntries(t *testing.T) {
	buffer := NewBuffer(10)
	ch, unsubscribe := buffer.Subscribe()
	defer unsubscribe()

	buffer.Append(Entry{Time: time.Now(), Level: "INFO", Message: "live"})

	select {
	case got := <-ch:
		if got.Message != "live" {
			t.Fatalf("message = %q", got.Message)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for entry")
	}
}

func TestHandlerEnabledAllowsBufferOnlyLogger(t *testing.T) {
	handler := NewHandler(nil, NewBuffer(10))
	if !handler.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("buffer-only handler should be enabled")
	}
}

func TestHandlerBuffersDebugWithoutForwardingBelowNextLevel(t *testing.T) {
	buffer := NewBuffer(10)
	var out bytes.Buffer
	handler := NewHandler(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelInfo}), buffer)
	logger := slog.New(handler)

	logger.Debug("debug message")
	logger.Info("info message")

	got := buffer.Snapshot(10, slog.LevelDebug)
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Level != "DEBUG" || got[0].Message != "debug message" {
		t.Fatalf("debug entry = %#v", got[0])
	}
	if got[1].Level != "INFO" || got[1].Message != "info message" {
		t.Fatalf("info entry = %#v", got[1])
	}
	if bytes.Contains(out.Bytes(), []byte("debug message")) {
		t.Fatalf("debug message was forwarded to next handler: %q", out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("info message")) {
		t.Fatalf("info message was not forwarded to next handler: %q", out.String())
	}
}
