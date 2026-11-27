// SPDX-License-Identifier: GPL-2.0-or-later
// Opt-in adoption diagnostics: a RoundTripper wrapper that reports only the
// numeric HTTP status of inform replies, and only when it changes, so a
// repeated 404 does not spam the log. It never inspects or modifies the
// request, response, body or error, and never logs URLs, headers, bodies,
// error strings, keys or credentials.

package main

import (
	"log"
	"net/http"
	"sync"
)

// statusLoggingTransport wraps base (http.DefaultTransport in main) and logs
// "inform http status: <code>" on every status transition. A mutex guards the
// transition state; the wrapped exchange is untouched.
type statusLoggingTransport struct {
	base http.RoundTripper
	mu   sync.Mutex
	last int // last status seen; 0 = nothing logged yet
}

func newStatusLoggingTransport(base http.RoundTripper) *statusLoggingTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &statusLoggingTransport{base: base}
}

func (t *statusLoggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if resp != nil {
		t.noteStatus(resp.StatusCode)
	}
	// Errors (and responses) propagate untouched; nothing is printed for them.
	return resp, err
}

func (t *statusLoggingTransport) noteStatus(code int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if code != t.last {
		t.last = code
		log.Printf("inform http status: %d", code)
	}
}
