// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"
	"time"
)

func TestProvisionRetryRecoversSameVersionWithBoundedBackoff(t *testing.T) {
	var p provisionRetry
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	if !p.ready("v1", now) {
		t.Fatal("initial version was skipped")
	}
	failure := fmt.Errorf("health check failed: %w", &url.Error{Op: "Post", URL: "http://192.0.2.1/inform", Err: &net.OpError{Op: "dial", Err: errors.New("connection reset")}})
	for _, delay := range []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 5 * time.Minute, 5 * time.Minute} {
		if !p.failed("v1", failure, now) || p.delay != delay {
			t.Fatal("transient controller failure was marked complete or backoff was lost")
		}
		if p.ready("v1", now.Add(delay-time.Nanosecond)) || !p.ready("v1", now.Add(delay)) {
			t.Fatal("retry did not honor its deadline")
		}
		if !p.ready("v2", now) {
			t.Fatal("new configuration was delayed behind an obsolete retry")
		}
		now = now.Add(delay)
	}
	p.complete("v1")
	if p.ready("v1", now.Add(time.Hour)) || !p.ready("v2", now) {
		t.Fatal("successful plan repeated or blocked a new version")
	}
	if !p.failed("v2", failure, now) || p.delay != time.Minute {
		t.Fatal("new version inherited old failure backoff")
	}
}

func TestProvisionRetryRejectsPermanentFailures(t *testing.T) {
	for _, err := range []error{errors.New("unsupported setting"), errors.New("pending rollback journal exists"), context.Canceled, controllerHTTPError{400}, controllerHTTPError{401}, controllerHTTPError{404}} {
		var p provisionRetry
		if p.failed("v1", err, time.Now()) || p.ready("v1", time.Now().Add(time.Hour)) {
			t.Fatalf("permanent configuration/recovery error retries: %v", err)
		}
	}
	for _, err := range []error{context.DeadlineExceeded, controllerHTTPError{408}, controllerHTTPError{429}, controllerHTTPError{500}, controllerHTTPError{503}} {
		var p provisionRetry
		if !p.failed("v1", fmt.Errorf("wrapped health failure: %w", err), time.Now()) {
			t.Fatalf("recoverable controller failure was dropped: %v", err)
		}
	}
}
