package sandbox_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	fr "github.com/agincgit/fabricrunner"
	"github.com/agincgit/fabricrunner/sandbox"
)

func TestDefaultExecutorDenies(t *testing.T) {
	_, err := (sandbox.Deny{}).Establish(context.Background(), sandbox.Config{Root: t.TempDir()})
	if !errors.Is(err, fr.ErrSandboxDenied) {
		t.Fatalf("got %v", err)
	}
}
func TestUnsupportedPlatformDeniesWithReason(t *testing.T) {
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		t.Skip("supported platform")
	}
	_, err := sandbox.New().Establish(context.Background(), sandbox.Config{Root: t.TempDir()})
	if !errors.Is(err, fr.ErrSandboxDenied) || !strings.Contains(err.Error(), runtime.GOOS) {
		t.Fatalf("got %v", err)
	}
}
func establish(t *testing.T, root string) sandbox.Sandbox {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("no native sandbox on " + runtime.GOOS)
	}
	box, err := sandbox.New().Establish(context.Background(), sandbox.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := box.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return box
}
func TestSandboxEstablishmentFailureDenies(t *testing.T) {
	_, err := sandbox.New().Establish(context.Background(), sandbox.Config{Root: filepath.Join(t.TempDir(), "absent")})
	if !errors.Is(err, fr.ErrSandboxDenied) {
		t.Fatalf("got %v", err)
	}
}
func TestConfinedWriteOutsideRootFails(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "escape")
	box := establish(t, root)
	_, err := box.Run(context.Background(), sandbox.Command{Args: []string{"/usr/bin/touch", outside}, MaxOutput: 1024})
	if err == nil {
		t.Fatal("outside write succeeded")
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("outside file: %v", err)
	}
	result, err := box.Run(context.Background(), sandbox.Command{Args: []string{"/usr/bin/touch", "inside"}, MaxOutput: 1024})
	if err != nil {
		t.Fatalf("inside write failed: %v %s", err, result.Output)
	}
	if _, err := os.Stat(filepath.Join(root, "inside")); err != nil {
		t.Fatal(err)
	}
}
func TestConfinedNetworkCallFails(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux Python network probe; macOS evidence remains separate")
	}
	box := establish(t, t.TempDir())
	result, err := box.Run(context.Background(), sandbox.Command{Args: []string{"/usr/bin/python3", "-c", "import socket; s=socket.socket(); s.settimeout(.2); s.connect(('1.1.1.1',443))"}, MaxOutput: 4096})
	if err == nil || !strings.Contains(string(result.Output), "Network is unreachable") {
		t.Fatalf("network not denied as expected: %v %s", err, result.Output)
	}
}
func TestConfinedProcessEscapeFails(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux PID namespace probe; macOS evidence remains separate")
	}
	root := t.TempDir()
	box := establish(t, root)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	code := "import os,time; p=os.fork(); os.setsid() if p==0 else None; time.sleep(.7); open('escaped','w').write('survived')"
	_, err := box.Run(ctx, sandbox.Command{Args: []string{"/usr/bin/python3", "-c", code}, MaxOutput: 1024})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
	time.Sleep(time.Second)
	if _, err := os.Stat(filepath.Join(root, "escaped")); !os.IsNotExist(err) {
		t.Fatalf("escaped child survived: %v", err)
	}
}
