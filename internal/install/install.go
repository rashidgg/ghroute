// Package install activates routing, GitHub credential handling, and shell PATH setup.
package install

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rashidgg/ghroute/internal/process"
)

const helperKey = "credential.https://github.com.helper"

type Paths struct {
	RealGH, HomebrewGH, Binary, Link, State, Brew string
}

func DefaultPaths(home string) Paths {
	return Paths{RealGH: process.RealGH, HomebrewGH: "/opt/homebrew/bin/gh", Binary: filepath.Join(home, ".local/bin/ghroute"), Link: filepath.Join(home, ".local/bin/gh"), State: filepath.Join(home, ".local/state/ghroute/install.json"), Brew: "/opt/homebrew/bin/brew"}
}

type Plan struct {
	Paths       Paths
	GitCommands [][]string
	Helper      string
}

func NewPlan(p Paths) Plan {
	helper := "!" + shellQuote(p.Binary) + " credential"
	return Plan{Paths: p, Helper: helper, GitCommands: [][]string{
		{"config", "--global", "credential.https://github.com.useHttpPath", "true"},
		{"config", "--global", "--add", helperKey, ""},
		{"config", "--global", "--add", helperKey, helper},
	}}
}

func shellQuote(s string) string {
	if !strings.ContainsAny(s, " \t\n'\"\\$`;|&()<>*?[]{}!") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

type State struct {
	Disabled       bool       `json:"disabled,omitempty"`
	OriginalTarget string     `json:"original_homebrew_target,omitempty"`
	Helper         string     `json:"helper"`
	AddedHelper    bool       `json:"added_helper"`
	AddedReset     bool       `json:"added_reset"`
	PackageBinary  string     `json:"package_binary,omitempty"`
	ConfigPath     string     `json:"git_config_path,omitempty"`
	PathEdits      []PathEdit `json:"path_edits,omitempty"`
}

// RoutingDisabled is generated activation state, never user-edited routing config.
func RoutingDisabled(path string) (bool, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("cannot read ghroute activation state")
	}
	var state State
	if json.Unmarshal(b, &state) != nil {
		return false, fmt.Errorf("invalid ghroute activation state")
	}
	return state.Disabled, nil
}

type Installer struct {
	Runner        process.Runner
	Env           []string
	Executable    string
	PackageBinary string
	PathEdits     []PathEdit
	Plan          Plan
}

func (i Installer) run(ctx context.Context, name string, args ...string) error {
	_, err := i.Runner.Run(ctx, process.Command{Name: name, Args: args, Env: i.Env})
	if err != nil {
		return fmt.Errorf("%s %s failed", filepath.Base(name), args[0])
	}
	return nil
}

func (i Installer) helpers(ctx context.Context, file string) ([]string, error) {
	b, err := i.Runner.Run(ctx, process.Command{Name: "git", Args: []string{"config", "--file", file, "--null", "--get-all", helperKey}, Env: i.Env})
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot inspect git credential helpers")
	}
	return strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00"), nil
}

func writeState(path string, state State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, b, 0600)
}

