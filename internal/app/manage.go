package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rashidgg/ghroute/internal/discover"
	"github.com/rashidgg/ghroute/internal/install"
	"github.com/rashidgg/ghroute/internal/process"
)

// Version is supplied by release builds; source builds identify as dev.
var Version = "dev"

const usage = `ghroute — the right GitHub account for each repository

Usage:
  ghroute on [--shell fish|zsh|bash|sh] [--no-path]
  ghroute off
  ghroute uninstall
  ghroute refresh
  ghroute --version

On activates the gh shim, GitHub HTTPS credentials, and shell PATH setup.
Off restores normal gh behavior and previous Git helpers, keeping ghroute available.
Uninstall also removes ghroute's shim and PATH entries. Install is an alias for on.
Invoke gh as usual after opening a new terminal.`

func manage(ctx context.Context, home, self string, args, env []string, runner process.Runner) error {
	if len(args) == 0 {
		fmt.Println(usage)
		return nil
	}
	switch args[0] {
	case "help", "--help", "-h":
		fmt.Println(usage)
		return nil
	case "--version", "version":
		fmt.Println("ghroute " + Version)
		return nil
	case "on", "install", "setup":
		flags := flag.NewFlagSet("ghroute "+args[0], flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		shell := flags.String("shell", envMap(env)["SHELL"], "shell whose startup files should be updated")
		noPath := flags.Bool("no-path", false, "leave shell startup files unchanged")
		if err := flags.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if flags.NArg() != 0 {
			return fmt.Errorf("unexpected activation arguments; use ghroute %s --help", args[0])
		}
		p := install.DefaultPaths(home)
		realGH, err := process.FindGH(env, self)
		if err != nil {
			return err
		}
		p.RealGH = realGH
		i := install.Installer{Runner: runner, Env: env, Executable: self, PackageBinary: install.ManagedBinary, Plan: install.NewPlan(p)}
		if i.PackageBinary == "self" {
			i.PackageBinary, err = filepath.EvalSymlinks(self)
			if err != nil {
				return err
			}
		}
		if !*noPath {
			i.PathEdits, err = install.ShellPaths(home, *shell, envMap(env))
			if err != nil {
				return err
			}
		}
		apply := i.Apply
		if args[0] == "setup" {
			apply = i.Setup
		}
		if err := apply(ctx); err != nil {
			return err
		}
		disabled, err := install.RoutingDisabled(p.State)
		if err != nil {
			return err
		}
		if disabled {
			fmt.Println("ghroute is installed. Routing remains off; run ghroute on to enable it.")
		} else {
			fmt.Println("ghroute is on. Router:", p.Binary)
		}
		if *noPath {
			fmt.Println("Put ~/.local/bin first on PATH to use the gh shim.")
		} else {
			fmt.Println("Added ~/.local/bin to your shell startup files. Open a new terminal to activate PATH.")
		}
		return nil
	case "off", "uninstall":
		if len(args) != 1 {
			return fmt.Errorf("usage: ghroute %s", args[0])
		}
		i := install.Installer{Runner: runner, Env: env, Plan: install.NewPlan(install.DefaultPaths(home))}
		if args[0] == "off" {
			if err := i.Off(ctx); err != nil {
				return err
			}
			fmt.Println("ghroute is off. Commands use the official gh and your previous Git credential helpers.")
			fmt.Println("Run ghroute on to enable routing again.")
		} else {
			if err := i.Restore(ctx); err != nil {
				return err
			}
			fmt.Println("Removed ghroute routing, its credential helper, and its managed PATH entries. Open a new terminal.")
		}
		return nil
	case "refresh":
		if len(args) != 1 {
			return fmt.Errorf("usage: ghroute refresh")
		}
		realGH, err := process.FindGH(env, self)
		if err != nil {
			return err
		}
		s := &discover.Service{Runner: runner, GH: realGH, Env: env, Path: filepath.Join(home, ".cache/ghroute/cache.json")}
		c, _, err := s.Load(ctx, true)
		if err != nil {
			return err
		}
		fmt.Printf("Refreshed %d GitHub.com accounts\n", len(c.Accounts))
		return nil
	default:
		return fmt.Errorf("unknown ghroute command %q; use ghroute --help", args[0])
	}
}
