// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/antoniocosta/unifi-openwrt/internal/keenetic"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func journalID(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func armWatchdog(path string) error {
	id, err := journalID(path)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(executable, "-rollback-watch", path, "-rollback-id", id)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Reap the child while the daemon is alive; it continues independently
	// after the daemon exits because it owns a separate session.
	go func() { _ = cmd.Wait() }()
	return nil
}

func watchRollback(path, id string) error {
	if len(id) != 64 {
		return fmt.Errorf("invalid rollback identity")
	}
	time.Sleep(65 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return keenetic.RecoverUndoMatching(ctx, path, id, keenetic.Local)
}
