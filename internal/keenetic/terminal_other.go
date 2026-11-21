//go:build !linux

// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"fmt"
)

func Terminal(context.Context, string) ([]byte, error) {
	return nil, fmt.Errorf("NDM terminal execution requires Linux")
}
