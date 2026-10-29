//go:build darwin || linux

package ui

import (
	"context"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// Output gives Go's poller ownership of a duplicate stdout descriptor. Unlike the
// inherited blocking descriptor, closing this file interrupts a full pipe write.
// Restoring status flags matters because dup shares the open-file description.
func Output(ctx context.Context, original *os.File) (io.Writer, func() error, error) {
	fd := int(original.Fd())
	flags, e := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	if e != nil {
		return nil, nil, e
	}
	dup, e := unix.Dup(fd)
	if e != nil {
		return nil, nil, e
	}
	if e = unix.SetNonblock(dup, true); e != nil {
		_ = unix.Close(dup)
		return nil, nil, e
	}
	f := os.NewFile(uintptr(dup), "tracefull stdout")
	closed := make(chan error, 1)
	stop := context.AfterFunc(ctx, func() { closed <- f.Close() })
	cleanup := func() error {
		var closeErr error
		if stop() {
			closeErr = f.Close()
		} else {
			closeErr = <-closed
		}
		_, restoreErr := unix.FcntlInt(uintptr(fd), unix.F_SETFL, flags)
		return errors.Join(closeErr, restoreErr)
	}
	return f, cleanup, nil
}
