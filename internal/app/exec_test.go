package app

import (
	"reflect"
	"testing"
)

func TestTokenSelectionPrecedesArgumentTerminator(t *testing.T) {
	args := []string{"auth", "token", "--"}
	want := []string{"auth", "token", "--user", "rashidgg", "--"}
	if got := tokenArgs(args, "rashidgg"); !reflect.DeepEqual(got, want) {
		t.Fatal("selected token user was placed after argument terminator")
	}
	if !reflect.DeepEqual(args, []string{"auth", "token", "--"}) {
		t.Fatal("original arguments mutated")
	}
}
