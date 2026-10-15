package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func (i Installer) disabledPath() string {
	return filepath.Join(filepath.Dir(i.Plan.Paths.State), "disabled")
}

// Setup is the package install entrypoint. Homebrew runs its vendor uninstall
// command during upgrades too, without passing upgrade context to that command.
// Keep the explicit off preference separately from the removable ownership
// receipt, so upgrades and reinstalls do not silently turn routing back on.
func (i Installer) Setup(ctx context.Context) error {
	disabled, err := RoutingDisabled(i.Plan.Paths.State)
	if err != nil {
		return err
	}
	if b, err := os.ReadFile(i.disabledPath()); err == nil {
		if string(b) != "off\n" {
			return fmt.Errorf("invalid ghroute activation preference")
		}
		disabled = true
	} else if !os.IsNotExist(err) {
		return err
	}
	if disabled {
		// Migrate older disabled receipts before a failed setup can remove
		// them during rollback.
		if err := os.MkdirAll(filepath.Dir(i.disabledPath()), 0700); err != nil {
			return err
		}
		if err := atomicWrite(i.disabledPath(), []byte("off\n"), 0600); err != nil {
			return err
		}
	}
	if err := i.apply(ctx, disabled); err != nil {
		// Homebrew removes a failed installer payload. Undo any partial setup
		// first; if cleanup cannot complete, retain a working recovery binary
		// rather than leaving Git referencing a deleted package executable.
		if cleanup := i.Restore(ctx); cleanup != nil {
			if target, linkErr := os.Readlink(i.Plan.Paths.Binary); linkErr == nil && target == i.PackageBinary {
				i.PackageBinary = ""
				if copyErr := i.installBinary(); copyErr != nil {
					return errors.Join(err, cleanup, copyErr)
				}
			}
			return errors.Join(err, fmt.Errorf("partial setup retained; retry ghroute uninstall: %w", cleanup))
		}
		return err
	}
	return nil
}
