package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/rashidgg/ghroute/internal/discover"
	"github.com/rashidgg/ghroute/internal/identity"
	"github.com/rashidgg/ghroute/internal/model"
	"github.com/rashidgg/ghroute/internal/parse"
	"github.com/rashidgg/ghroute/internal/route"
)

func selectAccount(ctx context.Context, s *discover.Service, c discover.Cache, active string, r model.Request) (model.Account, error) {
	d := route.Decide(r, c.Accounts, active)
	switch d.Kind {
	case model.UseAccount:
		return d.Account, nil
	case model.Probe, model.Ambiguous:
		return s.Probe(ctx, c, d.Target)
	default:
		return model.Account{}, fmt.Errorf("%s", d.Reason)
	}
}

func ensureIdentity(ctx context.Context, git identity.Git, s *discover.Service, c discover.Cache, active string, request model.Request) error {
	// Fork creation can replace origin. Let the next routed command initialize
	// identity from that final origin instead of pinning the source identity.
	if request.Fork {
		return nil
	}
	url := git.Remote(ctx, "origin")
	if url == "" {
		return nil
	}
	target := parse.Target(url)
	if target.Host != "github.com" || target.Owner == "" {
		return nil
	}
	email, err := git.LocalEmail(ctx)
	if err != nil || email != "" {
		return err
	}
	a, err := selectAccount(ctx, s, c, active, model.Request{Target: target, TargetExplicit: true})
	if err != nil {
		return fmt.Errorf("cannot select origin's commit identity: %w", err)
	}
	return git.Apply(ctx, a, c.Profiles[strings.ToLower(a.Login)])
}

func repositoryContext(ctx context.Context, git identity.Git, r model.Request) (model.Request, error) {
	if r.TargetExplicit && !parse.NeedsContext(r.Target) {
		return r, nil
	}
	cmd, sub := parse.Command(r.Args)
	// API/auth have a host-specific endpoint. For ordinary repository commands,
	// GH_HOST is only a fallback when the host cannot be inferred from Git.
	if (cmd == "api" || cmd == "auth") && r.Target.Host != "" && r.Target.Host != "github.com" {
		return r, nil
	}
	remote := git.BaseRemote(ctx, cmd == "pr" && sub == "create")
	if r.Fork {
		if r.RemoteURL == "" && remote != "" {
			r.RemoteURL = remote
			r.Target.Host = parse.Target(remote).Host
		}
	} else if parse.NeedsContext(r.Target) {
		r.Target = parse.ResolveContext(r.Target, parse.Target(remote))
		if r.Target.Owner == "" || r.Target.Repo == "" {
			return r, fmt.Errorf("API repository placeholders require GH_REPO or a local repository")
		}
	} else if remote != "" {
		r.RemoteURL, r.Target = remote, parse.Target(remote)
	}
	return r, nil
}
