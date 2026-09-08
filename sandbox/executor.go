// Package sandbox provides fail-closed OS confinement for side-effecting tools.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	fr "github.com/agincgit/fabricrunner"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

type Config struct {
	Root   string
	Helper string
}
type Command struct {
	Args      []string
	Input     []byte
	MaxOutput int
}
type Result struct {
	Output    []byte
	Truncated bool
	Started   bool
	ExitCode  int
}
type Sandbox interface {
	Capability() fr.SandboxCapability
	Run(context.Context, Command) (Result, error)
	Close(context.Context) error
}
type Executor interface {
	Establish(context.Context, Config) (Sandbox, error)
}
type Deny struct{}

func (Deny) Establish(ctx context.Context, _ Config) (Sandbox, error) {
	err := fr.RecordSandboxEvent(ctx, fr.SandboxEvent{Phase: "denied", Backend: "none"})
	return nil, errors.Join(fr.ErrSandboxDenied, err)
}

type native struct{}

func New() Executor { return native{} }

type confinement struct {
	config  Config
	backend string
	mu      sync.Mutex
	closed  bool
	cancel  context.CancelFunc
	active  sync.WaitGroup
	ctx     context.Context
}

func (native) Establish(ctx context.Context, config Config) (Sandbox, error) {
	backend := "unsupported:" + runtime.GOOS
	if runtime.GOOS == "linux" {
		backend = "bubblewrap"
	}
	if runtime.GOOS == "darwin" {
		backend = "seatbelt"
	}
	deny := func(cause error) (Sandbox, error) {
		err := fr.RecordSandboxEvent(ctx, fr.SandboxEvent{Phase: "denied", Backend: backend})
		return nil, errors.Join(fmt.Errorf("%w (%s): %w", fr.ErrSandboxDenied, backend, cause), err)
	}
	if err := ctx.Err(); err != nil {
		return deny(err)
	}
	root, err := filepath.Abs(config.Root)
	if err != nil || config.Root == "" {
		return deny(errors.New("explicit root required"))
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return deny(err)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return deny(errors.New("root is not a directory"))
	}
	config.Root = root
	if config.Helper != "" {
		helper, err := filepath.Abs(config.Helper)
		if err != nil {
			return deny(err)
		}
		helper, err = filepath.EvalSymlinks(helper)
		if err != nil {
			return deny(err)
		}
		info, err := os.Stat(helper)
		if err != nil || !info.Mode().IsRegular() {
			return deny(errors.New("helper is not a regular file"))
		}
		rel, err := filepath.Rel(root, helper)
		if err == nil && filepath.IsLocal(rel) {
			return deny(errors.New("trusted helper must be outside writable root"))
		}
		config.Helper = helper
	}
	lifetime, cancel := context.WithCancel(context.WithoutCancel(ctx))
	box := &confinement{config: config, backend: backend, cancel: cancel, ctx: lifetime}
	probeCtx, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()
	result, err := box.Run(probeCtx, Command{Args: []string{"/usr/bin/true"}, MaxOutput: 1024})
	if err != nil {
		cancel()
		return deny(fmt.Errorf("probe failed: %w (%s)", err, result.Output))
	}
	if err := fr.RecordSandboxEvent(ctx, fr.SandboxEvent{Phase: "established", Backend: backend}); err != nil {
		cancel()
		return nil, err
	}
	return box, nil
}
func (b *confinement) Capability() fr.SandboxCapability {
	b.mu.Lock()
	defer b.mu.Unlock()
	return fr.SandboxCapability{Established: !b.closed, Backend: b.backend}
}
func (b *confinement) Run(ctx context.Context, command Command) (Result, error) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return Result{}, fr.ErrSandboxDenied
	}
	b.active.Add(1)
	b.mu.Unlock()
	defer b.active.Done()
	if len(command.Args) == 0 || command.Args[0] == "" {
		return Result{}, errors.New("argument vector is required")
	}
	if command.MaxOutput <= 0 || command.MaxOutput > 16<<20 {
		return Result{}, errors.New("output bound must be between 1 and 16 MiB")
	}
	if len(command.Input) > 16<<20 {
		return Result{}, errors.New("input exceeds 16 MiB")
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(b.ctx, cancel)
	defer stop()
	if err := runCtx.Err(); err != nil {
		return Result{}, err
	}
	return runNative(runCtx, b.config, command)
}
func (b *confinement) Close(ctx context.Context) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	b.cancel()
	b.mu.Unlock()
	done := make(chan struct{})
	go func() { b.active.Wait(); close(done) }()
	select {
	case <-done:
		return fr.RecordSandboxEvent(ctx, fr.SandboxEvent{Phase: "teardown", Backend: b.backend})
	case <-ctx.Done():
		return ctx.Err()
	}
}
