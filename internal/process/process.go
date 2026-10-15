// Package process runs external commands and prepares isolated GitHub token environments.
package process

import (
	"context"
	"os/exec"
	"strings"
)

// RealGH is a stable Homebrew dependency path, overridden by the formula at build time.
var RealGH = "/opt/homebrew/opt/gh/bin/gh"

// Runner keeps discovery and git operations testable without network access.
// Child stderr is deliberately not included in returned errors: it may contain secrets.
type Runner interface {
	Run(context.Context, Command) ([]byte, error)
}

type Command struct {
	Name  string
	Args  []string
	Env   []string
	Dir   string
	Input string
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, c Command) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Env, cmd.Dir = c.Env, c.Dir
	cmd.Stdin = strings.NewReader(c.Input)
	return cmd.Output()
}

// CleanEnv never mutates the parent's environment.
func CleanEnv(env []string) []string {
	var result []string
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN", "GHROUTE_ROUTER":
		default:
			result = append(result, entry)
		}
	}
	return result
}

func TokenEnv(env []string, token string) []string {
	return append(CleanEnv(env), "GH_TOKEN="+token, "GHROUTE_ROUTER=1")
}
