package auth

import (
	"sync"
	"time"
)

const (
	DefaultFailureLimit    = 5
	DefaultFailureCooldown = 30 * time.Second
)

type FailureLimiter struct {
	mu       sync.Mutex
	failures map[string]Failure
	limit    int
	cooldown time.Duration
}

type Failure struct {
	Count        int
	LastFailedAt time.Time
	BlockedAt    time.Time
}

func NewFailureLimiter(limit int, cooldown time.Duration) *FailureLimiter {
	if limit <= 0 {
		limit = DefaultFailureLimit
	}
	if cooldown <= 0 {
		cooldown = DefaultFailureCooldown
	}
	return &FailureLimiter{
		failures: map[string]Failure{},
		limit:    limit,
		cooldown: cooldown,
	}
}

func (l *FailureLimiter) Blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	failure, ok := l.failures[key]
	if !ok {
		return false
	}
	if failure.Count < l.limit {
		if time.Since(failure.LastFailedAt) >= l.cooldown {
			delete(l.failures, key)
		}
		return false
	}
	if time.Since(failure.BlockedAt) >= l.cooldown {
		delete(l.failures, key)
		return false
	}
	return true
}

func (l *FailureLimiter) RecordFailure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	failure := l.failures[key]
	now := time.Now()
	if !failure.LastFailedAt.IsZero() && now.Sub(failure.LastFailedAt) >= l.cooldown {
		failure = Failure{}
	}
	failure.Count++
	failure.LastFailedAt = now
	if failure.Count >= l.limit && failure.BlockedAt.IsZero() {
		failure.BlockedAt = now
	}
	l.failures[key] = failure
}

func (l *FailureLimiter) Clear(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, key)
}

func (l *FailureLimiter) Set(key string, failure Failure) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures[key] = failure
}

func (l *FailureLimiter) Get(key string) (Failure, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	failure, ok := l.failures[key]
	return failure, ok
}
