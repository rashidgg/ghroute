package discover

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rashidgg/ghroute/internal/model"
	"github.com/rashidgg/ghroute/internal/process"
	"github.com/rashidgg/ghroute/internal/route"
)

const MaxAge = 6 * time.Hour

type Cache struct {
	Timestamp time.Time                `json:"timestamp"`
	Accounts  []model.Account          `json:"accounts"`
	Profiles  map[string]model.Profile `json:"profiles"`
	Probes    map[string]string        `json:"probes"`
}

type Service struct {
	Runner process.Runner
	GH     string
	Env    []string
	Path   string
	Now    func() time.Time
}

type statusAccount struct {
	Login  string `json:"login"`
	Active bool   `json:"active"`
}

func (s *Service) run(ctx context.Context, token string, args ...string) ([]byte, error) {
	env := process.CleanEnv(s.Env)
	if token != "" {
		env = process.TokenEnv(env, token)
	}
	return s.Runner.Run(ctx, process.Command{Name: s.GH, Args: args, Env: env})
}

func (s *Service) Token(ctx context.Context, login string) (string, error) {
	b, err := s.run(ctx, "", "auth", "token", "--hostname", "github.com", "--user", login)
	token := strings.TrimSpace(string(b))
	if err != nil || token == "" || strings.ContainsAny(token, "\r\n") {
		return "", fmt.Errorf("cannot read token for account %s", login)
	}
	return token, nil
}

func (s *Service) Status(ctx context.Context) ([]string, string, error) {
	b, err := s.run(ctx, "", "auth", "status", "--hostname", "github.com", "--json", "hosts")
	if err != nil {
		return nil, "", fmt.Errorf("cannot discover GitHub.com logins with gh auth status")
	}
	var status struct {
		Hosts map[string][]statusAccount `json:"hosts"`
	}
	if json.Unmarshal(b, &status) != nil {
		return nil, "", fmt.Errorf("invalid gh auth status response")
	}
	var logins []string
	active := ""
	for _, a := range status.Hosts["github.com"] {
		if a.Login == "" {
			continue
		}
		logins = append(logins, a.Login)
		if a.Active {
			active = a.Login
		}
	}
	if len(logins) == 0 {
		return nil, "", fmt.Errorf("no logged-in GitHub.com accounts; run gh auth login")
	}
	return logins, active, nil
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) lock() (func(), error) {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(s.Path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}

func (s *Service) read() (Cache, error) {
	var c Cache
	b, err := os.ReadFile(s.Path)
	if err == nil {
		err = json.Unmarshal(b, &c)
	}
	return c, err
}

func (s *Service) save(c Cache) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.Path), ".cache-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), s.Path)
}

