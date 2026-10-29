// Package device defines bounded, policy-checked device session primitives.
// Production drivers supply command policies; there is no permissive default.
package device

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/amb1s1/tracefull/internal/process"
)

// Policy must validate the complete command, parameters and platform-specific
// setup exceptions. A broad "show" prefix is not a sufficient policy.
type Policy func(string) error

func validate(policy Policy, command string) error {
	if policy == nil {
		return fmt.Errorf("device has no command policy")
	}
	if command == "" || len(command) > 4096 || strings.ContainsAny(command, ";&<>$`|") || strings.ContainsFunc(command, unicode.IsControl) {
		return fmt.Errorf("command contains unsupported syntax")
	}
	return policy(command)
}

type Result struct {
	process.Result
	Command            string
	Argv               []string
	StartedAt, EndedAt time.Time
}

type Session interface {
	Run(context.Context, string) (Result, error)
	Close() error
}

// acquire serializes commands per session while allowing a queued caller to cancel.
func acquire(ctx context.Context, gate chan struct{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-gate
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func timeoutValid(timeout time.Duration) error {
	if timeout <= 0 || timeout > 5*time.Minute {
		return fmt.Errorf("session timeout must be positive and at most 5m")
	}
	return nil
}
