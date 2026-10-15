package parse

import (
	"reflect"
	"testing"

	"github.com/rashidgg/ghroute/internal/model"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name                  string
		args                  []string
		env                   map[string]string
		target                model.Target
		explicit, pass, fork  bool
		org, user, op, remote string
	}{
		{name: "repo before command", args: []string{"-R", "Owner/Repo", "pr", "list"}, target: model.Target{Host: "github.com", Owner: "Owner", Repo: "Repo"}, explicit: true},
		{name: "repo equals", args: []string{"pr", "list", "--repo=Owner/Repo"}, target: model.Target{Host: "github.com", Owner: "Owner", Repo: "Repo"}, explicit: true},
		{name: "repo compact", args: []string{"pr", "list", "-ROwner/Repo"}, target: model.Target{Host: "github.com", Owner: "Owner", Repo: "Repo"}, explicit: true},
		{name: "environment", args: []string{"pr", "list"}, env: map[string]string{"GH_REPO": "Env/Repo"}, target: model.Target{Host: "github.com", Owner: "Env", Repo: "Repo"}, explicit: true},
		{name: "flag before environment", args: []string{"--repo=Flag/Repo", "pr", "list"}, env: map[string]string{"GH_REPO": "Env/Repo"}, target: model.Target{Host: "github.com", Owner: "Flag", Repo: "Repo"}, explicit: true},
		{name: "body ignored", args: []string{"pr", "create", "--body", "https://github.com/Other/Repo"}},
		{name: "body repo flag ignored", args: []string{"pr", "create", "--body", "--repo=Other/Repo"}},
		{name: "separator ignored", args: []string{"pr", "create", "--", "--repo=Other/Repo", "https://github.com/Other/Repo"}},
		{name: "clone", args: []string{"repo", "clone", "Owner/Repo"}, target: model.Target{Host: "github.com", Owner: "Owner", Repo: "Repo"}, explicit: true},
		{name: "clone source after description", args: []string{"repo", "create", "--description", "Other/Repo", "Owner/New"}, target: model.Target{Host: "github.com", Owner: "Owner", Repo: "New"}, explicit: true},
		{name: "create without owner", args: []string{"repo", "create", "New"}, target: model.Target{Host: "github.com", Repo: "New"}, explicit: true},
		{name: "view", args: []string{"repo", "view", "https://github.com/Owner/Repo"}, target: model.Target{Host: "github.com", Owner: "Owner", Repo: "Repo"}, explicit: true},
		{name: "sync", args: []string{"repo", "sync", "Owner/Repo"}, target: model.Target{Host: "github.com", Owner: "Owner", Repo: "Repo"}, explicit: true},
		{name: "delete", args: []string{"repo", "delete", "Owner/Repo"}, target: model.Target{Host: "github.com", Owner: "Owner", Repo: "Repo"}, explicit: true},
		{name: "rename repository", args: []string{"repo", "rename", "Owner/Repo"}, target: model.Target{Host: "github.com", Owner: "Owner", Repo: "Repo"}, explicit: true},
		{name: "rename new name is not owner", args: []string{"repo", "rename", "New"}},
		{name: "list", args: []string{"repo", "list", "Owner"}, target: model.Target{Host: "github.com", Owner: "Owner"}, explicit: true},
		{name: "fork source", args: []string{"repo", "fork", "Ref-Platform/web-platform"}, target: model.Target{Host: "github.com"}, fork: true, remote: "Ref-Platform/web-platform"},
		{name: "fork org", args: []string{"repo", "fork", "--org", "SomeOrg", "Other/Repo"}, target: model.Target{Host: "github.com"}, fork: true, org: "SomeOrg", remote: "Other/Repo"},
		{name: "fork remote boolean", args: []string{"repo", "fork", "--remote", "Other/Repo"}, target: model.Target{Host: "github.com"}, fork: true, remote: "Other/Repo"},
		{name: "api slash", args: []string{"api", "/repos/Owner/name/pulls"}, target: model.Target{Host: "github.com", Owner: "Owner", Repo: "name"}, explicit: true},
		{name: "api no slash", args: []string{"api", "repos/Owner/name"}, target: model.Target{Host: "github.com", Owner: "Owner", Repo: "name"}, explicit: true},
		{name: "api fields ignored", args: []string{"api", "-f", "value=Other/Repo", "/repos/Owner/name"}, target: model.Target{Host: "github.com", Owner: "Owner", Repo: "name"}, explicit: true},
		{name: "status", args: []string{"auth", "status"}, pass: true},
		{name: "token user", args: []string{"auth", "token", "-u", "someone"}, user: "someone"},
		{name: "token user equals", args: []string{"auth", "token", "--user=someone"}, user: "someone"},
		{name: "credential get", args: []string{"auth", "git-credential", "get"}, op: "get"},
		{name: "credential store", args: []string{"auth", "git-credential", "store"}, op: "store", pass: true},
		{name: "credential erase", args: []string{"auth", "git-credential", "erase"}, op: "erase", pass: true},
		{name: "enterprise", args: []string{"api", "repos/Owner/name", "--hostname", "enterprise.test"}, target: model.Target{Host: "enterprise.test", Owner: "Owner", Repo: "name"}, explicit: true},
		{name: "help", args: []string{"pr", "create", "--help"}, pass: true},
		{name: "help body ignored", args: []string{"pr", "create", "--body", "--help"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Parse(tt.args, tt.env)
			if r.Target != tt.target || r.TargetExplicit != tt.explicit || r.Passthrough != tt.pass || r.Fork != tt.fork || r.Org != tt.org || r.TokenUser != tt.user || r.CredentialOp != tt.op || r.RemoteURL != tt.remote {
				t.Fatalf("unexpected routing metadata: %+v", r)
			}
			if !reflect.DeepEqual(r.Args, tt.args) {
				t.Fatal("argv changed")
			}
		})
	}
}

