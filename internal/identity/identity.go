package identity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/rashidgg/ghroute/internal/model"
	"github.com/rashidgg/ghroute/internal/process"
)

func Noreply(a model.Account) string {
	return strconv.FormatInt(a.ID, 10) + "+" + a.Login + "@users.noreply.github.com"
}

func Values(a model.Account, p model.Profile) (string, string) {
	name := p.Name
	if name == "" {
		name = a.Login
	}
	email := Noreply(a)
	if a.Login == "rashidrefs" && p.Email == "" {
		email = "rashid@refs.me"
	}
	return name, email
}

type Git struct {
	Runner process.Runner
	Dir    string
	Env    []string
}

func (g Git) Run(ctx context.Context, args ...string) ([]byte, error) {
	return g.Runner.Run(ctx, process.Command{Name: "git", Args: args, Dir: g.Dir, Env: g.Env})
}

func (g Git) LocalEmail(ctx context.Context) (string, error) {
	b, err := g.Run(ctx, "config", "--local", "--get", "user.email")
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return "", fmt.Errorf("cannot read local git user.email")
		}
	}
	return strings.TrimSpace(string(b)), nil
}

func (g Git) Apply(ctx context.Context, a model.Account, p model.Profile) error {
	// Worktrees have different working directories but share this lock and config.
	// Git serializes individual writes; flock serializes our read/name/email sequence.
	b, err := g.Run(ctx, "rev-parse", "--git-common-dir")
	if err != nil {
		return fmt.Errorf("cannot find common git directory")
	}
	common := strings.TrimSpace(string(b))
	if !filepath.IsAbs(common) {
		common = filepath.Join(g.Dir, common)
	}
	f, err := os.OpenFile(filepath.Join(common, "ghroute-identity.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("cannot lock local git identity")
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("cannot lock local git identity")
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	email, err := g.LocalEmail(ctx)
	if err != nil {
		return err
	}
	if email != "" {
		return nil
	}
	name, email := Values(a, p)
	if _, err := g.Run(ctx, "config", "--local", "user.name", name); err != nil {
		return fmt.Errorf("cannot set local git user.name")
	}
	if _, err := g.Run(ctx, "config", "--local", "user.email", email); err != nil {
		return fmt.Errorf("cannot set local git user.email")
	}
	return nil
}

func (g Git) Remote(ctx context.Context, name string) string {
	b, err := g.Run(ctx, "remote", "get-url", name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// BaseRemote follows gh's configured base, upstream, then origin. PR creation
// deliberately uses origin so the selected account can push a fork's branch.
func (g Git) BaseRemote(ctx context.Context, prCreate bool) string {
	if prCreate {
		if origin := g.Remote(ctx, "origin"); origin != "" {
			return origin
		}
	}
	b, err := g.Run(ctx, "remote")
	if err != nil {
		return ""
	}
	for _, name := range strings.Fields(string(b)) {
		value, err := g.Run(ctx, "config", "--get", "remote."+name+".gh-resolved")
		if err == nil && strings.TrimSpace(string(value)) == "base" {
			return g.Remote(ctx, name)
		}
	}
	for _, name := range []string{"upstream", "origin"} {
		if u := g.Remote(ctx, name); u != "" {
			return u
		}
	}
	return ""
}
