package server

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// failureLimiter throttles repeated failed authentication attempts from one
// address.
//
// Only failures are counted, which matters more here than it might elsewhere.
// A venue puts several hundred phones behind a single NAT address, so a limiter
// that counted every attempt would let one hall lock itself out of its own
// transcript the moment a talk started. Somebody typing the passcode correctly
// costs nothing; somebody working through the keyspace costs one token per
// guess, and there is no way to do the second without the first.
//
// The budget is per address and refills steadily, so a burst of fumbled codes
// during the first minute of a session clears itself within a minute or two
// rather than needing anybody to intervene.
type failureLimiter struct {
	burst   float64
	refill  float64 // tokens per second
	now     func() time.Time
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	seen   time.Time
}

// newFailureLimiter allows burst consecutive failures from one address, with
// the budget refilling completely over window.
func newFailureLimiter(burst int, window time.Duration) *failureLimiter {
	if burst < 1 {
		burst = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	return &failureLimiter{
		burst:   float64(burst),
		refill:  float64(burst) / window.Seconds(),
		now:     time.Now,
		buckets: make(map[string]*bucket),
	}
}

// allow reports whether another attempt from key should be answered at all.
// It does not spend anything: an attempt that turns out to be correct must not
// count against the next one.
func (l *failureLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.tokensLocked(key) >= 1
}

// failed records a failed attempt, spending one token.
func (l *failureLimiter) failed(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.bucketLocked(key)
	b.tokens = l.tokensLocked(key) - 1
	if b.tokens < 0 {
		b.tokens = 0
	}
}

func (l *failureLimiter) tokensLocked(key string) float64 {
	b := l.bucketLocked(key)
	now := l.now()
	if elapsed := now.Sub(b.seen).Seconds(); elapsed > 0 {
		b.tokens += elapsed * l.refill
		if b.tokens > l.burst {
			b.tokens = l.burst
		}
	}
	b.seen = now
	return b.tokens
}

func (l *failureLimiter) bucketLocked(key string) *bucket {
	b, ok := l.buckets[key]
	if !ok {
		// A new address starts with a full budget, so the common case of
		// somebody mistyping a passcode once is never affected.
		b = &bucket{tokens: l.burst, seen: l.now()}
		l.buckets[key] = b
		l.sweepLocked()
	}
	return b
}

// sweepLocked forgets addresses whose budget has long since refilled, so an
// event running all day does not accumulate an entry per phone that ever
// fumbled a code. Called only when a new address appears, which bounds the
// work without needing a goroutine to own it.
func (l *failureLimiter) sweepLocked() {
	if len(l.buckets) < 1024 {
		return
	}
	cutoff := l.now().Add(-2 * time.Duration(l.burst/l.refill) * time.Second)
	for k, b := range l.buckets {
		if b.seen.Before(cutoff) {
			delete(l.buckets, k)
		}
	}
}

// requestIsHTTPS reports whether the browser reached the server over HTTPS,
// including when a reverse proxy terminated it.
//
// The forwarded header is honoured whatever trust_proxy says, deliberately.
// Getting this wrong in the cautious direction marks a cookie Secure on a
// plain HTTP deployment, and the only person who can arrange that is the
// client itself, which merely has to type its passcode again. Getting it wrong
// in the other direction puts the admin token, which is the cookie's own
// value, on the wire in clear text the first time anybody follows an http://
// link to the same host.
func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
