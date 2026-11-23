// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalDeadlineDoesNotWaitForDescendantOutputPipe(t *testing.T) {
	dir := t.TempDir()
	// Killing the shell leaves sleep holding its inherited output pipe open.
	script := "#!/bin/sh\nprintf 'private-native-output\\n'\nsleep 3 &\nwait\n"
	if err := os.WriteFile(filepath.Join(dir, "ndmc"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	out, err := Local(ctx, "show ip hotspot")
	elapsed := time.Since(start)
	if err == nil || len(out) != 0 || strings.Contains(err.Error(), "private-native-output") {
		t.Fatalf("cancelled output leaked or succeeded: %v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("descendant pipe defeated command deadline: %v", elapsed)
	}
}
