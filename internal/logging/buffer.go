package logging

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const DefaultBufferSize = 1000

type Entry struct {
	Time    time.Time      `json:"time"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Attrs   map[string]any `json:"attrs,omitempty"`
}

type Buffer struct {
	mu          sync.RWMutex
	entries     []Entry
	next        int
	full        bool
	subscribers map[chan Entry]struct{}
}

func NewBuffer(size int) *Buffer {
	if size <= 0 {
		size = DefaultBufferSize
	}
	return &Buffer{entries: make([]Entry, size), subscribers: map[chan Entry]struct{}{}}
}

func (b *Buffer) Append(entry Entry) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.entries[b.next] = entry
	b.next = (b.next + 1) % len(b.entries)
	if b.next == 0 {
		b.full = true
	}
	for ch := range b.subscribers {
		select {
		case ch <- entry:
		default:
		}
	}
	b.mu.Unlock()
}

func (b *Buffer) Snapshot(limit int, minLevel slog.Level) []Entry {
	if b == nil {
		return nil
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	ordered := b.orderedLocked()
	if limit <= 0 || limit > len(ordered) {
		limit = len(ordered)
	}
	out := make([]Entry, 0, limit)
	for i := len(ordered) - 1; i >= 0 && len(out) < limit; i-- {
		entry := ordered[i]
		if entryLevel(entry.Level) < minLevel {
			continue
		}
		out = append(out, entry)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func (b *Buffer) Subscribe() (<-chan Entry, func()) {
	ch := make(chan Entry, 128)
	if b == nil {
		close(ch)
		return ch, func() {}
	}
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		if _, ok := b.subscribers[ch]; ok {
			delete(b.subscribers, ch)
			close(ch)
		}
		b.mu.Unlock()
	}
}

func (b *Buffer) orderedLocked() []Entry {
	count := b.next
	if b.full {
		count = len(b.entries)
	}
	out := make([]Entry, 0, count)
	if !b.full {
		out = append(out, b.entries[:b.next]...)
		return out
	}
	out = append(out, b.entries[b.next:]...)
	out = append(out, b.entries[:b.next]...)
	return out
}

func entryLevel(level string) slog.Level {
	switch level {
	case slog.LevelDebug.String():
		return slog.LevelDebug
	case slog.LevelWarn.String():
		return slog.LevelWarn
	case slog.LevelError.String():
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

type Handler struct {
	next   slog.Handler
	buffer *Buffer
	attrs  []slog.Attr
	groups []string
}

func NewHandler(next slog.Handler, buffer *Buffer) *Handler {
	return &Handler{next: next, buffer: buffer}
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	if h.buffer != nil && level >= slog.LevelDebug {
		return true
	}
	return h.next == nil || h.next.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, record slog.Record) error {
	if h.buffer != nil {
		h.buffer.Append(h.entry(record))
	}
	if h.next == nil {
		return nil
	}
	if !h.next.Enabled(ctx, record.Level) {
		return nil
	}
	return h.next.Handle(ctx, record)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := h.next
	if next != nil {
		next = next.WithAttrs(attrs)
	}
	clone := h.clone(next)
	clone.attrs = append(clone.attrs, attrs...)
	return clone
}

func (h *Handler) WithGroup(name string) slog.Handler {
	next := h.next
	if next != nil {
		next = next.WithGroup(name)
	}
	clone := h.clone(next)
	if name != "" {
		clone.groups = append(clone.groups, name)
	}
	return clone
}

func (h *Handler) clone(next slog.Handler) *Handler {
	clone := &Handler{next: next, buffer: h.buffer}
	clone.attrs = append([]slog.Attr(nil), h.attrs...)
	clone.groups = append([]string(nil), h.groups...)
	return clone
}

func (h *Handler) entry(record slog.Record) Entry {
	attrs := map[string]any{}
	for _, attr := range h.attrs {
		addAttr(attrs, h.groups, attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		addAttr(attrs, h.groups, attr)
		return true
	})
	return Entry{Time: record.Time, Level: record.Level.String(), Message: record.Message, Attrs: attrs}
}

func addAttr(attrs map[string]any, groups []string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}
	key := attr.Key
	for i := len(groups) - 1; i >= 0; i-- {
		key = groups[i] + "." + key
	}
	attrs[key] = attrValue(attr.Value)
}

func attrValue(value slog.Value) any {
	switch value.Kind() {
	case slog.KindString:
		return value.String()
	case slog.KindBool:
		return value.Bool()
	case slog.KindInt64:
		return value.Int64()
	case slog.KindUint64:
		return value.Uint64()
	case slog.KindFloat64:
		return value.Float64()
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindTime:
		return value.Time()
	case slog.KindGroup:
		group := map[string]any{}
		for _, attr := range value.Group() {
			addAttr(group, nil, attr)
		}
		return group
	case slog.KindLogValuer:
		return attrValue(value.Resolve())
	default:
		return value.String()
	}
}