func TestIgnoredValues(t *testing.T) {
	for _, flag := range []string{"--body", "--title", "--message", "-b", "-t", "-m", "--notes", "--comment"} {
		for _, value := range []string{"https://github.com/Wrong/Repo", "Wrong/Repo"} {
			for _, args := range [][]string{{"repo", "view", flag, value}, {"repo", "view", flag + "=" + value}} {
				if r := Parse(args, nil); r.TargetExplicit {
					t.Fatalf("%s value became target", flag)
				}
			}
		}
	}
}

func TestManagementPassthrough(t *testing.T) {
	for _, sub := range []string{"status", "switch", "login", "logout", "refresh", "setup-git", "help"} {
		if !Parse([]string{"auth", sub}, nil).Passthrough {
			t.Fatalf("auth %s must pass through", sub)
		}
	}
	for _, cmd := range []string{"help", "--help", "--version", "completion", "__complete"} {
		if !Parse([]string{cmd}, nil).Passthrough {
			t.Fatalf("%s must pass through", cmd)
		}
	}
}

func TestTargets(t *testing.T) {
	for _, raw := range []string{"git@github.com-rashidrefs:Ref-Platform/web-platform.git", "ssh://git@github.com-rashidrefs/Ref-Platform/web-platform.git", "https://someone@github.com:443/Ref-Platform/web-platform.git", "Ref-Platform/web-platform", "github.com/Ref-Platform/web-platform"} {
		if got := Target(raw); got != (model.Target{Host: "github.com", Owner: "Ref-Platform", Repo: "web-platform"}) {
			t.Fatalf("target %q: %+v", raw, got)
		}
	}
	if got := Target("https://enterprise.test/Owner/Repo"); got.Host != "enterprise.test" {
		t.Fatal("enterprise host lost")
	}
}

func TestCommandFlagArityAndExplicitPrecedence(t *testing.T) {
	for _, args := range [][]string{
		{"repo", "clone", "--upstream-remote-name", "upstream", "Ref-Platform/web-platform"},
		{"repo", "clone", "-u", "upstream", "Ref-Platform/web-platform"},
		{"repo", "view", "Ref-Platform/web-platform"},
		{"repo", "create", "-p", "template/other", "Ref-Platform/web-platform"},
		{"api", "--preview", "some-preview", "repos/Ref-Platform/web-platform"},
		{"api", "-p", "some-preview", "repos/Ref-Platform/web-platform"},
	} {
		r := Parse(args, map[string]string{"GH_REPO": "rashidgg/babyapp"})
		if r.Target != (model.Target{Host: "github.com", Owner: "Ref-Platform", Repo: "web-platform"}) || !r.TargetExplicit {
			t.Fatalf("argv target lost for %v", args)
		}
	}
	for _, args := range [][]string{
		{"repo", "list", "--source", "Ref-Platform"},
		{"repo", "list", "--language", "Go", "Ref-Platform"},
		{"repo", "list", "--topic", "go", "Ref-Platform"},
		{"repo", "list", "-lGo", "Ref-Platform"},
	} {
		r := Parse(args, map[string]string{"GH_REPO": "rashidgg/babyapp"})
		if r.Target.Owner != "Ref-Platform" || r.Target.Repo != "" || !r.TargetExplicit {
			t.Fatalf("list owner lost for %v", args)
		}
	}
	for _, args := range [][]string{{"repo", "view", "https://github.com/Ref-Platform/web-platform"}, {"pr", "list", "-R", "github.com/Ref-Platform/web-platform"}, {"repo", "fork", "https://github.com/Ref-Platform/web-platform"}} {
		if r := Parse(args, map[string]string{"GH_HOST": "enterprise.example"}); r.Target.Host != "github.com" {
			t.Fatal("GH_HOST overrode explicit host")
		}
	}
	if r := Parse([]string{"auth", "token", "-u", "someone"}, map[string]string{"GH_REPO": "enterprise.example/Other/Repo"}); r.Target.Host != "" {
		t.Fatal("named token retrieval depends on GH_REPO")
	}
}

func TestAPIPlaceholders(t *testing.T) {
	r := Parse([]string{"api", "repos/{owner}/{repo}/issues"}, map[string]string{"GH_REPO": "Ref-Platform/web-platform"})
	if NeedsContext(r.Target) || r.Target != (model.Target{Host: "github.com", Owner: "Ref-Platform", Repo: "web-platform"}) {
		t.Fatal("GH_REPO did not resolve placeholders")
	}
	r = Parse([]string{"api", "repos/{owner}/{repo}/issues"}, nil)
	if !NeedsContext(r.Target) {
		t.Fatal("missing placeholder context marker")
	}
	resolved := ResolveContext(r.Target, model.Target{Host: "github.com", Owner: "Owner", Repo: "Repo"})
	if resolved != (model.Target{Host: "github.com", Owner: "Owner", Repo: "Repo"}) {
		t.Fatal("base remote did not resolve placeholders")
	}
	r = Parse([]string{"api", "repos/Explicit/{repo}"}, map[string]string{"GH_REPO": "Other/Name"})
	if r.Target.Owner != "Explicit" || r.Target.Repo != "Name" {
		t.Fatal("placeholder resolution overwrote explicit owner")
	}
}
