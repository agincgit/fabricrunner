// This executable attempts forbidden actions for native confinement acceptance.
package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	switch os.Args[1] {
	case "partial":
		if err := os.WriteFile("partial", []byte("effect"), 0600); err != nil {
			os.Exit(1)
		}
		time.Sleep(10 * time.Second)
	case "network":
		connection, err := net.DialTimeout("tcp", os.Args[2], 300*time.Millisecond)
		if err != nil {
			fmt.Println("connect_denied")
			os.Exit(1)
		}
		connection.Close()
	case "escape":
		executable, err := os.Executable()
		if err != nil {
			os.Exit(2)
		}
		child := exec.Command(executable, "child")
		child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := child.Start(); err != nil {
			fmt.Println("fork_denied")
			os.Exit(1)
		}
		fmt.Println("child_started")
		time.Sleep(10 * time.Second)
	case "child":
		time.Sleep(time.Second)
		if err := os.WriteFile("escaped", []byte("survived"), 0600); err != nil {
			os.Exit(1)
		}
	default:
		os.Exit(2)
	}
}
