package terminal

import "testing"

func TestParseSystemProfilerFontFamilies(t *testing.T) {
	raw := []byte(`{
		"SPFontsDataType": [
			{
				"enabled": "yes",
				"typefaces": [
					{"family": "JetBrains Mono", "enabled": "yes"},
					{"family": "Menlo", "enabled": "yes"}
				]
			},
			{
				"enabled": "yes",
				"typefaces": [
					{"family": "jetbrains mono", "enabled": "yes"},
					{"family": ".SFNS-Regular", "enabled": "yes"},
					{"family": "Fira Code", "enabled": "no"}
				]
			},
			{
				"enabled": "no",
				"typefaces": [
					{"family": "Should Be Ignored", "enabled": "yes"}
				]
			}
		]
	}`)

	families, err := parseSystemProfilerFontFamilies(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(families) != 2 {
		t.Fatalf("expected 2 families, got %d (%v)", len(families), families)
	}

	if families[0] != "JetBrains Mono" {
		t.Fatalf("expected JetBrains Mono at index 0, got %q", families[0])
	}

	if families[1] != "Menlo" {
		t.Fatalf("expected Menlo at index 1, got %q", families[1])
	}
}

func TestPrioritizeTerminalFamilies(t *testing.T) {
	families := []string{"Hack", "Menlo", "SF Mono", "Courier New", "Another Font"}
	ordered := prioritizeTerminalFamilies(families)

	if len(ordered) != len(families) {
		t.Fatalf("expected %d families, got %d", len(families), len(ordered))
	}

	if ordered[0] != "SF Mono" {
		t.Fatalf("expected SF Mono at index 0, got %q", ordered[0])
	}

	if ordered[1] != "Menlo" {
		t.Fatalf("expected Menlo at index 1, got %q", ordered[1])
	}
}
