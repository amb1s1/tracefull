//go:build linux || darwin

package investigate

import (
	"context"
	"os"
	"strings"

	"github.com/amb1s1/tracefull/internal/process"
)

type ProcessResult = process.Result

type Runner interface {
	Run(context.Context, Command) ProcessResult
}
type ProcessRunner struct{}

func (ProcessRunner) Run(ctx context.Context, c Command) ProcessResult {
	env := []string{}
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if key != "LC_ALL" && key != "LANG" && key != "MTR_OPTIONS" && key != "MTR_PACKET" {
			env = append(env, item)
		}
	}
	env = append(env, "LC_ALL=C", "LANG=C")
	return process.Run(ctx, c.Executable, c.Args, env)
}
