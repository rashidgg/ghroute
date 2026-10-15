// Package route contains the pure account selection rules.
package route

import (
	"fmt"
	"strings"

	"github.com/rashidgg/ghroute/internal/model"
)

func Match(owner string, accounts []model.Account) []model.Account {
	var users, orgs []model.Account
	for _, a := range accounts {
		if strings.EqualFold(a.Login, owner) {
			users = append(users, a)
			continue
		}
		for _, org := range a.Orgs {
			if strings.EqualFold(org, owner) {
				orgs = append(orgs, a)
				break
			}
		}
	}
	if len(users) > 0 {
		return users
	}
	return orgs
}

func Decide(r model.Request, accounts []model.Account, active string) model.Decision {
	d := model.Decision{Target: r.Target}
	if r.Passthrough || (r.Target.Host != "" && !strings.EqualFold(r.Target.Host, "github.com")) {
		d.Kind = model.Passthrough
		return d
	}
	owner := r.Target.Owner
	if r.TokenUser != "" {
		for _, a := range accounts {
			if strings.EqualFold(a.Login, r.TokenUser) {
				d.Kind, d.Account = model.UseAccount, a
				return d
			}
		}
		d.Kind, d.Reason = model.Missing, "account "+r.TokenUser+" is not logged in"
		return d
	}
	if r.Fork {
		owner = r.Org
		d.Target = model.Target{Host: "github.com", Owner: owner}
	}
	if !r.Fork && !r.TargetExplicit && r.InheritedToken && !r.RouterToken {
		d.Kind = model.Passthrough
		return d
	}
	if !r.Fork && !r.TargetExplicit && owner == "" && r.InheritedToken && r.RouterToken {
		// A child outside a checkout has no new repository to route. Retain its
		// parent's selected account instead of reverting to the global active login.
		d.Kind = model.Passthrough
		return d
	}
	if owner == "" {
		for _, a := range accounts {
			if strings.EqualFold(a.Login, active) {
				d.Kind, d.Account = model.UseAccount, a
				return d
			}
		}
		d.Kind, d.Reason = model.Missing, "no active GitHub.com account; run gh auth login"
		return d
	}
	matches := Match(owner, accounts)
	switch len(matches) {
	case 0:
		d.Kind, d.Reason = model.Probe, "no account owns "+owner
	case 1:
		d.Kind, d.Account = model.UseAccount, matches[0]
	default:
		d.Kind, d.Reason = model.Ambiguous, "multiple accounts belong to "+owner
	}
	return d
}

// RequireToken fails closed and never embeds a provider error (or token).
func RequireToken(a model.Account, get func(string) (string, error)) (string, error) {
	token, err := get(a.Login)
	if err != nil || strings.TrimSpace(token) == "" {
		return "", fmt.Errorf("cannot read token for account %s; run gh auth login for that account", a.Login)
	}
	return token, nil
}
