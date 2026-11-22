package gofast

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

// TestSourceHygiene keeps invisible and look-alike characters out of the
// Go source: a test that needs one writes it as an escape. It looks for
// characters of categories Cf, Co, Cc (but tab and newline), Zs (but the
// space), Zl and Zp, the ligatures U+FB00 to U+FB06, and U+FFFD.
func TestSourceHygiene(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, r := range string(b) {
			bad := unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Co, r) ||
				unicode.Is(unicode.Cc, r) && r != '\t' && r != '\n' ||
				unicode.Is(unicode.Zs, r) && r != ' ' ||
				unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) ||
				r >= 0xFB00 && r <= 0xFB06 || r == 0xFFFD
			if bad {
				t.Errorf("%s: byte %d: literal %U", path, i, r)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
