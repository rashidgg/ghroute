package credential

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rashidgg/ghroute/internal/model"
)

func TestCredentialTargets(t *testing.T) {
	for _, input := range []string{
		"protocol=https\nhost=github.com:443\npath=Owner/Repo.git\n\n",
		"protocol=https\nhost=github.com\npath=/Owner/Repo.git\n\n",
		"url=ssh://git@github.com/Owner/Repo.git\n\n",
		"url=git@github.com:Owner/Repo.git\n\n",
		"url=https://someone@github.com/Owner/Repo.git\n\n",
		"url=ssh://git@github.com-rashidrefs/Owner/Repo.git\n\n",
	} {
		r, err := Read(strings.NewReader(input))
		if err != nil {
			t.Fatal(err)
		}
		if r.Target() != (model.Target{Host: "github.com", Owner: "Owner", Repo: "Repo"}) {
			t.Fatal("incorrect credential target")
		}
		if r.Replay() != input {
			t.Fatal("credential record did not round trip")
		}
	}
}

func TestRecordBoundaryAndReply(t *testing.T) {
	r, err := Read(strings.NewReader("protocol=https\nhost=github.com\n\nignored=value\n"))
	if err != nil || len(r.Lines) != 2 {
		t.Fatal("record boundary not respected")
	}
	r, err = Read(strings.NewReader("host=github.com"))
	if err != nil || !strings.HasSuffix(r.Replay(), "\n\n") {
		t.Fatal("missing terminating blank line")
	}
	var out bytes.Buffer
	const token = "unit-test-placeholder"
	if err := Write(&out, token); err != nil {
		t.Fatal("write failed")
	}
	if out.String() != "username=x-access-token\npassword="+token+"\n\n" {
		t.Fatal("invalid credential response")
	}
	if err := Write(&out, "bad\nvalue"); err == nil {
		t.Fatal("newline injection accepted")
	}
	if _, err := Read(strings.NewReader("invalid\n\n")); err == nil {
		t.Fatal("malformed record accepted")
	}
}
