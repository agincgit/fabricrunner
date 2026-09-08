// fabricrunner-tool is a trusted filesystem helper. Invoke through tool.Config
// and a sandbox executor, not directly with untrusted inputs on the host.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/agincgit/fabricrunner/internal/toolop"
	"io"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "one operation is required")
		os.Exit(2)
	}
	data, err := io.ReadAll(io.LimitReader(os.Stdin, toolop.MaxInput+1))
	if err != nil {
		os.Exit(2)
	}
	input, err := toolop.Decode(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid input")
		os.Exit(2)
	}
	result, err := toolop.Run(context.Background(), ".", os.Args[1], input, 64<<10)
	if err != nil {
		result.Error = "filesystem operation refused"
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		os.Exit(2)
	}
	if err != nil {
		os.Exit(1)
	}
}
