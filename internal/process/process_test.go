package process

import (
	"strings"
	"testing"
)

func TestTokenEnvironment(t *testing.T) {
	env := []string{"PATH=/bin", "GH_TOKEN=old", "GITHUB_TOKEN=old", "GH_ENTERPRISE_TOKEN=old", "GITHUB_ENTERPRISE_TOKEN=old", "GHROUTE_ROUTER=0"}
	clean := CleanEnv(env)
	if len(clean) != 1 || clean[0] != "PATH=/bin" {
		t.Fatal("token variables were not cleared")
	}
	out := TokenEnv(env, "unit-placeholder")
	if len(out) != 3 || out[2] != "GHROUTE_ROUTER=1" || !strings.HasPrefix(out[1], "GH_TOKEN=") {
		t.Fatal("invalid child environment")
	}
	if env[1] != "GH_TOKEN=old" {
		t.Fatal("parent environment mutated")
	}
}
