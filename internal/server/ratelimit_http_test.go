package server_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestSignInIsThrottledAfterRepeatedFailures(t *testing.T) {
	ts, _, _ := adminServer(t)

	var last int
	for range 40 {
		resp, err := http.Post(ts.URL+"/api/admin/session", "application/json",
			strings.NewReader(`{"token":"not-the-token"}`))
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		last = resp.StatusCode
		if last == http.StatusTooManyRequests {
			break
		}
	}
	if last != http.StatusTooManyRequests {
		t.Errorf("guessing the admin token was never throttled; last response was %d", last)
	}
}
