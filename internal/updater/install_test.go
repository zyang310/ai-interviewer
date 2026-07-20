//go:build darwin

package updater

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBundleFromExecutable covers the path arithmetic that locates the running
// .app bundle from its embedded executable — easy to get subtly wrong (an off
// level in either direction silently points the installer at the wrong path).
func TestBundleFromExecutable(t *testing.T) {
	cases := []struct {
		name    string
		exe     string
		want    string
		wantErr bool
	}{
		{
			name: "standard Applications install",
			exe:  "/Applications/Mogi.app/Contents/MacOS/Mogi",
			want: "/Applications/Mogi.app",
		},
		{
			name: "user-chosen install location",
			exe:  "/Users/pat/Desktop/Mogi.app/Contents/MacOS/Mogi",
			want: "/Users/pat/Desktop/Mogi.app",
		},
		{
			name:    "not inside a .app bundle",
			exe:     "/usr/local/bin/mogi",
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := bundleFromExecutable(tc.exe)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("bundleFromExecutable(%q) = %q, want error", tc.exe, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("bundleFromExecutable(%q) unexpected error: %v", tc.exe, err)
			}
			if got != tc.want {
				t.Errorf("bundleFromExecutable(%q) = %q, want %q", tc.exe, got, tc.want)
			}
		})
	}
}

// TestFindAppBundle covers picking the downloaded .app out of the extracted
// release directory, including ignoring stray non-.app entries and erroring
// when the release contains no bundle at all (a malformed or empty download).
func TestFindAppBundle(t *testing.T) {
	t.Run("finds the app bundle", func(t *testing.T) {
		dir := t.TempDir()
		appPath := filepath.Join(dir, "Mogi.app")
		if err := os.Mkdir(appPath, 0o755); err != nil {
			t.Fatal(err)
		}

		got, err := findAppBundle(dir)
		if err != nil {
			t.Fatalf("findAppBundle: unexpected error: %v", err)
		}
		if got != appPath {
			t.Errorf("findAppBundle() = %q, want %q", got, appPath)
		}
	})

	t.Run("ignores non-.app entries", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hi"), 0o644); err != nil {
			t.Fatal(err)
		}
		appPath := filepath.Join(dir, "Mogi.app")
		if err := os.Mkdir(appPath, 0o755); err != nil {
			t.Fatal(err)
		}

		got, err := findAppBundle(dir)
		if err != nil {
			t.Fatalf("findAppBundle: unexpected error: %v", err)
		}
		if got != appPath {
			t.Errorf("findAppBundle() = %q, want %q", got, appPath)
		}
	})

	t.Run("errors when no .app bundle exists", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := findAppBundle(dir); err == nil {
			t.Error("findAppBundle() with no .app present, want error")
		}
	})
}
