package sandbox_test

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
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
	box, helper := probeBox(t, t.TempDir())
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	result, err := box.Run(context.Background(), sandbox.Command{Args: []string{helper, "network", listener.Addr().String()}, MaxOutput: 4096})
	if err == nil || !strings.Contains(string(result.Output), "connect_denied") {
		t.Fatalf("network not denied as expected: %v %s", err, result.Output)
	}
}
func TestConfinedProcessEscapeFails(t *testing.T) {
	root := t.TempDir()
	box, helper := probeBox(t, root)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	result, err := box.Run(ctx, sandbox.Command{Args: []string{helper, "escape"}, MaxOutput: 1024})
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(string(result.Output), "fork_denied") {
		t.Fatalf("got %v", err)
	}
	time.Sleep(1200 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(root, "escaped")); !os.IsNotExist(err) {
		t.Fatalf("escaped child survived: %v", err)
	}
}

func probeBox(t *testing.T, root string) (sandbox.Sandbox, string) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("native sandbox unsupported on " + runtime.GOOS)
	}
	helper := filepath.Join(t.TempDir(), "probe")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if data, err := exec.CommandContext(ctx, "go", "build", "-o", helper, "./testdata/probe").CombinedOutput(); err != nil {
		t.Fatalf("probe build: %v %s", err, data)
	}
	box, err := sandbox.New().Establish(ctx, sandbox.Config{Root: root, Helper: helper})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := box.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return box, helper
}
