// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/antoniocosta/unifi-openwrt/internal/keenetic"
)

type controllerHTTPError struct{ status int }

func (e controllerHTTPError) Error() string { return fmt.Sprintf("HTTP %d", e.status) }

// provisionRetry separates a completed plan from a transiently failed apply.
// New versions bypass backoff; unchanged successful/rejected plans do not
// repeatedly disrupt WLANs. Network/deadline, transient HTTP and missing native
// host-registration preflight failures retry, from one minute up to five minutes.
type provisionRetry struct {
	version string
	at      time.Time
	delay   time.Duration
}

func (p *provisionRetry) ready(version string, now time.Time) bool {
	return version != p.version || (!p.at.IsZero() && !now.Before(p.at))
}

func (p *provisionRetry) complete(version string) {
	p.version, p.at, p.delay = version, time.Time{}, 0
}

func (p *provisionRetry) failed(version string, err error, now time.Time) bool {
	var network net.Error
	var httpErr controllerHTTPError
	transient := errors.As(err, &network) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, keenetic.ErrKnownHostRegistrationMissing) ||
		(errors.As(err, &httpErr) && ((httpErr.status >= 500 && httpErr.status < 600) || httpErr.status == 408 || httpErr.status == 429))
	if !transient {
		p.complete(version)
		return false
	}
	if p.version != version || p.delay == 0 {
		p.delay = time.Minute
	} else {
		p.delay *= 2
		if p.delay > 5*time.Minute {
			p.delay = 5 * time.Minute
		}
	}
	p.version, p.at = version, now.Add(p.delay)
	return true
}
