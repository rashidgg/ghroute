// Package parse inspects routing arguments without changing gh's argv.
package parse

import (
	"net"
	"net/url"
	"strings"

	"github.com/rashidgg/ghroute/internal/model"
)

func Host(host string) string {
	host = strings.ToLower(host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	// This is the existing SSH host alias, not an account or owner map.
	if host == "github.com-rashidrefs" {
		host = "github.com"
	}
	return host
}

func Target(raw string) model.Target {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return model.Target{}
	}
	host, path := "github.com", raw
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			return model.Target{}
		}
		host, path = Host(u.Host), u.Path
	} else if at := strings.Index(raw, "@"); at >= 0 {
		if h, p, ok := strings.Cut(raw[at+1:], ":"); ok {
			host, path = Host(h), p
		} else {
			return model.Target{}
		}
	} else {
		parts := strings.Split(strings.TrimPrefix(raw, "/"), "/")
		if len(parts) >= 3 && (strings.Contains(parts[0], ".") || parts[0] == "localhost") {
			host, path = Host(parts[0]), strings.Join(parts[1:], "/")
		}
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	t := model.Target{Host: host}
	if len(parts) >= 2 {
		t.Owner, t.Repo = parts[0], strings.TrimSuffix(parts[1], ".git")
	}
	return t
}

// flag.Value cannot inspect unknown, interspersed gh flags; this scanner only
// extracts routing metadata, leaving validation and execution to the real CLI.
var valueFlags = map[string]bool{
	"--body": true, "--title": true, "--message": true, "-b": true, "-t": true, "-m": true,
	"--notes": true, "--comment": true, "--body-file": true, "--notes-file": true,
	"--repo": true, "-R": true, "--org": true, "--user": true, "-u": true, "--hostname": true,
	"--jq": true, "-q": true, "--json": true, "--template": true, "--method": true, "-X": true,
	"--field": true, "-F": true, "--raw-field": true, "-f": true, "--header": true, "-H": true,
	"--input": true, "--cache": true, "--remote-name": true, "--fork-name": true,
	"--source": true, "-s": true, "--remote": true, "-r": true, "--description": true, "-d": true,
	"--homepage": true, "--license": true, "-l": true, "--gitignore": true, "-g": true,
	"--team": true, "--branch": true, "--base": true, "-B": true, "--head": true,
	"--limit": true, "-L": true, "--search": true, "-S": true, "--visibility": true,
}

var repoValues = map[string]string{
	"clone":  "--upstream-remote-name -u",
	"view":   "--branch -b --jq -q --json --template -t",
	"sync":   "--branch -b --source -s",
	"delete": "",
	"rename": "",
	"create": "--description -d --gitignore -g --homepage -h --license -l --remote -r --source -s --team -t --template -p",
	"new":    "--description -d --gitignore -g --homepage -h --license -l --remote -r --source -s --team -t --template -p",
	"list":   "--jq -q --json --language -l --limit -L --template -t --topic --visibility",
	"fork":   "--fork-name --org --remote-name",
}

func takesValue(key string, pos []string) bool {
	// Never inspect free-form content as routing flags, even before the command.
	switch key {
	case "--repo", "-R", "--body", "--title", "--message", "-b", "-t", "-m", "--notes", "--comment", "--body-file", "--notes-file":
		return true
	}
	if len(pos) > 1 && pos[0] == "repo" {
		if flags, ok := repoValues[pos[1]]; ok {
			return strings.Contains(" "+flags+" ", " "+key+" ")
		}
	}
	if len(pos) > 0 && pos[0] == "api" {
		return strings.Contains(" --cache --field -F --header -H --hostname --input --jq -q --method -X --preview -p --raw-field -f --template -t ", " "+key+" ")
	}
	if key == "-h" && len(pos) > 0 && pos[0] == "auth" {
		return true
	}
	return valueFlags[key]
}

func HasHost(raw string) bool {
	parts := strings.Split(strings.Trim(raw, "/"), "/")
	return strings.Contains(raw, "://") || strings.Contains(raw, "@") || (len(parts) >= 3 && (strings.Contains(parts[0], ".") || parts[0] == "localhost"))
}

func NeedsContext(t model.Target) bool { return t.Owner == "{owner}" || t.Repo == "{repo}" }

func ResolveContext(t, base model.Target) model.Target {
	if t.Owner == "{owner}" {
		t.Owner = base.Owner
	}
	if t.Repo == "{repo}" {
		t.Repo = base.Repo
	}
	return t
}

