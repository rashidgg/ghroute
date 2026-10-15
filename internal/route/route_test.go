package route

import (
	"errors"
	"strings"
	"testing"

	"github.com/rashidgg/ghroute/internal/model"
)

var accounts = []model.Account{
	{Login: "rashidgg", ID: 45042692, Orgs: []string{"SomeOrg", "Shared"}},
	{Login: "rashidrefs", ID: 342224320, Orgs: []string{"Ref-Platform", "Shared"}},
}

func TestDecide(t *testing.T) {
	tests := []struct {
		name  string
		req   model.Request
		kind  model.DecisionKind
		login string
	}{
		{"fork uses destination", model.Request{Fork: true, Target: model.Target{Host: "github.com", Owner: "Ref-Platform", Repo: "web-platform"}}, model.UseAccount, "rashidgg"},
		{"fork org", model.Request{Fork: true, Org: "Ref-Platform"}, model.UseAccount, "rashidrefs"},
		{"explicit owner", model.Request{Target: model.Target{Host: "github.com", Owner: "REF-platform", Repo: "web-platform"}, TargetExplicit: true}, model.UseAccount, "rashidrefs"},
		{"explicit overrides outside token", model.Request{Target: model.Target{Host: "github.com", Owner: "Ref-Platform", Repo: "web-platform"}, TargetExplicit: true, InheritedToken: true}, model.UseAccount, "rashidrefs"},
		{"outside token", model.Request{InheritedToken: true}, model.Passthrough, ""},
		{"routed child without repository", model.Request{InheritedToken: true, RouterToken: true}, model.Passthrough, ""},
		{"outside token with cwd", model.Request{InheritedToken: true, Target: model.Target{Host: "github.com", Owner: "Ref-Platform", Repo: "web-platform"}}, model.Passthrough, ""},
		{"router explicit", model.Request{InheritedToken: true, RouterToken: true, TargetExplicit: true, Target: model.Target{Host: "github.com", Owner: "Ref-Platform", Repo: "web-platform"}}, model.UseAccount, "rashidrefs"},
		{"no repository", model.Request{}, model.UseAccount, "rashidgg"},
		{"unknown owner", model.Request{Target: model.Target{Host: "github.com", Owner: "Unknown", Repo: "repo"}}, model.Probe, ""},
		{"shared org", model.Request{Target: model.Target{Host: "github.com", Owner: "Shared", Repo: "repo"}}, model.Ambiguous, ""},
		{"enterprise", model.Request{Target: model.Target{Host: "enterprise.test", Owner: "Ref-Platform", Repo: "repo"}}, model.Passthrough, ""},
		{"auth management", model.Request{Passthrough: true, Target: model.Target{Host: "github.com", Owner: "Ref-Platform", Repo: "repo"}}, model.Passthrough, ""},
		{"token user", model.Request{TokenUser: "rashidrefs", Target: model.Target{Host: "github.com", Owner: "rashidgg", Repo: "repo"}}, model.UseAccount, "rashidrefs"},
		{"missing user", model.Request{TokenUser: "unknown"}, model.Missing, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Decide(tt.req, accounts, "rashidgg")
			if d.Kind != tt.kind || d.Account.Login != tt.login {
				t.Fatalf("got %+v", d)
			}
		})
	}
}

func TestLoginWinsOverOrg(t *testing.T) {
	as := append([]model.Account{}, accounts...)
	as[1].Orgs = append(as[1].Orgs, "rashidgg")
	got := Match("RASHIDGG", as)
	if len(got) != 1 || got[0].Login != "rashidgg" {
		t.Fatal("login must beat organization")
	}
}

func TestChosenTokenFailureIsHardError(t *testing.T) {
	for _, get := range []func(string) (string, error){func(string) (string, error) { return "", nil }, func(string) (string, error) { return "", errors.New("sensitive provider output") }} {
		_, err := RequireToken(accounts[1], get)
		if err == nil || !strings.Contains(err.Error(), "rashidrefs") || strings.Contains(err.Error(), "sensitive provider output") {
			t.Fatal("missing safe account-specific error")
		}
	}
}
