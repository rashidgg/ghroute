package install

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rashidgg/ghroute/internal/process"
)

type fakeRunner func(context.Context, process.Command) ([]byte, error)

func (f fakeRunner) Run(ctx context.Context, c process.Command) ([]byte, error) { return f(ctx, c) }

func fixture(t *testing.T) (Installer, *[]string) {
	t.Helper()
	dir := t.TempDir()
	p := Paths{RealGH: filepath.Join(dir, "real-gh"), HomebrewGH: filepath.Join(dir, "brew-gh"), Binary: filepath.Join(dir, "local", "ghroute"), Link: filepath.Join(dir, "local", "gh"), State: filepath.Join(dir, "state", "install.json"), Brew: "brew"}
	if err := os.WriteFile(p.RealGH, []byte("fixture executable"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real-gh", p.HomebrewGH); err != nil {
		t.Fatal(err)
	}
	commands := []string{}
	source := filepath.Join(dir, "ghroute")
	if err := os.WriteFile(source, []byte("fixture ghroute executable"), 0755); err != nil {
		t.Fatal(err)
	}
	i := Installer{Env: append(os.Environ(), "GIT_CONFIG_GLOBAL="+filepath.Join(dir, "gitconfig"), "GIT_CONFIG_NOSYSTEM=1"), Executable: source, Plan: NewPlan(p)}
	i.Runner = fakeRunner(func(ctx context.Context, c process.Command) ([]byte, error) {
		commands = append(commands, c.Name+" "+strings.Join(c.Args, " "))
		switch c.Name {
		case "brew":
			return nil, nil
		default:
			return (process.ExecRunner{}).Run(ctx, c)
		}
	})
	return i, &commands
}

func TestInstallPlan(t *testing.T) {
	p := NewPlan(DefaultPaths("/Users/example"))
	if p.Paths.RealGH != process.RealGH || p.Paths.HomebrewGH == p.Paths.RealGH {
		t.Fatal("real gh must bypass Homebrew entrypoint")
	}
	if strings.Join(p.GitCommands[0], " ") != "config --global credential.https://github.com.useHttpPath true" {
		t.Fatal("install plan lacks useHttpPath")
	}
	if p.GitCommands[1][len(p.GitCommands[1])-1] != "" {
		t.Fatal("install plan needs empty helper")
	}
	if p.Helper != "!/Users/example/.local/bin/ghroute credential" {
		t.Fatal("helper must use absolute ghroute path")
	}
}

func TestInstallAndUninstallPreserveConfiguration(t *testing.T) {
	i, commands := fixture(t)
	// State written by the original installer must keep its Homebrew integration.
	if err := writeState(i.Plan.Paths.State, State{OriginalTarget: "real-gh", Helper: i.Plan.Helper}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	git := func(args ...string) string {
		t.Helper()
		b, err := i.Runner.Run(ctx, process.Command{Name: "git", Args: args, Env: i.Env})
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	git("config", "--global", "--add", helperKey, "")
	git("config", "--global", "--add", helperKey, "!old-gh auth git-credential")
	git("config", "--global", "--add", "url.git@github.com-rashidrefs:Ref-Platform/.insteadOf", "https://github.com/Ref-Platform/")
	git("config", "--global", "--add", "includeIf.hasconfig:remote.*.url:https://github.com/Ref-Platform/**.path", "/identity")
	protected := git("config", "--global", "--get-regexp", "^(url\\.|includeif\\.)")
	if err := i.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if err := i.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	p := i.Plan.Paths
	if got, err := os.Readlink(p.HomebrewGH); err != nil || got != p.Link {
		t.Fatal("Homebrew link not routed")
	}
	if got, err := os.Readlink(p.Link); err != nil || got != p.Binary {
		t.Fatal("local gh link not routed")
	}
	helper := git("config", "--global", "--get-all", helperKey)
	if strings.Count(helper, i.Plan.Helper) != 1 || !strings.HasPrefix(helper, "\n!old-gh auth git-credential\n\n") {
		t.Fatal("helper order or idempotence broken")
	}
	if got := git("config", "--global", "credential.https://github.com.useHttpPath"); got != "true\n" {
		t.Fatal("useHttpPath not installed")
	}
	info, err := os.Stat(p.State)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("installation state must be private")
	}
	if err := i.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(p.HomebrewGH); err != nil || got != "real-gh" {
		t.Fatal("original Homebrew symlink not restored")
	}
	if _, err := os.Lstat(p.Link); !os.IsNotExist(err) {
		t.Fatal("local gh symlink not removed")
	}
	helper = git("config", "--global", "--get-all", helperKey)
	if helper != "\n!old-gh auth git-credential\n" {
		t.Fatal("uninstall changed unrelated helpers")
	}
	if got := git("config", "--global", "--get-regexp", "^(url\\.|includeif\\.)"); got != protected {
		t.Fatal("protected git rules changed")
	}
	if !strings.Contains(strings.Join(*commands, "\n"), "brew pin gh") || !strings.Contains(strings.Join(*commands, "\n"), "brew unpin gh") {
		t.Fatal("missing brew pin/unpin")
	}
	if err := i.Restore(ctx); err != nil {
		t.Fatal("uninstall must be idempotent")
	}
}

func TestNewInstallUsesPathWithoutChangingHomebrew(t *testing.T) {
	i, commands := fixture(t)
	home := filepath.Dir(i.Executable)
	var err error
	i.PathEdits, err = ShellPaths(home, "bash", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for n := 0; n < 2; n++ {
		if err := i.Apply(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := os.Readlink(i.Plan.Paths.HomebrewGH); err != nil || got != "real-gh" {
		t.Fatal("fresh install changed the official Homebrew entrypoint")
	}
	if b, err := os.ReadFile(i.Plan.Paths.Binary); err != nil || string(b) != "fixture ghroute executable" {
		t.Fatal("packaged executable was not copied")
	}
	if got, err := os.Readlink(i.Plan.Paths.Link); err != nil || got != i.Plan.Paths.Binary {
		t.Fatal("local gh does not route to ghroute")
	}
	for _, edit := range i.PathEdits {
		b, err := os.ReadFile(edit.Path)
		if err != nil || strings.Count(string(b), pathStart) != 1 {
			t.Fatal("PATH activation missing or duplicated")
		}
	}
	if err := i.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	for _, edit := range i.PathEdits {
		if _, err := os.Stat(edit.Path); !os.IsNotExist(err) {
			t.Fatal("created shell file not removed on uninstall")
		}
	}
	for _, command := range *commands {
		if strings.HasPrefix(command, "brew ") || strings.HasPrefix(command, "go ") {
			t.Fatal("packaged activation must not require the compiler or modify Homebrew")
		}
	}
}

func TestHomebrewAliasFollowsUpgrades(t *testing.T) {
	i, _ := fixture(t)
	i.PackageBinary = filepath.Join(filepath.Dir(i.Executable), "stable-ghroute")
	if err := os.Symlink(i.Executable, i.PackageBinary); err != nil {
		t.Fatal(err)
	}
	if err := i.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(i.Plan.Paths.Binary); err != nil || got != i.PackageBinary {
		t.Fatal("installed binary is not a stable package alias")
	}
	upgraded := filepath.Join(filepath.Dir(i.Executable), "upgraded")
	if err := os.WriteFile(upgraded, []byte("new version"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := replaceLink(i.PackageBinary, upgraded); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(i.Plan.Paths.Link); err != nil || string(b) != "new version" {
		t.Fatal("routing did not follow the Homebrew upgrade")
	}
	if err := i.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(i.Plan.Paths.Binary); !os.IsNotExist(err) {
		t.Fatal("managed binary alias not removed")
	}
	if _, err := os.Stat(upgraded); err != nil {
		t.Fatal("uninstall removed the package executable")
	}
}

func TestOffAndOnPreserveLegacyActivationAcrossOfficialGHUpgrade(t *testing.T) {
	i, _ := fixture(t)
	ctx := context.Background()
	p := i.Plan.Paths
	if err := writeState(p.State, State{OriginalTarget: "real-gh", Helper: i.Plan.Helper}); err != nil {
		t.Fatal(err)
	}
	if err := i.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 2; n++ {
		if err := i.Off(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if disabled, err := RoutingDisabled(p.State); err != nil || !disabled {
		t.Fatal("off did not persist disabled routing")
	}
	if got, err := os.Readlink(p.HomebrewGH); err != nil || got != "real-gh" {
		t.Fatal("off did not restore the official Homebrew CLI")
	}
	if got, err := os.Readlink(p.Link); err != nil || got != p.Binary {
		t.Fatal("off removed the shim needed by already-open shells")
	}
	// gh is unpinned while off. Homebrew can replace the official target then.
	upgraded := filepath.Join(filepath.Dir(p.RealGH), "new-official-gh")
	if err := os.WriteFile(upgraded, []byte("upgraded official CLI"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := replaceLink(p.HomebrewGH, upgraded); err != nil {
		t.Fatal(err)
	}
	i.Plan.Paths.RealGH = upgraded
	if err := i.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if disabled, err := RoutingDisabled(p.State); err != nil || disabled {
		t.Fatal("on did not clear disabled routing")
	}
	if got, err := os.Readlink(p.HomebrewGH); err != nil || got != p.Link {
		t.Fatal("on lost the previous legacy integration")
	}
	if err := i.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(p.HomebrewGH); err != nil || got != upgraded {
		t.Fatal("uninstall restored a stale Homebrew version")
	}
}

func TestUnrelatedLocalBinaryIsNotOverwritten(t *testing.T) {
	i, _ := fixture(t)
	if err := os.MkdirAll(filepath.Dir(i.Plan.Paths.Binary), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(i.Plan.Paths.Binary, []byte("user executable"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := i.Apply(context.Background()); err == nil {
		t.Fatal("unrelated executable was overwritten")
	}
}

func TestStandaloneCanReinstallAfterUninstall(t *testing.T) {
	i, _ := fixture(t)
	i.Executable += "-compiled"
	build := exec.Command("go", "build", "-o", i.Executable, "./cmd/ghroute")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build test executable: %v, %s", err, out)
	}
	ctx := context.Background()
	if err := i.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if err := i.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	previous := i.Executable
	i.Executable += "-new-build"
	b, err := os.ReadFile(previous)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(i.Executable, b, 0755); err != nil {
		t.Fatal(err)
	}
	if err := i.Apply(ctx); err != nil {
		t.Fatalf("cannot reactivate using a different source build: %v", err)
	}
}

func TestMissingRealGHRefusesInstall(t *testing.T) {
	i, commands := fixture(t)
	if err := os.Remove(i.Plan.Paths.RealGH); err != nil {
		t.Fatal(err)
	}
	if err := i.Apply(context.Background()); err == nil {
		t.Fatal("missing real gh did not prevent installation")
	}
	if len(*commands) != 0 {
		t.Fatal("install mutated system before checking real gh")
	}
}

func TestUnrelatedLocalGHIsNotOverwritten(t *testing.T) {
	i, _ := fixture(t)
	if err := os.MkdirAll(filepath.Dir(i.Plan.Paths.Link), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(i.Plan.Paths.Link, []byte("user file"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := i.Apply(context.Background()); err == nil {
		t.Fatal("unrelated local gh was overwritten")
	}
}
