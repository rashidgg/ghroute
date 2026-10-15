package install

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rashidgg/ghroute/internal/process"
)

func TestPackageLifecyclePreservesOffAndCleansIntegration(t *testing.T) {
	for _, off := range []bool{false, true} {
		t.Run(map[bool]string{false: "on", true: "off"}[off], func(t *testing.T) {
			i, _ := fixture(t)
			ctx := context.Background()
			i.PackageBinary = i.Executable
			profile := filepath.Join(t.TempDir(), "profile")
			original := "# user shell configuration\n"
			if err := os.WriteFile(profile, []byte(original), 0640); err != nil {
				t.Fatal(err)
			}
			i.PathEdits = []PathEdit{{Path: profile, Block: "\n" + pathStart + "\nexport PATH=test\n" + pathEnd + "\n"}}
			if err := i.Setup(ctx); err != nil {
				t.Fatal(err)
			}
			if off {
				if err := i.Off(ctx); err != nil {
					t.Fatal(err)
				}
			}
			// Cask upgrades and reinstalls run the same uninstall/install pair.
			for cycle := 0; cycle < 2; cycle++ {
				if err := i.Restore(ctx); err != nil {
					t.Fatal(err)
				}
				for _, path := range []string{i.Plan.Paths.Binary, i.Plan.Paths.Link, i.Plan.Paths.State} {
					if _, err := os.Lstat(path); !os.IsNotExist(err) {
						t.Fatal("package removal retained a managed link or ownership receipt")
					}
				}
				if b, err := os.ReadFile(profile); err != nil || string(b) != original {
					t.Fatal("uninstall did not restore the user's shell configuration")
				}
				if err := i.Setup(ctx); err != nil {
					t.Fatal(err)
				}
				if disabled, err := RoutingDisabled(i.Plan.Paths.State); err != nil || disabled != off {
					t.Fatal("package reinstall changed the on/off choice")
				}
				path, _ := globalConfigPath(i.Env)
				b, err := os.ReadFile(path)
				if err != nil || strings.Contains(string(b), "ghroute credential") == off {
					t.Fatal("credential helper does not match the preserved mode")
				}
			}
			if err := i.Apply(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(i.disabledPath()); !os.IsNotExist(err) {
				t.Fatal("explicit on did not clear the off preference")
			}
			if err := i.Restore(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFailedPackageSetupDoesNotLeaveDanglingIntegrations(t *testing.T) {
	i, _ := fixture(t)
	i.PackageBinary = i.Executable
	originalRunner := i.Runner
	i.Runner = fakeRunner(func(ctx context.Context, c process.Command) ([]byte, error) {
		for _, arg := range c.Args {
			if arg == "--add" {
				return nil, errors.New("injected setup failure")
			}
		}
		return originalRunner.Run(ctx, c)
	})
	if err := i.Setup(context.Background()); err == nil {
		t.Fatal("setup should fail")
	}
	for _, path := range []string{i.Plan.Paths.Binary, i.Plan.Paths.Link, i.Plan.Paths.State} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatal("failed setup left integration behind")
		}
	}
}

func TestFailedSetupAndCleanupRetainUsableRecoveryExecutable(t *testing.T) {
	i, _ := fixture(t)
	i.PackageBinary = i.Executable
	if err := os.WriteFile(i.Executable, []byte("#!/bin/sh\nprintf 'recovery executable\\n'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	originalRunner := i.Runner
	failed := false
	i.Runner = fakeRunner(func(ctx context.Context, c process.Command) ([]byte, error) {
		for _, arg := range c.Args {
			if arg == "--add" {
				failed = true
			}
		}
		if failed {
			return nil, errors.New("injected setup and cleanup failure")
		}
		return originalRunner.Run(ctx, c)
	})
	if err := i.Setup(context.Background()); err == nil {
		t.Fatal("setup should fail")
	}
	if err := os.Remove(i.Executable); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(i.Plan.Paths.Binary); err != nil || !info.Mode().IsRegular() {
		t.Fatal("recovery executable was not copied before staging was removed")
	}
	if out, err := exec.Command(i.Plan.Paths.Binary).Output(); err != nil || string(out) != "recovery executable\n" {
		t.Fatal("retained recovery executable is unusable")
	}
	// Recovered package builds still resolve ManagedBinary=self. Activation
	// must not turn that local executable into a self-referencing symlink.
	i.Executable, i.PackageBinary = i.Plan.Paths.Binary, i.Plan.Paths.Binary
	i.Runner = originalRunner
	if err := i.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(i.Plan.Paths.Binary); err != nil || !info.Mode().IsRegular() {
		t.Fatal("recovery activation replaced the binary with a self-link")
	}
}

func TestFailedPackageMigrationPreservesLegacyOffPreference(t *testing.T) {
	i, _ := fixture(t)
	i.PackageBinary = i.Executable
	ctx := context.Background()
	if err := i.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if err := i.Off(ctx); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(i.disabledPath()); err != nil {
		t.Fatal(err)
	}
	// A changed block fails setup before Off gets a chance to save the choice.
	profile := filepath.Join(t.TempDir(), "profile")
	if err := os.WriteFile(profile, []byte(pathStart+"\nchanged by user\n"+pathEnd), 0600); err != nil {
		t.Fatal(err)
	}
	i.PathEdits = []PathEdit{{Path: profile, Block: "original block"}}
	if err := i.Setup(ctx); err == nil {
		t.Fatal("changed PATH block should fail setup")
	}
	i.PathEdits = nil
	i.PackageBinary = i.Executable
	if err := i.Setup(ctx); err != nil {
		t.Fatal(err)
	}
	if off, err := RoutingDisabled(i.Plan.Paths.State); err != nil || !off {
		t.Fatal("failed migration lost the user's off preference")
	}
}
