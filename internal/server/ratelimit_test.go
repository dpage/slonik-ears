package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTheLimiterOnlySpendsOnFailures(t *testing.T) {
	// The case this protects: a hall of phones behind one NAT address all
	// signing in at the start of a talk. Every one of them is correct, so
	// none of them should ever be turned away.
	l := newFailureLimiter(3, time.Minute)
	for i := range 100 {
		if !l.allow("198.51.100.7") {
			t.Fatalf("a correct passcode was refused on attempt %d", i+1)
		}
	}
}

func TestTheLimiterStopsRepeatedGuessing(t *testing.T) {
	l := newFailureLimiter(3, time.Minute)
	for range 3 {
		if !l.allow("198.51.100.7") {
			t.Fatal("refused before the budget was spent")
		}
		l.failed("198.51.100.7")
	}
	if l.allow("198.51.100.7") {
		t.Error("a fourth guess was allowed on a budget of three")
	}
	// One address running out must not affect anybody else.
	if !l.allow("198.51.100.8") {
		t.Error("a different address was caught up in the block")
	}
}

func TestTheLimiterRefillsOverTime(t *testing.T) {
	l := newFailureLimiter(3, time.Minute)
	now := time.Now()
	l.now = func() time.Time { return now }

	for range 3 {
		l.failed("198.51.100.7")
	}
	if l.allow("198.51.100.7") {
		t.Fatal("the budget was not spent")
	}

	// Twenty seconds is a third of the window, so a third of the budget.
	now = now.Add(21 * time.Second)
	if !l.allow("198.51.100.7") {
		t.Error("the budget had not begun to refill after a third of the window")
	}
}

func TestACookieIsSecureBehindAProxyThatTerminatedTLS(t *testing.T) {
	// The deployment the documentation recommends puts a proxy in front, so
	// r.TLS is nil even though the browser is on HTTPS. The cookie's value is
	// the admin token itself, so getting this wrong hands over the configured
	// secret rather than a session that can be revoked.
	r := httptest.NewRequest(http.MethodPost, "/api/admin/session", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	if !requestIsHTTPS(r) {
		t.Error("a request forwarded as https was treated as plain http")
	}

	plain := httptest.NewRequest(http.MethodPost, "/api/admin/session", nil)
	if requestIsHTTPS(plain) {
		t.Error("a plain http request was treated as https")
	}
}

func TestTheForwardedAddressIsTheOneTheProxySaw(t *testing.T) {
	// Everything to the left of the last entry was supplied by the client, so
	// taking the leftmost lets a caller pick its own identity and keep
	// guessing for ever under a fresh address each time.
	s := &Server{cfg: Config{}}
	s.cfg.Server.TrustProxy = true

	r := httptest.NewRequest(http.MethodPost, "/api/session", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.9, 198.51.100.7")
	if got := s.clientIP(r); got != "198.51.100.7" {
		t.Errorf("client address is %q, want the address the proxy observed", got)
	}
}
