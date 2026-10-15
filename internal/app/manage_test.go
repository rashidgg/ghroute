package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoutingCanBeTurnedOffAndOn(t *testing.T) {
	dir := t.TempDir()
	realGH := filepath.Join(dir, "official-gh")
	if err := os.WriteFile(realGH, []byte("#!/bin/sh\nprintf 'official gh fixture\\n'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "ghroute")
	build := exec.Command("go", "build", "-ldflags", "-X github.com/rashidgg/ghroute/internal/process.RealGH="+realGH, "-o", binary, "./cmd/ghroute")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build test executable: %v, %s", err, out)
	}
	for _, activate := range []string{"on", "install"} {
		t.Run(activate+"-off", func(t *testing.T) {
			home := t.TempDir()
			env := append(os.Environ(), "HOME="+home, "SHELL=/bin/bash", "GIT_CONFIG_GLOBAL="+filepath.Join(home, ".gitconfig"), "GIT_CONFIG_NOSYSTEM=1")
			localBinary := filepath.Join(home, ".local/bin/ghroute")
			shim := filepath.Join(home, ".local/bin/gh")
			profile := filepath.Join(home, ".bash_profile")
			state := filepath.Join(home, ".local/state/ghroute/install.json")
			run := func(program string, args ...string) string {
				t.Helper()
				cmd := exec.Command(program, args...)
				cmd.Env = env
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("command failed: %v, %s", err, out)
				}
				return string(out)
			}
			// A full off/on cycle must work from the retained standalone binary.
			for cycle := 0; cycle < 2; cycle++ {
				program := binary
				if cycle > 0 {
					program = localBinary
				}
				if out := run(program, activate, "--shell", "bash"); !strings.Contains(out, "ghroute is on") {
					t.Fatal("activation did not report routing on")
				}
				if out := run(shim, "--version"); out != "official gh fixture\n" {
					t.Fatal("shim did not invoke the official CLI")
				}
				if out := run(localBinary, "off"); !strings.Contains(out, "ghroute is off") {
					t.Fatal("deactivation did not report routing off")
				}
				for _, path := range []string{shim, profile, state, localBinary} {
					if _, err := os.Stat(path); err != nil {
						t.Fatal("off removed an integration needed to keep ghroute available")
					}
				}
				if out := run(shim, "api", "user"); out != "official gh fixture\n" {
					t.Fatal("off still attempted account discovery or routing")
				}
				if out := run(localBinary, "credential", "get"); out != "official gh fixture\n" {
					t.Fatal("off still routed the credential helper")
				}
				b, err := os.ReadFile(filepath.Join(home, ".gitconfig"))
				if err != nil || strings.Contains(string(b), "ghroute credential") {
					t.Fatal("off left the routing credential helper enabled")
				}
				run(localBinary, "off") // Already off is a successful no-op.
			}
			run(localBinary, "uninstall")
			for _, path := range []string{shim, profile, state} {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatal("uninstall left a managed integration behind")
				}
			}
		})
	}
}
