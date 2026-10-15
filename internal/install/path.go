package install

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const pathStart = "# >>> ghroute PATH >>>"
const pathEnd = "# <<< ghroute PATH <<<"

// PathEdit records only our appended text, never a copy of the user's shell config.
type PathEdit struct {
	Path    string `json:"path"`
	Block   string `json:"block"`
	Created bool   `json:"created,omitempty"`
}

// ShellPaths uses the shell's own startup files and PATH facilities.
func ShellPaths(home, shell string, env map[string]string) ([]PathEdit, error) {
	var paths []string
	command := `case "$PATH" in
  "$HOME/.local/bin"|"$HOME/.local/bin":*) ;;
  *) export PATH="$HOME/.local/bin:$PATH" ;;
esac`
	switch filepath.Base(shell) {
	case "fish":
		config := env["XDG_CONFIG_HOME"]
		if config == "" {
			config = filepath.Join(home, ".config")
		}
		paths = []string{filepath.Join(config, "fish/config.fish")}
		command = `fish_add_path --prepend --move --path "$HOME/.local/bin"`
	case "zsh":
		dir := env["ZDOTDIR"]
		if dir == "" {
			dir = home
		}
		paths = []string{filepath.Join(dir, ".zprofile"), filepath.Join(dir, ".zshrc")}
	case "bash":
		profile := filepath.Join(home, ".bash_profile")
		for _, name := range []string{".bash_profile", ".bash_login", ".profile"} {
			candidate := filepath.Join(home, name)
			if _, err := os.Stat(candidate); err == nil {
				profile = candidate
				break
			} else if !os.IsNotExist(err) {
				return nil, err
			}
		}
		paths = []string{profile, filepath.Join(home, ".bashrc")}
	case "sh":
		paths = []string{filepath.Join(home, ".profile")}
	default:
		return nil, fmt.Errorf("unsupported shell %q; use ghroute on --shell fish|zsh|bash|sh or --no-path", shell)
	}
	var edits []PathEdit
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			return nil, fmt.Errorf("shell configuration path must be absolute: %s", path)
		}
		// Preserve dotfile-manager symlinks by editing their target.
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			path = resolved
		} else if !os.IsNotExist(err) {
			return nil, err
		} else if _, err := os.Lstat(path); err == nil {
			return nil, fmt.Errorf("cannot edit broken shell configuration symlink: %s", path)
		}
		_, err = os.Stat(path)
		edits = append(edits, PathEdit{Path: path, Block: "\n" + pathStart + "\n" + command + "\n" + pathEnd + "\n", Created: os.IsNotExist(err)})
	}
	return edits, nil
}

func (e PathEdit) contents(remove bool) ([]byte, os.FileMode, error) {
	b, err := os.ReadFile(e.Path)
	mode := os.FileMode(0644)
	if err != nil && !os.IsNotExist(err) {
		return nil, mode, err
	}
	if info, err := os.Stat(e.Path); err == nil {
		mode = info.Mode().Perm()
	}
	s := string(b)
	if strings.Count(s, e.Block) == 1 {
		if remove {
			return []byte(strings.Replace(s, e.Block, "", 1)), mode, nil
		}
		return b, mode, nil
	}
	if strings.Contains(s, pathStart) || strings.Contains(s, pathEnd) {
		return nil, mode, fmt.Errorf("ghroute PATH block in %s was changed; restore or remove that block before retrying", e.Path)
	}
	if remove {
		return b, mode, nil
	}
	return []byte(s + e.Block), mode, nil
}

func (e PathEdit) apply(remove bool) error {
	info, err := os.Lstat(e.Path)
	if remove && os.IsNotExist(err) {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if info != nil && info.Mode()&os.ModeSymlink != 0 {
		// A dotfile manager may have introduced a symlink since activation.
		resolved, err := filepath.EvalSymlinks(e.Path)
		if err != nil {
			return err
		}
		e.Path, e.Created = resolved, false
	}
	b, mode, err := e.contents(remove)
	if err != nil {
		return err
	}
	if before, err := os.ReadFile(e.Path); err == nil && bytes.Equal(before, b) {
		return nil
	}
	if remove && e.Created && len(b) == 0 {
		err := os.Remove(e.Path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(e.Path), 0755); err != nil {
		return err
	}
	return atomicWrite(e.Path, b, mode)
}
