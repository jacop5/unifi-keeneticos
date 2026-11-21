// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

var ndmPrompt = regexp.MustCompile(`\(config[^\r\n)]*\)>\s*$`)

// Terminal executes a single NDM command in a private controlling terminal.
// The command, including any key, travels through stdin rather than argv.
func Terminal(ctx context.Context, command string) ([]byte, error) {
	if strings.ContainsAny(command, "\r\n") {
		return nil, fmt.Errorf("multiline NDM command refused")
	}
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	master := os.NewFile(uintptr(fd), "ndm-pty")
	defer master.Close()
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		return nil, err
	}
	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		return nil, err
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, err
	}
	defer slave.Close()
	if err := unix.IoctlSetWinsize(fd, unix.TIOCSWINSZ, &unix.Winsize{Row: 24, Col: 160}); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "ndmc")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	slave.Close()
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-ctx.Done():
			master.Close()
		case <-stopped:
		}
	}()
	readPrompt := func() ([]byte, error) {
		var output []byte
		buffer := make([]byte, 4096)
		for len(output) < 1<<20 {
			if err := ctx.Err(); err != nil {
				return output, err
			}
			fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
			ready, err := unix.Poll(fds, 100)
			if err == unix.EINTR {
				continue
			}
			if err != nil {
				return output, err
			}
			if ready == 0 {
				continue
			}
			n, err := unix.Read(fd, buffer)
			if err == unix.EAGAIN {
				continue
			}
			if n < 0 {
				n = 0
			}
			output = append(output, buffer[:n]...)
			if ndmPrompt.MatchString(ansi.ReplaceAllString(string(output), "")) {
				return output, nil
			}
			if err != nil {
				return output, fmt.Errorf("NDM terminal closed before prompt")
			}
		}
		return nil, fmt.Errorf("NDM terminal output exceeded limit")
	}
	if greeting, err := readPrompt(); err != nil {
		return nil, fmt.Errorf("%w (startup: %.240q)", err, greeting)
	}
	if _, err := master.Write([]byte(command + "\n")); err != nil {
		return nil, err
	}
	output, err := readPrompt()
	if err != nil {
		return nil, err
	}
	if ndmError.Match(output) {
		return nil, fmt.Errorf("NDM terminal rejected command")
	}
	return output, nil
}
