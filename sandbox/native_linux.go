package sandbox

import (
	"context"
	"os"
	"os/exec"
)

func runNative(ctx context.Context, config Config, command Command) (Result, error) {
	path, err := exec.LookPath("bwrap")
	if err != nil {
		return Result{}, err
	}
	args := []string{"--die-with-parent", "--new-session", "--unshare-all", "--cap-drop", "ALL", "--clearenv"}
	for _, dir := range []string{"/usr", "/bin", "/lib", "/lib64"} {
		if _, err := os.Stat(dir); err == nil {
			args = append(args, "--ro-bind", dir, dir)
		}
	}
	if _, err := os.Stat("/etc/ld.so.cache"); err == nil {
		args = append(args, "--ro-bind", "/etc/ld.so.cache", "/etc/ld.so.cache")
	}
	args = append(args, "--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp", "--bind", config.Root, "/workspace", "--chdir", "/workspace", "--setenv", "PATH", "/usr/bin:/bin", "--setenv", "HOME", "/tmp", "--setenv", "TMPDIR", "/tmp", "--setenv", "LANG", "C")
	if config.Helper != "" {
		args = append(args, "--ro-bind", config.Helper, "/runner-helper")
	}
	argv := append([]string(nil), command.Args...)
	if argv[0] == config.Helper && config.Helper != "" {
		argv[0] = "/runner-helper"
	}
	args = append(args, "--")
	args = append(args, argv...)
	return runProcess(ctx, config, command, path, args)
}
