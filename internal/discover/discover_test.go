package discover

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rashidgg/ghroute/internal/model"
	"github.com/rashidgg/ghroute/internal/process"
)

type fakeRunner func(context.Context, process.Command) ([]byte, error)

func (f fakeRunner) Run(ctx context.Context, c process.Command) ([]byte, error) { return f(ctx, c) }

func fixture(t *testing.T) (*Service, *int) {
	t.Helper()
	calls := 0
	s := &Service{GH: process.RealGH, Env: []string{"PATH=/bin", "GH_TOKEN=outside", "GITHUB_TOKEN=outside", "GH_ENTERPRISE_TOKEN=outside", "GITHUB_ENTERPRISE_TOKEN=outside"}, Path: filepath.Join(t.TempDir(), "cache.json"), Now: func() time.Time { return time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC) }}
	s.Runner = fakeRunner(func(_ context.Context, c process.Command) ([]byte, error) {
		calls++
		if c.Name != process.RealGH {
			t.Fatal("wrong real gh path")
		}
		for _, entry := range c.Env {
			if strings.HasSuffix(entry, "=outside") {
				t.Fatal("inherited token leaked into discovery")
			}
		}
		args := strings.Join(c.Args, " ")
		switch args {
		case "auth status --hostname github.com --json hosts":
			return []byte(`{"hosts":{"github.com":[{"login":"first","active":true},{"login":"second","active":false}]}}`), nil
		case "auth token --hostname github.com --user first":
			return []byte("fixture-first"), nil
		case "auth token --hostname github.com --user second":
			return []byte("fixture-second"), nil
		}
		login := ""
		for _, entry := range c.Env {
			if entry == "GH_TOKEN=fixture-first" {
				login = "first"
			}
			if entry == "GH_TOKEN=fixture-second" {
				login = "second"
			}
		}
		if login == "" {
			t.Fatal("API call has no account token")
		}
		if args == "api user --hostname github.com" {
			return []byte(`{"login":"` + login + `","id":123,"name":"Example User","email":null}`), nil
		}
		if args == "api user/orgs --paginate --hostname github.com" {
			return []byte(`[{"login":"OneOrg"}][{"login":"AnotherOrg"}]`), nil
		}
		t.Fatal("unexpected discovery command")
		return nil, errors.New("unexpected command")
	})
	return s, &calls
}

