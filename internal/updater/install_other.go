//go:build !darwin

package updater

import (
	"context"
	"fmt"
)

// Install is unsupported outside macOS: Mogi only ships for macOS (see
// CLAUDE.md), and the swap mechanism in install.go (ditto/codesign/spctl/open)
// is Darwin-specific. Callers fall back to OpenReleasePage for a manual install.
func Install(_ context.Context, _ string) error {
	return fmt.Errorf("updater: self-install is only supported on macOS")
}
