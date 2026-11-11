package refreshguard

import (
	"errors"
	"sync"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/model"
)

// ErrBusy is returned when the global refresh concurrency limit is reached.
var ErrBusy = errors.New("refresh concurrency limit reached")

const (
	bucketIdleGC     = 10 * time.Minute
	bucketMaxEntries = 10000
)

// Limiter is a per-key token bucket limiter.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	lastGC  time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

// NewLimiter creates an empty limiter.
func NewLimiter() *Limiter {
	return &Limiter{buckets: make(map[string]*bucket), lastGC: time.Now()}
}

// Allow consumes one token for key. rate is tokens per second, burst is the bucket capacity.
// When the request is denied, the second return value is the time until the next token.
func (l *Limiter) Allow(key string, rate, burst float64, now time.Time) (bool, time.Duration) {
	if burst < 1 {
		burst = 1
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: burst, last: now}
		l.buckets[key] = b
	}
	if rate > 0 && now.After(b.last) {
		b.tokens += now.Sub(b.last).Seconds() * rate
		if b.tokens > burst {
			b.tokens = burst
		}
	}
	b.last = now
	l.gcLocked(now)
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	if rate <= 0 {
		return false, time.Second
	}
	wait := time.Duration((1 - b.tokens) / rate * float64(time.Second))
	if wait < time.Second {
		wait = time.Second
	}
	return false, wait
}

func (l *Limiter) gcLocked(now time.Time) {
	if now.Sub(l.lastGC) < bucketIdleGC || len(l.buckets) < bucketMaxEntries {
		return
	}
	l.lastGC = now
	for k, b := range l.buckets {
		if now.Sub(b.last) > bucketIdleGC {
			delete(l.buckets, k)
		}
	}
}

// Group deduplicates concurrent refreshes of the same path.
type Group struct {
	mu    sync.Mutex
	calls map[string]*call
}

type call struct {
	done chan struct{}
	objs []model.Obj
	err  error
}

// NewGroup creates an empty group.
func NewGroup() *Group {
	return &Group{calls: make(map[string]*call)}
}

// Do runs fn once for a given key while other callers wait for its result.
func (g *Group) Do(key string, fn func() ([]model.Obj, error)) ([]model.Obj, error) {
	g.mu.Lock()
	if c, ok := g.calls[key]; ok {
		g.mu.Unlock()
		<-c.done
		return c.objs, c.err
	}
	c := &call{done: make(chan struct{})}
	g.calls[key] = c
	g.mu.Unlock()
	c.objs, c.err = fn()
	close(c.done)
	g.mu.Lock()
	delete(g.calls, key)
	g.mu.Unlock()
	return c.objs, c.err
}

// Counter bounds concurrent work by a limit read at call time.
type Counter struct {
	mu sync.Mutex
	n  int
}

// NewCounter creates an empty counter.
func NewCounter() *Counter {
	return &Counter{}
}

// TryEnter reserves a slot. limit is read from settings on every call, so it can change live.
func (c *Counter) TryEnter(limit int) bool {
	if limit <= 0 {
		limit = 1
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.n >= limit {
		return false
	}
	c.n++
	return true
}

// Leave releases a slot.
func (c *Counter) Leave() {
	c.mu.Lock()
	if c.n > 0 {
		c.n--
	}
	c.mu.Unlock()
}
