// Package toolop implements the filesystem helper protocol inside confinement.
package toolop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const MaxInput = 8 << 20

type Input struct {
	Path string   `json:"path"`
	Text string   `json:"text"`
	Old  string   `json:"old"`
	New  string   `json:"new"`
	Args []string `json:"args"`
}
type Result struct {
	Text           string `json:"text"`
	Truncated      bool   `json:"truncated"`
	PartialEffects bool   `json:"partial_effects"`
	ExitCode       int    `json:"exit_code"`
	Error          string `json:"error,omitempty"`
}

func Decode(data []byte) (Input, error) {
	var input Input
	if len(data) > MaxInput {
		return input, errors.New("tool input exceeds 8 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return input, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return input, errors.New("input must contain exactly one JSON object")
	}
	return input, nil
}
func ValidatePath(path string) error {
	// Backslashes and colons are refused on every platform to avoid ambiguous
	// Windows paths, alternate data streams, and cross-platform reinterpretation.
	if !filepath.IsLocal(path) || strings.ContainsAny(path, "\\:\x00") {
		return errors.New("path must be local to the tool root")
	}
	return nil
}
func Run(ctx context.Context, root, operation string, input Input, maxOutput int) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if err := ValidatePath(input.Path); err != nil {
		return Result{}, err
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return Result{}, err
	}
	defer r.Close()
	result := Result{}
	switch operation {
	case "read":
		f, err := openRead(r, input.Path)
		if err != nil {
			return result, err
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return result, err
		}
		if !info.Mode().IsRegular() {
			return result, errors.New("only regular files may be read")
		}
		data, err := io.ReadAll(io.LimitReader(f, int64(maxOutput)+1))
		if err != nil {
			return result, err
		}
		if len(data) > maxOutput {
			data = data[:maxOutput]
			result.Truncated = true
		}
		result.Text = string(data)
		return result, ctx.Err()
	case "write", "edit":
		if info, err := r.Lstat(input.Path); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return result, errors.New("write and edit refuse symlink targets")
		}
		if _, err := r.Stat(input.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return result, err
		}
		text := input.Text
		if operation == "edit" {
			if input.Old == "" {
				return result, errors.New("edit match must not be empty")
			}
			f, err := openRead(r, input.Path)
			if err != nil {
				return result, err
			}
			info, err := f.Stat()
			if err != nil {
				f.Close()
				return result, err
			}
			if !info.Mode().IsRegular() {
				f.Close()
				return result, errors.New("only regular files may be edited")
			}
			data, err := io.ReadAll(io.LimitReader(f, MaxInput+1))
			f.Close()
			if err != nil {
				return result, err
			}
			if len(data) > MaxInput {
				return result, errors.New("file exceeds edit bound")
			}
			if strings.Count(string(data), input.Old) != 1 {
				return result, errors.New("edit requires exactly one match")
			}
			text = strings.Replace(string(data), input.Old, input.New, 1)
		}
		if len(text) > MaxInput {
			return result, errors.New("write exceeds 8 MiB")
		}
		if err := ctx.Err(); err != nil {
			return result, err
		}
		// A private temporary file plus rename avoids partial replacement and avoids
		// writing through hard links or symlinks. The parent path is resolved by Root.
		parent := filepath.Dir(input.Path)
		var f *os.File
		var temp string
		for i := 0; i < 10; i++ {
			temp = filepath.Join(parent, ".fabricrunner-"+randomName())
			f, err = r.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if !errors.Is(err, os.ErrExist) {
				break
			}
		}
		if err != nil {
			return result, err
		}
		defer r.Remove(temp)
		_, writeErr := io.WriteString(f, text)
		syncErr := f.Sync()
		closeErr := f.Close()
		if err := errors.Join(writeErr, syncErr, closeErr, ctx.Err()); err != nil {
			return result, err
		}
		if err := r.Rename(temp, input.Path); err != nil {
			return result, err
		}
		result.Text = "ok"
		result.PartialEffects = true
		return result, ctx.Err()
	default:
		return result, errors.New("unknown filesystem operation")
	}
}