func sameAccounts(logins []string, accounts []model.Account) bool {
	if len(logins) != len(accounts) {
		return false
	}
	for _, login := range logins {
		found := false
		for _, a := range accounts {
			if strings.EqualFold(login, a.Login) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// Load uses live status for the active account and cached metadata for six hours.
// A changed login set invalidates the cache immediately. The lock serializes
// updates across independent T3 threads; rename keeps readers from seeing partial JSON.
func (s *Service) Load(ctx context.Context, force bool) (Cache, string, error) {
	logins, active, err := s.Status(ctx)
	if err != nil {
		return Cache{}, "", err
	}
	unlock, err := s.lock()
	if err != nil {
		return Cache{}, "", err
	}
	defer unlock()
	c, err := s.read()
	age := s.now().Sub(c.Timestamp)
	if err == nil && !force && age >= 0 && age < MaxAge && sameAccounts(logins, c.Accounts) {
		if err := os.Chmod(s.Path, 0600); err != nil {
			return Cache{}, "", err
		}
		return c, active, nil
	}
	c = Cache{Timestamp: s.now(), Profiles: map[string]model.Profile{}, Probes: map[string]string{}}
	for _, login := range logins {
		token, err := s.Token(ctx, login)
		if err != nil {
			return Cache{}, "", err
		}
		b, err := s.run(ctx, token, "api", "user", "--hostname", "github.com")
		if err != nil {
			return Cache{}, "", fmt.Errorf("cannot read GitHub profile for account %s", login)
		}
		var user struct {
			Login string
			ID    int64
			Name  string
			Email string
		}
		if json.Unmarshal(b, &user) != nil || user.ID == 0 || !strings.EqualFold(user.Login, login) {
			return Cache{}, "", fmt.Errorf("invalid GitHub profile for account %s", login)
		}
		b, err = s.run(ctx, token, "api", "user/orgs", "--paginate", "--hostname", "github.com")
		if err != nil {
			return Cache{}, "", fmt.Errorf("cannot read organizations for account %s", login)
		}
		a := model.Account{Login: user.Login, ID: user.ID}
		// gh --paginate emits consecutive JSON arrays, one per page.
		decoder := json.NewDecoder(bytes.NewReader(b))
		for {
			var orgs []struct{ Login string }
			err = decoder.Decode(&orgs)
			if err == io.EOF {
				break
			}
			if err != nil {
				return Cache{}, "", fmt.Errorf("invalid organizations for account %s", login)
			}
			for _, org := range orgs {
				a.Orgs = append(a.Orgs, org.Login)
			}
		}
		c.Accounts = append(c.Accounts, a)
		c.Profiles[strings.ToLower(user.Login)] = model.Profile{Name: user.Name, Email: user.Email}
	}
	if err = s.save(c); err != nil {
		return Cache{}, "", err
	}
	return c, active, nil
}

func probeKey(t model.Target) string { return strings.ToLower(t.Host + "/" + t.Owner + "/" + t.Repo) }

func (s *Service) Probe(ctx context.Context, c Cache, target model.Target) (model.Account, error) {
	if target.Owner == "" || target.Repo == "" {
		return model.Account{}, fmt.Errorf("cannot uniquely select an account for owner %s without a repository to probe", target.Owner)
	}
	key := probeKey(target)
	if login := c.Probes[key]; login != "" {
		for _, a := range c.Accounts {
			if strings.EqualFold(a.Login, login) {
				return a, nil
			}
		}
	}
	var matches []model.Account
	for _, a := range c.Accounts {
		token, err := route.RequireToken(a, func(login string) (string, error) { return s.Token(ctx, login) })
		if err != nil {
			return model.Account{}, err
		}
		response, err := s.run(ctx, token, "api", "--method", "GET", "repos/"+target.Owner+"/"+target.Repo, "--hostname", "github.com", "--include", "--silent")
		if err == nil {
			matches = append(matches, a)
		} else {
			// A failed transport or rate limit is not evidence that this account
			// cannot see the repository. Do not choose another account on that basis.
			line, _, _ := strings.Cut(string(response), "\n")
			fields := strings.Fields(line)
			if len(fields) < 2 || !strings.HasPrefix(fields[0], "HTTP/") || fields[1] != "404" {
				return model.Account{}, fmt.Errorf("cannot determine repository access for account %s; probe failed", a.Login)
			}
		}
	}
	if len(matches) == 0 {
		return model.Account{}, fmt.Errorf("no logged-in account can access %s/%s", target.Owner, target.Repo)
	}
	if len(matches) > 1 {
		return model.Account{}, fmt.Errorf("multiple logged-in accounts can access %s/%s; refusing to guess", target.Owner, target.Repo)
	}
	unlock, err := s.lock()
	if err != nil {
		return model.Account{}, err
	}
	defer unlock()
	// Merge concurrent probe results only into the same discovery generation.
	current, err := s.read()
	if err == nil && current.Timestamp.Equal(c.Timestamp) {
		if current.Probes == nil {
			current.Probes = map[string]string{}
		}
		current.Probes[key] = matches[0].Login
		if err := s.save(current); err != nil {
			return model.Account{}, err
		}
	}
	return matches[0], nil
}
