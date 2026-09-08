package sandbox

import (
	"context"
	"strconv"
)

func runNative(ctx context.Context, config Config, command Command) (Result, error) {
	// Deny by default. Only the system runtime, helper and workspace are readable;
	// only the workspace is writable. No network permission is granted.
	// Process creation is denied on macOS in this initial profile. This prevents
	// a child from escaping process-group cleanup via setsid/double-fork. Linux
	// permits children because its PID namespace supplies stronger teardown.
	profile := `(version 1)(deny default)(allow process-exec)(allow sysctl-read)(allow file-read-metadata)(allow file-read* (subpath "/usr") (subpath "/bin") (subpath "/System") (subpath "/Library/Apple") (literal "/dev/null") (literal "/dev/urandom") (subpath ` + strconv.Quote(config.Root) + `)) (allow file-write* (subpath ` + strconv.Quote(config.Root) + `))`
	if config.Helper != "" {
		profile += `(allow file-read* (literal ` + strconv.Quote(config.Helper) + `))`
	}
	args := append([]string{"-p", profile, "--"}, command.Args...)
	return runProcess(ctx, config, command, "/usr/bin/sandbox-exec", args)
}
