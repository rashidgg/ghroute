package install

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/rashidgg/ghroute/internal/process"
)

func TestHelperChangesAreAtomicWhenGitWriteFails(t *testing.T) {
	for _, operation := range []string{"activate", "off", "uninstall"} {
		t.Run(operation, func(t *testing.T) {
			i, _ := fixture(t)
			ctx := context.Background()
			for _, helper := range []string{"", "!original-helper"} {
				if err := i.run(ctx, "git", "config", "--global", "--add", helperKey, helper); err != nil {
					t.Fatal(err)
				}
			}
			if operation != "activate" {
				if err := i.Apply(ctx); err != nil {
					t.Fatal(err)
				}
			}
			path, err := globalConfigPath(i.Env)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			originalRunner := i.Runner
			failed := false
			i.Runner = fakeRunner(func(ctx context.Context, c process.Command) ([]byte, error) {
				// Fail after activation's reset, or after restoration's unset-all.
				for n, arg := range c.Args {
					if c.Name == "git" && arg == "--add" && len(c.Args) > n+2 && c.Args[n+1] == helperKey {
						if operation != "activate" || c.Args[n+2] == i.Plan.Helper {
							failed = true
							return nil, errors.New("injected Git config write failure")
						}
					}
				}
				return originalRunner.Run(ctx, c)
			})
			run := i.Apply
			if operation == "off" {
				run = i.Off
			} else if operation == "uninstall" {
				run = i.Restore
			}
			if err := run(ctx); err == nil || !failed {
				t.Fatal("did not exercise the intended failure boundary")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("failed operation changed the live Git configuration")
			}
			if _, err := os.Stat(path + ".lock"); !os.IsNotExist(err) {
				t.Fatal("failed operation left its lock behind")
			}
			i.Runner = originalRunner
			if operation == "activate" {
				if err := i.Apply(ctx); err != nil {
					t.Fatal(err)
				}
			}
			if err := i.Restore(ctx); err != nil {
				t.Fatal(err)
			}
			helpers, err := i.helpers(ctx, path)
			if err != nil || !reflect.DeepEqual(helpers, []string{"", "!original-helper"}) {
				t.Fatal("retry lost original helpers or retained a stray reset")
			}
		})
	}
}

func TestGitTransactionPreservesSymlinkModeAndHonorsExistingLock(t *testing.T) {
	i, _ := fixture(t)
	dir := t.TempDir()
	target, link := filepath.Join(dir, "dotfile"), filepath.Join(dir, ".gitconfig")
	before := []byte("# user's configuration\n[user]\n\tname = Example\n")
	if err := os.WriteFile(target, before, 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	i.Env = append(i.Env, "GIT_CONFIG_GLOBAL="+link)
	lock := target + ".lock"
	if err := os.WriteFile(lock, []byte("another Git process"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := i.editGitConfig("", func(_, _ string) error {
		t.Fatal("edited configuration despite an existing lock")
		return nil
	}); err == nil {
		t.Fatal("existing Git lock ignored")
	}
	if b, err := os.ReadFile(lock); err != nil || string(b) != "another Git process" {
		t.Fatal("another process's lock was modified")
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if err := i.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(link); err != nil || got != target {
		t.Fatal("Git configuration symlink replaced")
	}
	if info, err := os.Stat(target); err != nil || info.Mode().Perm() != 0640 {
		t.Fatal("Git configuration permissions changed")
	}
	if b, err := os.ReadFile(target); err != nil || !bytes.HasPrefix(b, before) {
		t.Fatal("unrelated configuration was changed")
	}
}

func TestOffEditsOnlyTheRecordedGlobalConfigFile(t *testing.T) {
	i, _ := fixture(t)
	home := t.TempDir()
	xdg := filepath.Join(home, ".config/git/config")
	if err := os.MkdirAll(filepath.Dir(xdg), 0755); err != nil {
		t.Fatal(err)
	}
	i.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1"}
	ctx := context.Background()
	if err := i.run(ctx, "git", "config", "--file", xdg, "--add", helperKey, "!xdg-helper"); err != nil {
		t.Fatal(err)
	}
	if err := i.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	// A newer .gitconfig becomes Git's default write destination after install.
	main := filepath.Join(home, ".gitconfig")
	if err := i.run(ctx, "git", "config", "--file", main, "--add", helperKey, "!main-helper"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(main)
	if err != nil {
		t.Fatal(err)
	}
	if err := i.Off(ctx); err != nil {
		t.Fatal(err)
	}
	if after, err := os.ReadFile(main); err != nil || !bytes.Equal(before, after) {
		t.Fatal("off edited a different global file than the one it activated")
	}
	if helpers, err := i.helpers(ctx, xdg); err != nil || !reflect.DeepEqual(helpers, []string{"!xdg-helper"}) {
		t.Fatal("off moved or duplicated helpers across global files")
	}
}

func TestUnchangedGitTransactionDoesNotCreateMissingConfig(t *testing.T) {
	i, _ := fixture(t)
	path, err := globalConfigPath(i.Env)
	if err != nil {
		t.Fatal(err)
	}
	if err := i.editGitConfig("", func(_, _ string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{path, path + ".lock"} {
		if _, err := os.Lstat(name); !os.IsNotExist(err) {
			t.Fatal("no-op transaction created a configuration file or retained its lock")
		}
	}
}
