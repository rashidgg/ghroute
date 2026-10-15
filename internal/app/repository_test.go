package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rashidgg/ghroute/internal/discover"
	"github.com/rashidgg/ghroute/internal/identity"
	"github.com/rashidgg/ghroute/internal/model"
	"github.com/rashidgg/ghroute/internal/parse"
	"github.com/rashidgg/ghroute/internal/process"
)

func TestIdentityUsesOriginRegardlessOfActiveOrBaseAccount(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	git := identity.Git{Runner: process.ExecRunner{}, Dir: dir, Env: append(os.Environ(), "GIT_CONFIG_GLOBAL="+filepath.Join(dir, "global"), "GIT_CONFIG_NOSYSTEM=1")}
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"remote", "add", "origin", "https://github.com/rashidgg/fork"},
		{"remote", "add", "upstream", "https://github.com/Ref-Platform/web-platform"},
		{"config", "remote.upstream.gh-resolved", "base"},
	} {
		if _, err := git.Run(ctx, args...); err != nil {
			t.Fatal(err)
		}
	}
	c := discover.Cache{
		Accounts: []model.Account{{Login: "rashidgg", ID: 45042692}, {Login: "rashidrefs", ID: 342224320, Orgs: []string{"Ref-Platform"}}},
		Profiles: map[string]model.Profile{"rashidgg": {Name: "Origin Name"}, "rashidrefs": {Name: "Upstream Name"}},
	}
	if err := ensureIdentity(ctx, git, &discover.Service{}, c, "rashidrefs", model.Request{}); err != nil {
		t.Fatal(err)
	}
	if email, err := git.LocalEmail(ctx); err != nil || email != "45042692+rashidgg@users.noreply.github.com" {
		t.Fatal("origin account identity was not used")
	}
	if _, err := git.Run(ctx, "config", "--local", "user.email", "keep@example.test"); err != nil {
		t.Fatal(err)
	}
	if err := ensureIdentity(ctx, git, &discover.Service{}, discover.Cache{}, "rashidrefs", model.Request{}); err != nil {
		t.Fatal("existing identity should not need account discovery")
	}
	if email, err := git.LocalEmail(ctx); err != nil || email != "keep@example.test" {
		t.Fatal("existing local identity changed")
	}
}

func TestForkDefersIdentityUntilOriginChanges(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	git := identity.Git{Runner: process.ExecRunner{}, Dir: dir, Env: append(os.Environ(), "GIT_CONFIG_GLOBAL="+filepath.Join(dir, "global"), "GIT_CONFIG_NOSYSTEM=1")}
	for _, args := range [][]string{{"init", "-b", "main"}, {"remote", "add", "origin", "https://github.com/Ref-Platform/web-platform"}} {
		if _, err := git.Run(ctx, args...); err != nil {
			t.Fatal(err)
		}
	}
	c := discover.Cache{Accounts: []model.Account{{Login: "rashidgg", ID: 45042692}, {Login: "rashidrefs", ID: 342224320, Orgs: []string{"Ref-Platform"}}}, Profiles: map[string]model.Profile{}}
	if err := ensureIdentity(ctx, git, &discover.Service{}, c, "rashidgg", model.Request{Fork: true}); err != nil {
		t.Fatal(err)
	}
	if email, err := git.LocalEmail(ctx); err != nil || email != "" {
		t.Fatal("fork command pinned the source identity")
	}
	if _, err := git.Run(ctx, "remote", "set-url", "origin", "https://github.com/rashidgg/web-platform"); err != nil {
		t.Fatal(err)
	}
	if err := ensureIdentity(ctx, git, &discover.Service{}, c, "rashidrefs", model.Request{}); err != nil {
		t.Fatal(err)
	}
	if email, err := git.LocalEmail(ctx); err != nil || email != "45042692+rashidgg@users.noreply.github.com" {
		t.Fatal("completed fork did not get destination identity")
	}
}

func TestContextHostPrecedence(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	git := identity.Git{Runner: process.ExecRunner{}, Dir: dir, Env: append(os.Environ(), "GIT_CONFIG_GLOBAL="+filepath.Join(dir, "global"), "GIT_CONFIG_NOSYSTEM=1")}
	for _, args := range [][]string{{"init", "-b", "main"}, {"remote", "add", "origin", "https://github.com/Ref-Platform/web-platform"}} {
		if _, err := git.Run(ctx, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, tt := range []struct {
		args []string
		host string
	}{
		{[]string{"pr", "list"}, "github.com"},
		{[]string{"repo", "view"}, "github.com"},
		{[]string{"repo", "fork"}, "github.com"},
		{[]string{"api", "user"}, "enterprise.example"},
		{[]string{"pr", "list", "-R", "enterprise.example/Other/Repo"}, "enterprise.example"},
	} {
		r, err := repositoryContext(ctx, git, parse.Parse(tt.args, map[string]string{"GH_HOST": "enterprise.example"}))
		if err != nil || r.Target.Host != tt.host {
			t.Fatalf("wrong context host for %v", tt.args)
		}
	}
	if _, err := git.Run(ctx, "remote", "set-url", "origin", "https://enterprise.example/Other/Repo"); err != nil {
		t.Fatal(err)
	}
	r, err := repositoryContext(ctx, git, parse.Parse([]string{"pr", "list"}, map[string]string{"GH_HOST": "github.com"}))
	if err != nil || r.Target.Host != "enterprise.example" {
		t.Fatal("enterprise checkout host was lost")
	}
	git.Dir = t.TempDir()
	r, err = repositoryContext(ctx, git, parse.Parse([]string{"pr", "list"}, map[string]string{"GH_HOST": "enterprise.example"}))
	if err != nil || r.Target.Host != "enterprise.example" {
		t.Fatal("GH_HOST fallback outside repository was lost")
	}
}
