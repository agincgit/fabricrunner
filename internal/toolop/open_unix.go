//go:build linux || darwin

package toolop

import (
	"os"
	"syscall"
)

func openRead(root *os.Root, path string) (*os.File, error) {
	return root.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
