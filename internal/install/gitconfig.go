package install

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// globalConfigPath follows Git's documented choice of its single global write
// target. Reads/writes within a transaction use --file, so we never flatten XDG
// configuration or include files into that target.
func globalConfigPath(env []string) (string, error) {
	values := map[string]string{}
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		values[key] = value
	}
	path, override := values["GIT_CONFIG_GLOBAL"]
	if !override {
		home := values["HOME"]
		if home == "" {
			return "", fmt.Errorf("HOME is required to locate global Git configuration")
		}
		path = filepath.Join(home, ".gitconfig")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			xdg := values["XDG_CONFIG_HOME"]
			if xdg == "" {
				xdg = filepath.Join(home, ".config")
			}
			candidate := filepath.Join(xdg, "git/config")
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
			} else if !os.IsNotExist(err) {
				return "", err
			}
		} else if err != nil {
			return "", err
		}
	}
	if path == "" {
		return "", fmt.Errorf("global Git configuration path is empty")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved, nil
	} else if !os.IsNotExist(err) {
		return "", err
	} else if _, err := os.Lstat(path); err == nil {
		return "", fmt.Errorf("global Git configuration is a broken symlink")
	}
	return path, nil
}

// editGitConfig batches native Git config writes into one atomic commit. Git's
// config command has no batch transaction API; its standard .lock convention
// protects the live file while Git edits the staged file. No config is parsed
// or formatted here, and failures leave the live file byte-for-byte unchanged.
func (i Installer) editGitConfig(path string, edit func(file, path string) error) error {
	env := i.Env
	if path != "" {
		env = []string{"GIT_CONFIG_GLOBAL=" + path}
	}
	path, err := globalConfigPath(env)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	lock := path + ".lock"
	f, err := os.OpenFile(lock, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("cannot lock global Git configuration; retry when no other Git config operation is running")
	}
	committed := false
	defer func() {
		if !committed {
			os.Remove(lock)
		}
	}()
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		f.Close()
		return err
	}
	mode := os.FileMode(0600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
		if !info.Mode().IsRegular() {
			f.Close()
			return fmt.Errorf("global Git configuration is not a regular file")
		}
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := edit(lock, path); err != nil {
		return err
	}
	after, err := os.ReadFile(lock)
	if err != nil {
		return err
	}
	if bytes.Equal(b, after) {
		return nil
	}
	if err := os.Chmod(lock, mode); err != nil {
		return err
	}
	if err := os.Rename(lock, path); err != nil {
		return err
	}
	committed = true
	return nil
}
