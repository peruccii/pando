package gitpanel

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGetCommitDetails(t *testing.T) {
	repoRoot := mustInitTestRepo(t)
	svc := NewService(nil)

	// Create a new commit with multiple file changes
	filePath1 := filepath.Join(repoRoot, "file1.txt")
	if err := os.WriteFile(filePath1, []byte("content1"), 0o644); err != nil {
		t.Fatalf("failed to create file1: %v", err)
	}
	filePath2 := filepath.Join(repoRoot, "file2.txt")
	if err := os.WriteFile(filePath2, []byte("content2"), 0o644); err != nil {
		t.Fatalf("failed to create file2: %v", err)
	}

	runGitOrFail(t, repoRoot, "add", ".")
	runGitOrFail(t, repoRoot, "commit", "-m", "second commit")

	// Get the hash of the last commit
	out, _, _, err := runGitWithInput(context.Background(), 5*time.Second, "", "-C", repoRoot, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("failed to get HEAD hash: %v", err)
	}
	commitHash := strings.TrimSpace(out)

	details, err := svc.GetCommitDetails(repoRoot, commitHash)
	if err != nil {
		t.Fatalf("GetCommitDetails failed: %v", err)
	}

	if details.Hash != commitHash {
		t.Errorf("expected hash %q, got %q", commitHash, details.Hash)
	}

	if len(details.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(details.Files))
	}

	fileMap := make(map[string]string)
	for _, f := range details.Files {
		fileMap[f.Path] = f.Status
	}

	if status, ok := fileMap["file1.txt"]; !ok || status != "A" {
		t.Errorf("expected file1.txt to be Added (A), got %q", status)
	}
	if status, ok := fileMap["file2.txt"]; !ok || status != "A" {
		t.Errorf("expected file2.txt to be Added (A), got %q", status)
	}
}

func TestGetCommitDiff(t *testing.T) {
	repoRoot := mustInitTestRepo(t)
	svc := NewService(nil)

	filePath := filepath.Join(repoRoot, "README.md")
	if err := os.WriteFile(filePath, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatalf("failed to modify file: %v", err)
	}

	runGitOrFail(t, repoRoot, "add", ".")
	runGitOrFail(t, repoRoot, "commit", "-m", "modify readme")

	out, _, _, err := runGitWithInput(context.Background(), 5*time.Second, "", "-C", repoRoot, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("failed to get HEAD hash: %v", err)
	}
	commitHash := strings.TrimSpace(out)

	// In TestGetCommitDiff, I need to pass commitHash as the 3rd argument, not 2nd.
	// Correcting signature: GetCommitDiff(repoRoot, filePath, commitHash, 3)
	// But in service.go I defined: GetCommitDiff(repoPath string, filePath string, commitHash string, contextLines int)
	// The call in test was: svc.GetCommitDiff(repoRoot, "README.md", commitHash, 3)
	// This looks correct.
	// Wait, I am fixing syntax error. The multiline string was broken.

	diff, err := svc.GetCommitDiff(repoRoot, "README.md", commitHash, 3)
	if err != nil {
		t.Fatalf("GetCommitDiff failed: %v", err)
	}

	if diff.IsTruncated {
		t.Errorf("expected diff not to be truncated")
	}

	if !strings.Contains(diff.Raw, "+world") {
		t.Errorf("expected diff to contain added line '+world'")
	}
}
