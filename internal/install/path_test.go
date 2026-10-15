package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellPathActivationAndExactRemoval(t *testing.T) {
	for _, shell := range []string{"zsh", "bash", "fish", "sh"} {
		t.Run(shell, func(t *testing.T) {
			binary, err := exec.LookPath(shell)
			if err != nil {
				t.Skip("shell is not installed")
			}
			home := t.TempDir()
			bin := filepath.Join(home, ".local/bin")
			if err := os.MkdirAll(bin, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
				t.Fatal(err)
			}
			edits, err := ShellPaths(home, shell, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, edit := range edits {
				if err := os.MkdirAll(filepath.Dir(edit.Path), 0755); err != nil {
					t.Fatal(err)
				}
				original := "# user's configuration without a final newline"
				if err := os.WriteFile(edit.Path, []byte(original), 0600); err != nil {
					t.Fatal(err)
				}
				edit.Created = false
				for n := 0; n < 2; n++ {
					if err := edit.apply(false); err != nil {
						t.Fatal(err)
					}
				}
				command := ". " + shellQuote(edit.Path) + "; command -v gh"
				if shell == "fish" {
					command = "source " + shellQuote(edit.Path) + "; command -s gh"
				}
				args := []string{"-c", command}
				if shell == "fish" {
					args = append([]string{"--no-config"}, args...)
				}
				cmd := exec.Command(binary, args...)
				cmd.Env = []string{"HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "ZDOTDIR=" + home, "PATH=/usr/bin:/bin"}
				out, err := cmd.CombinedOutput()
				if err != nil || strings.TrimSpace(string(out)) != filepath.Join(bin, "gh") {
					t.Fatalf("shell did not resolve the shim: %v, %q", err, out)
				}
				if err := edit.apply(true); err != nil {
					t.Fatal(err)
				}
				b, err := os.ReadFile(edit.Path)
				if err != nil || string(b) != original {
					t.Fatal("uninstall did not preserve original configuration")
				}
				info, err := os.Stat(edit.Path)
				if err != nil || info.Mode().Perm() != 0600 {
					t.Fatal("shell config permissions changed")
				}
			}
		})
	}
}

func TestShellPathPreservesSymlinksAndLaterEdits(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, "dotfile")
	link := filepath.Join(home, ".profile")
	if err := os.WriteFile(target, []byte("# initial\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	edits, err := ShellPaths(home, "sh", nil)
	if err != nil {
		t.Fatal(err)
	}
	edit := edits[0]
	if err := edit.apply(false); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(target, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("# later edit\n")
	f.Close()
	if err := edit.apply(true); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(link); err != nil || string(b) != "# initial\n# later edit\n" {
		t.Fatal("user edits were lost")
	}
	if got, err := os.Readlink(link); err != nil || got != target {
		t.Fatal("dotfile symlink was replaced")
	}
}

func TestChangedPathBlockIsNotOverwritten(t *testing.T) {
	edits, err := ShellPaths(t.TempDir(), "sh", nil)
	if err != nil {
		t.Fatal(err)
	}
	edit := edits[0]
	if err := os.WriteFile(edit.Path, []byte(pathStart+"\n# edited\n"+pathEnd+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, remove := range []bool{true, false} {
		if err := edit.apply(remove); err == nil {
			t.Fatal("user-edited PATH block was overwritten")
		}
	}
}

func TestUninstallDoesNotRecreateDeletedStartupFile(t *testing.T) {
	home := t.TempDir()
	profile := filepath.Join(home, ".bash_profile")
	if err := os.WriteFile(profile, []byte("# user's profile\n"), 0644); err != nil {
		t.Fatal(err)
	}
	edits, err := ShellPaths(home, "bash", nil)
	if err != nil {
		t.Fatal(err)
	}
	edit := edits[0]
	if err := edit.apply(false); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(edit.Path); err != nil {
		t.Fatal(err)
	}
	if err := edit.apply(true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(edit.Path); !os.IsNotExist(err) {
		t.Fatal("uninstall recreated a deleted login file")
	}
}

func TestUninstallPreservesNewDotfileSymlink(t *testing.T) {
	home := t.TempDir()
	edits, err := ShellPaths(home, "sh", nil)
	if err != nil {
		t.Fatal(err)
	}
	edit := edits[0]
	if err := edit.apply(false); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, "managed-profile")
	if err := os.Rename(edit.Path, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, edit.Path); err != nil {
		t.Fatal(err)
	}
	if err := edit.apply(true); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(edit.Path); err != nil || got != target {
		t.Fatal("uninstall replaced a newly introduced symlink")
	}
	if b, err := os.ReadFile(target); err != nil || len(b) != 0 {
		t.Fatal("uninstall must remove only its block from the managed target")
	}
}

func TestShellStartupLocations(t *testing.T) {
	home := t.TempDir()
	profile := filepath.Join(home, ".profile")
	if err := os.WriteFile(profile, nil, 0644); err != nil {
		t.Fatal(err)
	}
	edits, err := ShellPaths(home, "bash", nil)
	if err != nil || !sameFile(edits[0].Path, profile) {
		t.Fatal("bash would shadow an existing login profile")
	}
	for shell, key := range map[string]string{"fish": "XDG_CONFIG_HOME", "zsh": "ZDOTDIR"} {
		dir := t.TempDir()
		edits, err := ShellPaths(home, shell, map[string]string{key: dir})
		if err != nil || !strings.HasPrefix(edits[0].Path, dir+"/") {
			t.Fatal("custom shell config directory ignored")
		}
	}
	if _, err := ShellPaths(home, "unknown", nil); err == nil {
		t.Fatal("unsupported shell must give actionable feedback")
	}
}
