package terminal

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestEnrichPATH_PreservesAndDeduplicates(t *testing.T) {
	sep := string(filepath.ListSeparator)
	initial := strings.Join([]string{"/usr/bin", "/bin", "/usr/bin"}, sep)

	result := enrichPATH(initial, "darwin")
	parts := filepath.SplitList(result)

	if len(parts) == 0 {
		t.Fatalf("expected non-empty PATH")
	}

	expectedPrefix := []string{"/usr/bin", "/bin"}
	for i, want := range expectedPrefix {
		if parts[i] != want {
			t.Fatalf("expected prefix[%d]=%q, got %q", i, want, parts[i])
		}
	}

	seen := make(map[string]struct{})
	for _, part := range parts {
		if _, exists := seen[part]; exists {
			t.Fatalf("duplicate PATH segment found: %q", part)
		}
		seen[part] = struct{}{}
	}
}

func TestEnrichPATH_AddsHomebrewOnDarwin(t *testing.T) {
	sep := string(filepath.ListSeparator)
	result := enrichPATH("/usr/bin"+sep+"/bin", "darwin")
	parts := filepath.SplitList(result)
	set := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		set[part] = struct{}{}
	}

	for _, required := range []string{"/opt/homebrew/bin", "/usr/local/bin"} {
		if _, ok := set[required]; !ok {
			t.Fatalf("expected %q to be present in PATH, got %v", required, parts)
		}
	}
}
