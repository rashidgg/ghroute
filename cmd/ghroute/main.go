package main

import (
	"fmt"
	"os"

	"github.com/rashidgg/ghroute/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "ghroute:", err)
		os.Exit(1)
	}
}
