package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/rashidgg/ghroute/internal/credential"
)

func envMap(env []string) map[string]string {
	m := map[string]string{}
	for _, entry := range env {
		k, v, _ := strings.Cut(entry, "=")
		m[k] = v
	}
	return m
}

func realExec(realGH string, args, env []string) error {
	return syscall.Exec(realGH, append([]string{realGH}, args...), env)
}

func tokenArgs(args []string, login string) []string {
	end := len(args)
	for n, arg := range args {
		if arg == "--" {
			end = n
			break
		}
	}
	out := append([]string{}, args[:end]...)
	out = append(out, "--user", login)
	return append(out, args[end:]...)
}

func replayCredential(ctx context.Context, realGH string, args, env []string, record credential.Record) error {
	cmd := exec.CommandContext(ctx, realGH, args...)
	cmd.Env, cmd.Stdin = env, strings.NewReader(record.Replay())
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			os.Exit(exit.ExitCode())
		}
		return fmt.Errorf("real gh credential helper failed")
	}
	return nil
}
