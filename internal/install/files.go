package install

import (
	"debug/buildinfo"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func isGhrouteBinary(path string) bool {
	info, err := buildinfo.ReadFile(path)
	return err == nil && info.Path == "github.com/rashidgg/ghroute/cmd/ghroute"
}

// ManagedBinary is set to "self" in Homebrew release builds. The package's
// installer refreshes the local alias to its staged executable on each upgrade.
// An absolute path remains supported for older formula builds.
var ManagedBinary string

func atomicWrite(path string, b []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".ghroute-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func sameFile(a, b string) bool {
	ai, ae := os.Stat(a)
	bi, be := os.Stat(b)
	return ae == nil && be == nil && os.SameFile(ai, bi)
}

func (i Installer) installBinary() error {
	p := i.Plan.Paths
	if i.PackageBinary != "" {
		if !filepath.IsAbs(i.PackageBinary) || !sameFile(i.Executable, i.PackageBinary) {
			return fmt.Errorf("Homebrew executable does not match its managed path; reinstall with brew")
		}
		if filepath.Clean(i.PackageBinary) == filepath.Clean(p.Binary) {
			// A recovery copy may retain the package build marker. Never replace
			// that executable with a symlink pointing to itself.
			return nil
		}
		return replaceLink(p.Binary, i.PackageBinary)
	}
	if info, err := os.Lstat(p.Binary); err == nil && info.Mode().IsRegular() {
		if sameFile(i.Executable, p.Binary) {
			return nil
		}
	}
	source, err := os.Open(i.Executable)
	if err != nil {
		return err
	}
	defer source.Close()
	f, err := os.CreateTemp(filepath.Dir(p.Binary), ".ghroute-install-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = io.Copy(f, source); err != nil {
		f.Close()
		return err
	}
	if err = f.Chmod(0755); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), p.Binary)
}
