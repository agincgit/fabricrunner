//go:build !linux && !darwin

package sandbox

import (
	"context"
	"fmt"
	fr "github.com/agincgit/fabricrunner"
	"runtime"
)

func runNative(context.Context, Config, Command) (Result, error) {
	return Result{}, fmt.Errorf("%w: unsupported platform %s", fr.ErrSandboxDenied, runtime.GOOS)
}
