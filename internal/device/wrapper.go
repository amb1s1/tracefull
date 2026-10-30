package device

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/amb1s1/tracefull/internal/process"
	"github.com/mattn/go-shellwords"
)

// Target values are substituted after tokenization and never reparsed.
type Target struct{ Host, Address, User, Jump, Platform string }

type Wrapper struct {
	words   []string
	target  Target
	policy  Policy
	timeout time.Duration
	gate    chan struct{}
	ctx     context.Context
	cancel  context.CancelFunc
	close   sync.Once
}

var placeholder = regexp.MustCompile(`\{[a-z]+\}`)
var targetWord = regexp.MustCompile(`^[A-Za-z0-9_\[][A-Za-z0-9_.:@%/\[\]-]*$`)

func NewWrapper(template string, target Target, policy Policy, timeout time.Duration) (*Wrapper, error) {
	if err := timeoutValid(timeout); err != nil {
		return nil, err
	}
	if policy == nil {
		return nil, fmt.Errorf("device has no command policy")
	}
	if strings.ContainsAny(template, ";&<>$`|") || strings.ContainsFunc(template, unicode.IsControl) {
		return nil, fmt.Errorf("wrapper must be argv, without shell syntax or expansion")
	}
	// Explicit fields avoid the dependency's mutable global parsing defaults.
	p := &shellwords.Parser{ParseEnv: false, ParseBacktick: false, ParseComment: false}
	words, err := p.Parse(template)
	if err != nil || len(words) == 0 {
		return nil, fmt.Errorf("invalid wrapper template: %v", err)
	}
	if strings.ContainsAny(words[0], "{}") {
		return nil, fmt.Errorf("wrapper executable must be literal")
	}
	switch filepath.Base(words[0]) {
	case "sh", "bash", "zsh", "fish", "dash", "ksh", "csh", "tcsh", "cmd", "powershell", "pwsh":
		return nil, fmt.Errorf("wrapper must invoke an executable directly, not a shell")
	}
	cmds := 0
	for _, word := range words {
		if strings.Contains(word, "{cmd}") {
			if word != "{cmd}" {
				return nil, fmt.Errorf("{cmd} must occupy one complete argument")
			}
			cmds++
		}
	}
	if cmds != 1 {
		return nil, fmt.Errorf("wrapper requires exactly one {cmd} argument")
	}
	w := &Wrapper{words: words, target: target, policy: policy, timeout: timeout, gate: make(chan struct{}, 1)}
	if _, err := w.expand("validation-command"); err != nil {
		return nil, err
	}
	path, err := exec.LookPath(words[0])
	if err != nil {
		return nil, fmt.Errorf("wrapper executable: %w", err)
	}
	words[0], err = filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	w.ctx, w.cancel = context.WithCancel(context.Background())
	return w, nil
}

func (w *Wrapper) expand(command string) ([]string, error) {
	values := map[string]string{"{host}": w.target.Host, "{address}": w.target.Address, "{user}": w.target.User, "{jump}": w.target.Jump, "{platform}": w.target.Platform}
	argv := make([]string, len(w.words))
	for i, word := range w.words {
		if word == "{cmd}" {
			argv[i] = command
			continue
		}
		for _, key := range placeholder.FindAllString(word, -1) {
			value, ok := values[key]
			if !ok || !targetWord.MatchString(value) {
				return nil, fmt.Errorf("invalid or missing wrapper placeholder %s", key)
			}
			word = strings.ReplaceAll(word, key, value)
		}
		if strings.ContainsAny(word, "{}") || strings.ContainsFunc(word, unicode.IsControl) {
			return nil, fmt.Errorf("invalid wrapper argument")
		}
		argv[i] = word
	}
	return argv, nil
}

// Plan is the same validation/expansion path used immediately before execution.
func (w *Wrapper) Plan(command string) ([]string, error) {
	if err := validate(w.policy, command); err != nil {
		return nil, err
	}
	return w.expand(command)
}

func (w *Wrapper) Run(ctx context.Context, command string) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()
	stop := context.AfterFunc(w.ctx, cancel)
	defer stop()
	if err := acquire(ctx, w.gate); err != nil {
		return Result{}, err
	}
	defer func() { <-w.gate }()
	if w.ctx.Err() != nil {
		return Result{}, fmt.Errorf("wrapper session closed")
	}
	argv, err := w.Plan(command)
	if err != nil {
		return Result{}, err
	}
	r := Result{Command: command, Argv: argv, StartedAt: time.Now().UTC()}
	r.Result = process.Run(ctx, argv[0], argv[1:], os.Environ())
	r.EndedAt = time.Now().UTC()
	return r, nil
}

func (w *Wrapper) Close() error { w.close.Do(w.cancel); return nil }
