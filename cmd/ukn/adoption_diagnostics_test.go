// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "http://192.0.2.1:8080/inform?secret=supersecretvalue", strings.NewReader("authkey=0011223344556677889900aabbccddeeff"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer topsecrettoken123")
	return req
}

func testResponse(code int) *http.Response {
	return &http.Response{
		StatusCode: code,
		Body:       io.NopCloser(bytes.NewReader([]byte("reply-body-bytes"))),
		Header:     http.Header{"Content-Type": []string{"application/x-binary"}},
	}
}

// captureLog redirects the standard logger (without timestamp flags) for the
// duration of fn.
func captureLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	old, oldFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() { log.SetOutput(old); log.SetFlags(oldFlags) }()
	fn()
	return buf.String()
}

func TestStatusTransportPassThrough(t *testing.T) {
	req := testRequest(t)
	wantResp := testResponse(200)
	gotReq := (*http.Request)(nil)
	tr := newStatusLoggingTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotReq = r
		return wantResp, nil
	}))
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("error changed: %v", err)
	}
	if resp != wantResp {
		t.Fatal("response pointer not passed through")
	}
	if gotReq != req {
		t.Fatal("request not passed through unchanged")
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil || string(body) != "reply-body-bytes" {
		t.Fatalf("body changed: %q %v", body, err)
	}
}

func TestStatusTransportErrorPassThrough(t *testing.T) {
	wantErr := errors.New("dial tcp 192.0.2.1:8080: secret-refused-detail")
	tr := newStatusLoggingTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, wantErr
	}))
	logged := captureLog(t, func() {
		resp, err := tr.RoundTrip(testRequest(t))
		if resp != nil {
			t.Errorf("resp = %v, want nil", resp)
		}
		if err != wantErr {
			t.Errorf("error not propagated untouched: %v", err)
		}
	})
	if logged != "" {
		t.Errorf("error case logged %q, want nothing", logged)
	}
	if strings.Contains(logged, "secret-refused-detail") {
		t.Error("error string leaked into log")
	}
}

func TestStatusTransportLogsOnlyTransitions(t *testing.T) {
	codes := []int{404, 404, 404, 200, 200, 503, 404}
	i := 0
	tr := newStatusLoggingTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		c := codes[i]
		i++
		return testResponse(c), nil
	}))
	logged := captureLog(t, func() {
		for range codes {
			if _, err := tr.RoundTrip(testRequest(t)); err != nil {
				t.Fatal(err)
			}
		}
	})
	want := "inform http status: 404\ninform http status: 200\ninform http status: 503\ninform http status: 404\n"
	if logged != want {
		t.Fatalf("log = %q, want %q", logged, want)
	}
}

func TestStatusTransportNoSensitiveLeak(t *testing.T) {
	tr := newStatusLoggingTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return testResponse(404), nil
	}))
	logged := captureLog(t, func() {
		if _, err := tr.RoundTrip(testRequest(t)); err != nil {
			t.Fatal(err)
		}
	})
	for _, secret := range []string{"supersecretvalue", "0011223344556677889900aabbccddeeff", "topsecrettoken123", "192.0.2.1", "/inform?"} {
		if strings.Contains(logged, secret) {
			t.Errorf("log %q leaks %q", logged, secret)
		}
	}
	if !strings.Contains(logged, "404") {
		t.Errorf("log %q missing numeric status", logged)
	}
}

func TestStatusTransportNilBaseUsesDefault(t *testing.T) {
	if tr := newStatusLoggingTransport(nil); tr.base != http.DefaultTransport {
		t.Fatalf("nil base = %v, want http.DefaultTransport", tr.base)
	}
}

func TestStatusTransportConcurrentTransitions(t *testing.T) {
	tr := newStatusLoggingTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return testResponse(404), nil
	}))
	var wg sync.WaitGroup
	logged := captureLog(t, func() {
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := tr.RoundTrip(testRequest(t)); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
	})
	// Exactly one transition (0 -> 404) may be logged; never more.
	if n := strings.Count(logged, "inform http status: 404"); n != 1 {
		t.Fatalf("repeated 404 logged %d times (%q), want exactly 1", n, logged)
	}
}
