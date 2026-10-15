// Package app connects command parsing, account selection, and the real gh process.
package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rashidgg/ghroute/internal/credential"
	"github.com/rashidgg/ghroute/internal/discover"
	"github.com/rashidgg/ghroute/internal/identity"
	"github.com/rashidgg/ghroute/internal/install"
	"github.com/rashidgg/ghroute/internal/model"
	"github.com/rashidgg/ghroute/internal/parse"
	"github.com/rashidgg/ghroute/internal/process"
	"github.com/rashidgg/ghroute/internal/route"
)

// Run dispatches ghroute commands or routes an invocation made through the gh symlink.
func Run() error {
	ctx := context.Background()
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	env, args := os.Environ(), os.Args[1:]
	runner := process.ExecRunner{}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if filepath.Base(os.Args[0]) != "gh" {
		if len(args) > 0 && args[0] == "credential" {
			if len(args) != 2 {
				return fmt.Errorf("usage: ghroute credential get|store|erase")
			}
			args = []string{"auth", "git-credential", args[1]}
		} else {
			return manage(ctx, home, self, args, env, runner)
		}
	}
	realGH, err := process.FindGH(env, self)
	if err != nil {
		return err
	}
	disabled, err := install.RoutingDisabled(install.DefaultPaths(home).State)
	if err != nil {
		return err
	}
	if disabled {
		return realExec(realGH, args, env)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	s := &discover.Service{Runner: runner, GH: realGH, Env: env, Path: filepath.Join(home, ".cache/ghroute/cache.json")}
	r := parse.Parse(args, envMap(env))
	// Pass through auth management untouched, including credential store/erase stdin.
	if r.Passthrough {
		return realExec(realGH, args, env)
	}
	if r.TokenUser != "" {
		// An explicitly named stored token must not depend on unrelated accounts,
		// organization discovery, or the checkout's commit identity.
		if r.Target.Host == "" || r.Target.Host == "github.com" {
			return realExec(realGH, args, process.CleanEnv(env))
		}
		return realExec(realGH, args, env)
	}
	var record credential.Record
	if r.CredentialOp == "get" {
		record, err = credential.Read(os.Stdin)
		if err != nil {
			return err
		}
		r.CredentialURL, r.Target, r.TargetExplicit = record.URL(), record.Target(), true
		if r.Target.Host == "" {
			return fmt.Errorf("credential request has no host")
		}
		if r.Target.Host != "github.com" {
			return replayCredential(ctx, realGH, args, env, record)
		}
		if r.Target.Owner == "" || r.Target.Repo == "" {
			return fmt.Errorf("GitHub credential request needs owner/repo; set credential.https://github.com.useHttpPath=true")
		}
	}
	git := identity.Git{Runner: runner, Dir: cwd, Env: env}
	r, err = repositoryContext(ctx, git, r)
	if err != nil {
		return err
	}
	if d := route.Decide(r, nil, ""); d.Kind == model.Passthrough {
		return realExec(realGH, args, env)
	}
	c, active, err := s.Load(ctx, false)
	if err != nil {
		return err
	}
	a, err := selectAccount(ctx, s, c, active, r)
	if err != nil {
		return err
	}
	token, err := route.RequireToken(a, func(login string) (string, error) { return s.Token(ctx, login) })
	if err != nil {
		return err
	}
	if err := ensureIdentity(ctx, git, s, c, active, r); err != nil {
		return err
	}
	if r.CredentialOp == "get" {
		return credential.Write(os.Stdout, token)
	}
	cmd, sub := parse.Command(args)
	if cmd == "auth" && sub == "token" {
		// gh auth token reads stored credentials, so explicitly name our selection.
		if r.TokenUser == "" {
			args = tokenArgs(args, a.Login)
		}
		return realExec(realGH, args, process.CleanEnv(env))
	}
	return realExec(realGH, args, process.TokenEnv(env, token))
}
