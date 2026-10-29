//go:build windows

package ui

import (
	"context"
	"io"
	"os"
)

// Windows uses the OS file close cancellation path; runtime validation belongs
// to the Windows CI job and has not been claimed from cross-compilation.
func Output(ctx context.Context, f *os.File) (io.Writer, func() error, error) {
	closed := make(chan error, 1)
	stop := context.AfterFunc(ctx, func() { closed <- f.Close() })
	return f, func() error {
		if stop() {
			return nil
		}
		return <-closed
	}, nil
}
