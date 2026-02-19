package terminal

import "testing"

func TestDetectCLIFromText_ByBinaryName(t *testing.T) {
	got := detectCLIFromText("/opt/homebrew/bin/gemini")
	if got != CLIGemini {
		t.Fatalf("expected %q, got %q", CLIGemini, got)
	}
}

func TestDetectCLIFromText_ByNodeCommandLine(t *testing.T) {
	got := detectCLIFromText("node /opt/homebrew/lib/node_modules/@google/gemini-cli/bin/gemini.js --resume")
	if got != CLIGemini {
		t.Fatalf("expected %q, got %q", CLIGemini, got)
	}
}

func TestDetectCLIFromText_UnknownProcess(t *testing.T) {
	got := detectCLIFromText("/bin/fish")
	if got != CLINone {
		t.Fatalf("expected empty CLI type, got %q", got)
	}
}
