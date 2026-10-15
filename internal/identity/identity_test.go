package identity

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/rashidgg/ghroute/internal/model"
	"github.com/rashidgg/ghroute/internal/process"
)

func sandbox(t *testing.T) Git {
	t.Helper()
	dir := t.TempDir()
	g := Git{Runner: process.ExecRunner{}, Dir: dir, Env: append(os.Environ(), "GIT_CONFIG_GLOBAL="+filepath.Join(dir, "global"), "GIT_CONFIG_NOSYSTEM=1")}
	if _, err := g.Run(context.Background(), "init", "-b", "main"); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestNoreplyAndRefsFallback(t *testing.T) {
	a := model.Account{Login: "rashidgg", ID: 45042692}
	if Noreply(a) != "45042692+rashidgg@users.noreply.github.com" {
		t.Fatal("incorrect noreply address")
	}
	name, email := Values(model.Account{Login: "rashidrefs", ID: 342224320}, model.Profile{Name: "Rashid Goshtasbi"})
	if name != "Rashid Goshtasbi" || email != "rashid@refs.me" {
		t.Fatal("incorrect Refs fallback")
	}
	_, email = Values(model.Account{Login: "rashidrefs", ID: 342224320}, model.Profile{Email: "public@example.test"})
	if email != "342224320+rashidrefs@users.noreply.github.com" {
		t.Fatal("public email must use specified noreply rule")
	}
}

func TestLocalIdentityPreservedAndSharedWithWorktree(t *testing.T) {
	g := sandbox(t)
	ctx := context.Background()
	a := model.Account{Login: "rashidgg", ID: 45042692}
	if err := g.Apply(ctx, a, model.Profile{Name: "Rashid Goshtasbi"}); err != nil {
		t.Fatal(err)
	}
	if email, err := g.LocalEmail(ctx); err != nil || email != Noreply(a) {
		t.Fatal("local identity not written")
	}
	if _, err := g.Run(ctx, "config", "--local", "user.email", "existing@example.test"); err != nil {
		t.Fatal(err)
	}
	if err := g.Apply(ctx, model.Account{Login: "rashidrefs", ID: 342224320}, model.Profile{Name: "Another Name"}); err != nil {
		t.Fatal(err)
	}
	if email, _ := g.LocalEmail(ctx); email != "existing@example.test" {
		t.Fatal("existing local email overwritten")
	}
	name, err := g.Run(ctx, "config", "--local", "user.name")
	if err != nil || string(name) != "Rashid Goshtasbi\n" {
		t.Fatal("existing name overwritten")
	}
	if _, err := g.Run(ctx, "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "test"); err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(t.TempDir(), "worktree")
	if _, err := g.Run(ctx, "worktree", "add", "-b", "test-worktree", worktree); err != nil {
		t.Fatal(err)
	}
	g.Dir = worktree
	if email, err := g.LocalEmail(ctx); err != nil || email != "existing@example.test" {
		t.Fatal("worktree did not share local identity")
	}
}

func TestRemotePrecedenceAndForkPR(t *testing.T) {
	g := sandbox(t)
	ctx := context.Background()
	for _, r := range []struct{ name, url string }{{"origin", "https://github.com/rashidgg/fork"}, {"upstream", "https://github.com/Ref-Platform/original"}, {"base", "https://github.com/Another/base"}} {
		if _, err := g.Run(ctx, "remote", "add", r.name, r.url); err != nil {
			t.Fatal(err)
		}
	}
	if got := g.BaseRemote(ctx, false); got != "https://github.com/Ref-Platform/original" {
		t.Fatal("upstream must precede origin")
	}
	if _, err := g.Run(ctx, "config", "remote.base.gh-resolved", "base"); err != nil {
		t.Fatal(err)
	}
	if got := g.BaseRemote(ctx, false); got != "https://github.com/Another/base" {
		t.Fatal("configured base must win")
	}
	if got := g.BaseRemote(ctx, true); got != "https://github.com/rashidgg/fork" {
		t.Fatal("PR creation must use fork origin")
	}
	if _, err := g.Run(ctx, "remote", "remove", "base"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Run(ctx, "remote", "remove", "upstream"); err != nil {
		t.Fatal(err)
	}
	if got := g.BaseRemote(ctx, false); got != "https://github.com/rashidgg/fork" {
		t.Fatal("origin fallback missing")
	}
}

func TestConcurrentWorktreeIdentityInitialization(t *testing.T) {
	g := sandbox(t)
	ctx := context.Background()
	if _, err := g.Run(ctx, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "test"); err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(t.TempDir(), "worktree")
	if _, err := g.Run(ctx, "worktree", "add", "-b", "concurrent", worktree); err != nil {
		t.Fatal(err)
	}
	other := g
	other.Dir = worktree
	errors := make(chan error, 8)
	var wg sync.WaitGroup
	for n := 0; n < 8; n++ {
		git := g
		if n%2 == 1 {
			git = other
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			errors <- git.Apply(ctx, model.Account{Login: "rashidgg", ID: 45042692}, model.Profile{Name: "Rashid Goshtasbi"})
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if email, err := g.LocalEmail(ctx); err != nil || email != "45042692+rashidgg@users.noreply.github.com" {
		t.Fatal("concurrent identity initialization failed")
	}
}
