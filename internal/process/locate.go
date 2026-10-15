package process

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FindGH locates the official CLI without following our gh shim back into ghroute.
// Homebrew builds set RealGH to their dependency's stable opt path.
func FindGH(env []string, self string) (string, error) {
	candidates := []string{RealGH, "/usr/local/opt/gh/bin/gh"}
	for _, entry := range env {
		if value, ok := strings.CutPrefix(entry, "PATH="); ok {
			for _, dir := range filepath.SplitList(value) {
				// Do not execute an implicit current-directory binary.
				if filepath.IsAbs(dir) {
					candidates = append(candidates, filepath.Join(dir, "gh"))
				}
			}
		}
	}
	return findGH(candidates, self)
}

func findGH(candidates []string, self string) (string, error) {
	selfInfo, _ := os.Stat(self)
	for _, path := range candidates {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil || filepath.Base(resolved) == "ghroute" {
			continue
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
			continue
		}
		if selfInfo != nil && os.SameFile(info, selfInfo) {
			continue
		}
		// Retain the stable opt path, rather than a versioned Cellar path.
		return path, nil
	}
	return "", fmt.Errorf("official gh executable not found; install it with brew install gh")
}