func replaceLink(path, target string) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".gh-link-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	f.Close()
	os.Remove(tmp)
	defer os.Remove(tmp)
	if err = os.Symlink(target, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (i Installer) Apply(ctx context.Context) error {
	return i.apply(ctx, false)
}

func (i Installer) apply(ctx context.Context, keepDisabled bool) error {
	p := i.Plan.Paths
	info, err := os.Stat(p.RealGH)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return fmt.Errorf("real gh binary is missing or not executable: %s", p.RealGH)
	}
	state := State{Helper: i.Plan.Helper}
	existing := false
	if b, err := os.ReadFile(p.State); err == nil {
		if json.Unmarshal(b, &state) != nil || state.Helper != i.Plan.Helper {
			return fmt.Errorf("invalid ghroute installation state")
		}
		existing = true
	} else if !os.IsNotExist(err) {
		return err
	} else if current, _ := os.Readlink(p.HomebrewGH); current == p.Link || current == p.Binary {
		return fmt.Errorf("cannot recover original Homebrew symlink without installation state")
	}
	// Retain the original Mac's integration; new installations use PATH only.
	if state.OriginalTarget != "" {
		if err := checkLegacyLink(p, &state); err != nil {
			return err
		}
	}
	if target, err := os.Readlink(p.Link); err == nil {
		if target != p.Binary {
			return fmt.Errorf("refusing to replace existing %s", p.Link)
		}
	} else if _, err := os.Lstat(p.Link); err == nil || !os.IsNotExist(err) {
		return fmt.Errorf("refusing to replace existing %s", p.Link)
	}
	if _, err := os.Lstat(p.Binary); err == nil {
		if !existing && !sameFile(p.Binary, i.Executable) && !isGhrouteBinary(p.Binary) {
			return fmt.Errorf("refusing to replace existing %s without installation state", p.Binary)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	info, err = os.Stat(i.Executable)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return fmt.Errorf("ghroute executable is missing or not executable")
	}
	if i.PackageBinary != "" && (!filepath.IsAbs(i.PackageBinary) || !sameFile(i.Executable, i.PackageBinary)) {
		return fmt.Errorf("Homebrew executable does not match its managed path; reinstall with brew")
	}
	var activationEdits []PathEdit
	for _, edit := range i.PathEdits {
		found := false
		for _, old := range state.PathEdits {
			if old.Path == edit.Path {
				found = true
				edit = old
			}
		}
		if !found {
			state.PathEdits = append(state.PathEdits, edit)
		}
		activationEdits = append(activationEdits, edit)
	}
	for _, edit := range activationEdits {
		if _, _, err := edit.contents(false); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(p.Binary), 0755); err != nil {
		return err
	}
	state.PackageBinary = i.PackageBinary
	if keepDisabled {
		state.Disabled = true
	}
	// Record ownership before changes, so interrupted installs can be retried.
	if err := writeState(p.State, state); err != nil {
		return err
	}
	if err := i.installBinary(); err != nil {
		return err
	}
	if state.OriginalTarget != "" && !keepDisabled {
		if err := i.run(ctx, p.Brew, "pin", "gh"); err != nil {
			return err
		}
	}
	if !keepDisabled {
		if err := i.editGitConfig(state.ConfigPath, func(file, path string) error {
			helpers, err := i.helpers(ctx, file)
			if err != nil {
				return err
			}
			found := false
			for _, helper := range helpers {
				if helper == i.Plan.Helper {
					found = true
				}
			}
			if !found {
				state.AddedHelper, state.AddedReset = true, true
			}
			state.ConfigPath = path
			// Persist ownership before committing any helper changes.
			if err := writeState(p.State, state); err != nil {
				return err
			}
			for n, args := range i.Plan.GitCommands {
				if found && n > 0 {
					continue
				}
				args = append([]string{"config", "--file", file}, args[2:]...)
				if err := i.run(ctx, "git", args...); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	if err := replaceLink(p.Link, p.Binary); err != nil {
		return err
	}
	if state.OriginalTarget != "" && !keepDisabled {
		if err := replaceLink(p.HomebrewGH, p.Link); err != nil {
			return err
		}
	}
	for _, edit := range activationEdits {
		if err := edit.apply(false); err != nil {
			return err
		}
	}
	if keepDisabled {
		return i.Off(ctx)
	}
	if err := os.Remove(i.disabledPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	if state.Disabled {
		state.Disabled = false
		return writeState(p.State, state)
	}
	return nil
}

func checkLegacyLink(p Paths, state *State) error {
	current, err := os.Readlink(p.HomebrewGH)
	if err != nil {
		return fmt.Errorf("refusing to overwrite a non-symlink Homebrew gh")
	}
	// While off, gh is unpinned and Homebrew may upgrade it. Retain that new
	// official target so enabling/disabling again cannot restore a stale keg.
	if state.Disabled && current != p.Link && current != p.Binary && sameFile(p.HomebrewGH, p.RealGH) {
		state.OriginalTarget = current
	}
	if current != p.Link && current != p.Binary && current != state.OriginalTarget {
		return fmt.Errorf("Homebrew gh symlink changed since install; refusing to overwrite it")
	}
	return nil
}

func (i Installer) Restore(ctx context.Context) error {
	return i.restore(ctx, false)
}

// Off disables routing and restores Git helpers while preserving the executable,
// shell PATH, and shim. Keeping the shim lets already-open shells use normal gh
// immediately, even if they cached the shim's absolute path.
func (i Installer) Off(ctx context.Context) error {
	return i.restore(ctx, true)
}

func (i Installer) restore(ctx context.Context, keepInstalled bool) error {
	p := i.Plan.Paths
	b, err := os.ReadFile(p.State)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var state State
	if json.Unmarshal(b, &state) != nil || state.Helper != i.Plan.Helper {
		return fmt.Errorf("invalid ghroute installation state")
	}
	if !keepInstalled {
		for _, edit := range state.PathEdits {
			if _, _, err := edit.contents(true); err != nil {
				return err
			}
		}
	}
	if state.OriginalTarget != "" {
		if err := checkLegacyLink(p, &state); err != nil {
			return err
		}
	}
	if keepInstalled {
		// Switch passthrough on before restoring integrations. If restoration is
		// interrupted, subsequent invocations still use the official CLI.
		state.Disabled = true
		if err := writeState(p.State, state); err != nil {
			return err
		}
		if err := atomicWrite(i.disabledPath(), []byte("off\n"), 0600); err != nil {
			return err
		}
	}
	if state.OriginalTarget != "" {
		if err := i.run(ctx, p.Brew, "unpin", "gh"); err != nil {
			return err
		}
		if err := replaceLink(p.HomebrewGH, state.OriginalTarget); err != nil {
			return err
		}
	}
	if !keepInstalled {
		if target, err := os.Readlink(p.Link); err == nil && target == p.Binary {
			if err := os.Remove(p.Link); err != nil {
				return err
			}
		}
	}
	if state.AddedHelper {
		if err := i.editGitConfig(state.ConfigPath, func(file, _ string) error {
			helpers, err := i.helpers(ctx, file)
			if err != nil {
				return err
			}
			for n, helper := range helpers {
				if helper != state.Helper {
					continue
				}
				start := n
				// Remove our adjacent reset too: leaving it at the end would disable
				// the restored original helper. Keep every pre-existing empty helper.
				if state.AddedReset && n > 0 && helpers[n-1] == "" {
					start--
				}
				remaining := append(append([]string{}, helpers[:start]...), helpers[n+1:]...)
				// Git has no positional multivalue deletion. Use its own config writer
				// to replace only this key, preserving the current unrelated entries.
				if err := i.run(ctx, "git", "config", "--file", file, "--unset-all", helperKey); err != nil {
					return err
				}
				for _, value := range remaining {
					if err := i.run(ctx, "git", "config", "--file", file, "--add", helperKey, value); err != nil {
						return err
					}
				}
				break
			}
			return nil
		}); err != nil {
			return err
		}
	}
	if keepInstalled {
		return nil
	}
	for _, edit := range state.PathEdits {
		if err := edit.apply(true); err != nil {
			return err
		}
	}
	if target, err := os.Readlink(p.Binary); err == nil && state.PackageBinary != "" && target == state.PackageBinary {
		if err := os.Remove(p.Binary); err != nil {
			return err
		}
	}
	return os.Remove(p.State)
}
