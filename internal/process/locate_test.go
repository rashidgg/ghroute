package process

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindGHSkipsWrapperAliasesAndNonExecutables(t *testing.T) {
	dir := t.TempDir()
	self, old, real := filepath.Join(dir, "self"), filepath.Join(dir, "ghroute"), filepath.Join(dir, "official")
	for _, path := range []string{self, old, real} {
		if err := os.WriteFile(path, []byte("executable"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	aliases := []string{filepath.Join(dir, "self-alias"), filepath.Join(dir, "old-alias")}
	for n, target := range []string{self, old} {
		if err := os.Symlink(target, aliases[n]); err != nil {
			t.Fatal(err)
		}
	}
	notExecutable := filepath.Join(dir, "not-executable")
	if err := os.WriteFile(notExecutable, nil, 0644); err != nil {
		t.Fatal(err)
	}
	candidates := append(aliases, notExecutable, dir, filepath.Join(dir, "missing"))
	if _, err := findGH(candidates, self); err == nil {
		t.Fatal("accepted a wrapper, directory, or non-executable")
	}
	stable := filepath.Join(dir, "opt-gh")
	if err := os.Symlink(real, stable); err != nil {
		t.Fatal(err)
	}
	got, err := findGH(append(candidates, stable), self)
	if err != nil || got != stable {
		t.Fatal("did not retain the official CLI's stable path")
	}
}
