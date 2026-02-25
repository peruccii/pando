package gitprs

import "testing"

func TestParseGitHubRemoteURL(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantOwner string
		wantRepo  string
		wantOK    bool
	}{
		{
			name:      "ssh short syntax",
			raw:       "git@github.com:orch-labs/orch.git",
			wantOwner: "orch-labs",
			wantRepo:  "orch",
			wantOK:    true,
		},
		{
			name:      "https syntax",
			raw:       "https://github.com/orch-labs/orch.git",
			wantOwner: "orch-labs",
			wantRepo:  "orch",
			wantOK:    true,
		},
		{
			name:      "ssh url syntax",
			raw:       "ssh://git@github.com/orch-labs/orch",
			wantOwner: "orch-labs",
			wantRepo:  "orch",
			wantOK:    true,
		},
		{
			name:   "non github host",
			raw:    "https://gitlab.com/orch-labs/orch.git",
			wantOK: false,
		},
		{
			name:   "path traversal style rejected",
			raw:    "https://github.com/orch-labs/../orch.git",
			wantOK: false,
		},
		{
			name:   "extra path segments rejected",
			raw:    "https://github.com/orch-labs/orch/tree/main",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, ok := ParseGitHubRemoteURL(tt.raw)
			if ok != tt.wantOK {
				t.Fatalf("unexpected ok value: got=%v want=%v", ok, tt.wantOK)
			}
			if owner != tt.wantOwner || repo != tt.wantRepo {
				t.Fatalf("unexpected owner/repo: got=%s/%s want=%s/%s", owner, repo, tt.wantOwner, tt.wantRepo)
			}
		})
	}
}

func TestNormalizeOwnerRepo(t *testing.T) {
	owner, repo, err := NormalizeOwnerRepo("orch-labs", "orch.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if owner != "orch-labs" || repo != "orch" {
		t.Fatalf("unexpected normalized owner/repo: got=%s/%s", owner, repo)
	}

	if _, _, err := NormalizeOwnerRepo("", "orch"); err == nil {
		t.Fatalf("expected error for empty owner")
	}
	if _, _, err := NormalizeOwnerRepo("orch_labs", "orch"); err == nil {
		t.Fatalf("expected error for invalid owner")
	}
	if _, _, err := NormalizeOwnerRepo("orch-labs", "../orch"); err == nil {
		t.Fatalf("expected error for invalid repo")
	}
}

func TestSameOwnerRepo(t *testing.T) {
	if !SameOwnerRepo("Orch-Labs", "Orch", "orch-labs", "orch") {
		t.Fatalf("expected owner/repo to match ignoring case")
	}
	if SameOwnerRepo("orch-labs", "orch", "orch-labs", "orch-api") {
		t.Fatalf("expected owner/repo mismatch")
	}
}