type scanResult struct {
	pos   []string
	flags map[string]string
	help  bool
}

func scan(args []string) scanResult {
	s := scanResult{flags: map[string]string{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		if !strings.HasPrefix(a, "-") {
			s.pos = append(s.pos, a)
			continue
		}
		key, val, equals := strings.Cut(a, "=")
		if key == "--help" || key == "--version" {
			s.help = true
			continue
		}
		if len(key) > 2 && !strings.HasPrefix(key, "--") && takesValue(key[:2], s.pos) {
			key, val, equals = key[:2], a[2:], true
		}
		valueFlag := takesValue(key, s.pos)
		if key == "-h" && !valueFlag {
			s.help = true
			continue
		}
		if valueFlag && !equals && i+1 < len(args) && args[i+1] != "--" {
			i++
			val = args[i]
		}
		if key == "-R" {
			key = "--repo"
		}
		if key == "-u" {
			key = "--user"
		}
		s.flags[key] = val
	}
	return s
}

func Command(args []string) (string, string) {
	p := scan(args).pos
	if len(p) == 0 {
		return "", ""
	}
	if len(p) == 1 {
		return p[0], ""
	}
	return p[0], p[1]
}

func Parse(args []string, env map[string]string) model.Request {
	r := model.Request{Args: args, InheritedToken: env["GH_TOKEN"] != "" || env["GITHUB_TOKEN"] != "", RouterToken: env["GHROUTE_ROUTER"] == "1"}
	s := scan(args)
	cmd, sub := Command(args)
	r.Passthrough = s.help || cmd == "help" || cmd == "completion" || cmd == "__complete" || cmd == "__completeNoDesc"
	if cmd == "auth" {
		switch sub {
		case "status", "switch", "login", "logout", "refresh", "setup-git", "help":
			r.Passthrough = true
		case "token":
			r.TokenUser = s.flags["--user"]
		case "git-credential":
			if len(s.pos) > 2 {
				r.CredentialOp = s.pos[2]
			}
			if r.CredentialOp != "get" {
				r.Passthrough = true
			}
		}
	}
	r.Fork = cmd == "repo" && sub == "fork"
	r.Org = s.flags["--org"]
	host := env["GH_HOST"]
	if h := s.flags["--hostname"]; h != "" {
		host = h
	}
	if cmd == "auth" && s.flags["-h"] != "" {
		host = s.flags["-h"]
	}
	if host != "" {
		r.Target.Host = Host(host)
	}
	explicitHost := false
	set := func(raw string) { r.Target, r.TargetExplicit, explicitHost = Target(raw), true, HasHost(raw) }
	if raw := s.flags["--repo"]; raw != "" {
		set(raw)
	} else if cmd == "repo" && len(s.pos) > 2 {
		raw := s.pos[2]
		switch sub {
		case "clone", "view", "sync", "delete", "create", "new":
			set(raw)
			if !strings.Contains(raw, "/") {
				r.Target.Repo = raw
			}
		case "list":
			r.Target, r.TargetExplicit = model.Target{Host: "github.com", Owner: raw}, true
		// rename's positional argument is the new name; its repository is -R or cwd.
		case "rename":
			if strings.Contains(raw, "/") {
				set(raw)
			}
		case "fork":
			r.RemoteURL = raw
		}
	} else if cmd == "api" && len(s.pos) > 1 {
		raw := strings.TrimPrefix(s.pos[1], "/")
		if strings.HasPrefix(raw, "repos/") {
			t := Target(strings.TrimPrefix(raw, "repos/"))
			if t.Owner != "" && t.Repo != "" {
				r.Target, r.TargetExplicit = t, true
			}
		}
	}
	if raw := env["GH_REPO"]; raw != "" && r.TokenUser == "" {
		if !r.TargetExplicit {
			set(raw)
		} else if NeedsContext(r.Target) {
			r.Target = ResolveContext(r.Target, Target(raw))
		}
	}
	if host != "" && !explicitHost {
		r.Target.Host = Host(host)
	}
	if r.Fork {
		if len(s.pos) > 2 {
			r.RemoteURL = s.pos[2]
		}
		if r.RemoteURL == "" && r.TargetExplicit {
			r.RemoteURL = "https://" + r.Target.Host + "/" + r.Target.Owner + "/" + r.Target.Repo
		}
		if t := Target(r.RemoteURL); t.Host != "" && (host == "" || HasHost(r.RemoteURL)) {
			r.Target.Host = t.Host
		}
		r.Target.Owner, r.Target.Repo, r.TargetExplicit = "", "", false
	}
	return r
}
