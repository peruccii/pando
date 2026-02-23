package gitpanel

import "testing"

func TestValidatePatchTextRejectsTraversalPath(t *testing.T) {
	repoRoot := t.TempDir()

	patch := stringsJoinLines(
		"diff --git a/../../etc/passwd b/../../etc/passwd",
		"--- a/../../etc/passwd",
		"+++ b/../../etc/passwd",
		"@@ -1 +1 @@",
		"-x",
		"+y",
	)

	err := validatePatchText(repoRoot, patch)
	if err == nil {
		t.Fatalf("expected traversal path to be rejected")
	}

	bindingErr := AsBindingError(err)
	if bindingErr == nil {
		t.Fatalf("expected binding error, got: %v", err)
	}
	if bindingErr.Code != CodePatchInvalid {
		t.Fatalf("unexpected code: got=%s want=%s", bindingErr.Code, CodePatchInvalid)
	}
}

func TestValidatePatchTextAcceptsRenameAndCopyPaths(t *testing.T) {
	repoRoot := t.TempDir()

	renamePatch := stringsJoinLines(
		`diff --git "a/src/old name.txt" "b/src/new name.txt"`,
		"similarity index 100%",
		"rename from src/old name.txt",
		"rename to src/new name.txt",
		`--- "a/src/old name.txt"`,
		`+++ "b/src/new name.txt"`,
		"@@ -1 +1 @@",
		"-old",
		"+new",
	)
	if err := validatePatchText(repoRoot, renamePatch); err != nil {
		t.Fatalf("expected rename patch to be accepted, got: %v", err)
	}

	copyPatch := stringsJoinLines(
		"diff --git a/src/base.txt b/src/copied.txt",
		"similarity index 100%",
		"copy from src/base.txt",
		"copy to src/copied.txt",
		"--- a/src/base.txt",
		"+++ b/src/copied.txt",
		"@@ -1 +1 @@",
		"-base",
		"+copied",
	)
	if err := validatePatchText(repoRoot, copyPatch); err != nil {
		t.Fatalf("expected copy patch to be accepted, got: %v", err)
	}
}

func TestExtractPatchPathsFailsWhenPatchHasNoHeaders(t *testing.T) {
	_, err := extractPatchPaths("@@ -1 +1 @@\n-a\n+b\n")
	if err == nil {
		t.Fatalf("expected failure for patch without headers")
	}
}

func stringsJoinLines(lines ...string) string {
	if len(lines) == 0 {
		return ""
	}
	joined := lines[0]
	for i := 1; i < len(lines); i++ {
		joined += "\n" + lines[i]
	}
	return joined
}