func TestDiscoveryCacheAndRefresh(t *testing.T) {
	s, calls := fixture(t)
	ctx := context.Background()
	c, active, err := s.Load(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if active != "first" || len(c.Accounts) != 2 || len(c.Accounts[0].Orgs) != 2 {
		t.Fatal("incorrect discovered metadata")
	}
	if *calls != 7 {
		t.Fatal("unexpected initial discovery calls")
	}
	info, err := os.Stat(s.Path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("cache must be mode 0600")
	}
	b, err := os.ReadFile(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"fixture-first", "fixture-second", "outside"} {
		if strings.Contains(string(b), secret) {
			t.Fatal("secret written to cache")
		}
	}
	if _, _, err := s.Load(ctx, false); err != nil {
		t.Fatal(err)
	}
	if *calls != 8 {
		t.Fatal("fresh cache did not skip profile lookups")
	}
	if _, _, err := s.Load(ctx, true); err != nil {
		t.Fatal(err)
	}
	if *calls != 15 {
		t.Fatal("refresh did not bypass age")
	}
	now := s.now()
	s.Now = func() time.Time { return now.Add(MaxAge) }
	if _, _, err := s.Load(ctx, false); err != nil {
		t.Fatal(err)
	}
	if *calls != 22 {
		t.Fatal("six-hour-old cache was reused")
	}
}

func TestStatusActiveIsNotCached(t *testing.T) {
	s, _ := fixture(t)
	if _, _, err := s.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	prior := s.Runner
	s.Runner = fakeRunner(func(ctx context.Context, c process.Command) ([]byte, error) {
		if c.Args[0] == "auth" && c.Args[1] == "status" {
			return []byte(`{"hosts":{"github.com":[{"login":"first"},{"login":"second","active":true}]}}`), nil
		}
		return prior.Run(ctx, c)
	})
	_, active, err := s.Load(context.Background(), false)
	if err != nil || active != "second" {
		t.Fatal("active account must come from live gh status")
	}
}

func TestAccountChangeInvalidatesCache(t *testing.T) {
	s, calls := fixture(t)
	if _, _, err := s.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	prior := s.Runner
	s.Runner = fakeRunner(func(ctx context.Context, c process.Command) ([]byte, error) {
		if c.Args[0] == "auth" && c.Args[1] == "status" {
			return []byte(`{"hosts":{"github.com":[{"login":"first","active":true}]}}`), nil
		}
		return prior.Run(ctx, c)
	})
	c, _, err := s.Load(context.Background(), false)
	if err != nil || len(c.Accounts) != 1 || *calls != 10 {
		t.Fatal("changed login set did not invalidate cache")
	}
}

func TestProbe(t *testing.T) {
	for _, tt := range []struct {
		name      string
		visible   int
		wantError bool
	}{{"none", 0, true}, {"exactly one", 1, false}, {"multiple", 2, true}} {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := fixture(t)
			ctx := context.Background()
			c, _, err := s.Load(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			prior := s.Runner
			probes := 0
			s.Runner = fakeRunner(func(ctx context.Context, cmd process.Command) ([]byte, error) {
				if len(cmd.Args) > 2 && cmd.Args[0] == "api" && cmd.Args[1] == "--method" {
					probes++
					if probes <= tt.visible {
						return nil, nil
					}
					return []byte("HTTP/2.0 404 Not Found\n"), errors.New("HTTP 404")
				}
				return prior.Run(ctx, cmd)
			})
			target := model.Target{Host: "github.com", Owner: "Unknown", Repo: "Repo"}
			a, err := s.Probe(ctx, c, target)
			if (err != nil) != tt.wantError {
				t.Fatal("wrong probe result")
			}
			if probes != 2 {
				t.Fatal("probe must check every account sequentially")
			}
			if !tt.wantError {
				if a.Login != "first" {
					t.Fatal("wrong probe account")
				}
				cached, err := s.read()
				if err != nil {
					t.Fatal(err)
				}
				target.Owner, target.Repo = "UNKNOWN", "repo"
				if _, err := s.Probe(ctx, cached, target); err != nil {
					t.Fatal(err)
				}
				if probes != 2 {
					t.Fatal("remembered probe was not reused case-insensitively")
				}
			}
		})
	}
}

func TestTokenFailureAndMalformedDataAreSanitized(t *testing.T) {
	s, _ := fixture(t)
	prior := s.Runner
	s.Runner = fakeRunner(func(ctx context.Context, c process.Command) ([]byte, error) {
		if c.Args[0] == "auth" && c.Args[1] == "token" {
			return []byte("sensitive-output"), errors.New("sensitive-output")
		}
		return prior.Run(ctx, c)
	})
	_, _, err := s.Load(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "first") || strings.Contains(err.Error(), "sensitive-output") {
		t.Fatal("unsafe token error")
	}
	if _, err := os.Stat(s.Path); !os.IsNotExist(err) {
		t.Fatal("failed discovery wrote cache")
	}
}

func TestProbeDoesNotMistakeNetworkFailureForDeniedAccess(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	c, _, err := s.Load(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	prior := s.Runner
	probes := 0
	s.Runner = fakeRunner(func(ctx context.Context, cmd process.Command) ([]byte, error) {
		if len(cmd.Args) > 2 && cmd.Args[0] == "api" && cmd.Args[1] == "--method" {
			probes++
			if probes == 1 {
				return nil, nil
			}
			return nil, errors.New("network unavailable")
		}
		return prior.Run(ctx, cmd)
	})
	if _, err := s.Probe(ctx, c, model.Target{Host: "github.com", Owner: "Unknown", Repo: "repo"}); err == nil {
		t.Fatal("network uncertainty must not select a visible account")
	}
	cached, err := s.read()
	if err != nil || len(cached.Probes) != 0 {
		t.Fatal("failed probe cached a guess")
	}
}
