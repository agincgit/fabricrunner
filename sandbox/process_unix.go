//go:build linux || darwin

package sandbox

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type boundedBuffer struct {
	mu        sync.Mutex
	data      []byte
	max       int
	truncated bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	left := b.max - len(b.data)
	if len(p) > left {
		p = p[:left]
		b.truncated = true
	}
	b.data = append(b.data, p...)
	return n, nil
}
func runProcess(ctx context.Context, config Config, command Command, path string, args []string) (Result, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=/tmp", "TMPDIR=/tmp", "LANG=C"}
	cmd.Dir = config.Root
	cmd.Stdin = bytes.NewReader(command.Input)
	output := &boundedBuffer{max: command.MaxOutput}
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		return Result{}, err
	}
	err := cmd.Wait()
	// Kill lingering same-group children even on successful parent exit.
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	output.mu.Lock()
	result := Result{Output: append([]byte(nil), output.data...), Truncated: output.truncated, Started: true, ExitCode: cmd.ProcessState.ExitCode()}
	output.mu.Unlock()
	return result, errors.Join(err, ctx.Err())
}
