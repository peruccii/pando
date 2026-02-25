package gitprs

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	gitHubOwnerRegex = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$`)
	gitHubRepoRegex  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
)

// ParseGitHubRemoteURL resolve owner/repo a partir de URL de remote origin.
func ParseGitHubRemoteURL(raw string) (string, string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", false
	}

	if strings.HasPrefix(trimmed, "git@github.com:") {
		return parseGitHubPath(strings.TrimPrefix(trimmed, "git@github.com:"))
	}

	parsedURL, err := url.Parse(trimmed)
	if err != nil {
		return "", "", false
	}

	host := strings.ToLower(strings.TrimSpace(parsedURL.Hostname()))
	if host != "github.com" {
		return "", "", false
	}

	return parseGitHubPath(strings.TrimPrefix(parsedURL.Path, "/"))
}

// NormalizeOwnerRepo valida owner/repo para uso seguro no backend.
func NormalizeOwnerRepo(owner, repo string) (string, string, error) {
	normalizedOwner := strings.TrimSpace(owner)
	normalizedRepo := strings.TrimSpace(repo)

	if normalizedOwner == "" || normalizedRepo == "" {
		return "", "", fmt.Errorf("owner e repo sao obrigatorios")
	}

	normalizedRepo = strings.TrimSuffix(normalizedRepo, ".git")
	normalizedOwner = strings.TrimSpace(normalizedOwner)
	normalizedRepo = strings.TrimSpace(normalizedRepo)
	if normalizedOwner == "" || normalizedRepo == "" {
		return "", "", fmt.Errorf("owner/repo manual invalido")
	}

	if !gitHubOwnerRegex.MatchString(normalizedOwner) {
		return "", "", fmt.Errorf("owner invalido: %q", normalizedOwner)
	}
	if !gitHubRepoRegex.MatchString(normalizedRepo) {
		return "", "", fmt.Errorf("repo invalido: %q", normalizedRepo)
	}
	if strings.Contains(normalizedRepo, "..") {
		return "", "", fmt.Errorf("repo invalido: %q", normalizedRepo)
	}

	return normalizedOwner, normalizedRepo, nil
}

// SameOwnerRepo compara owner/repo ignorando case.
func SameOwnerRepo(ownerA, repoA, ownerB, repoB string) bool {
	return strings.EqualFold(strings.TrimSpace(ownerA), strings.TrimSpace(ownerB)) &&
		strings.EqualFold(strings.TrimSpace(repoA), strings.TrimSpace(repoB))
}

func parseGitHubPath(pathValue string) (string, string, bool) {
	trimmedPath := strings.TrimSpace(pathValue)
	trimmedPath = strings.TrimPrefix(trimmedPath, "/")
	trimmedPath = strings.TrimSuffix(trimmedPath, "/")
	trimmedPath = strings.TrimSuffix(trimmedPath, ".git")
	if trimmedPath == "" {
		return "", "", false
	}

	segments := strings.Split(trimmedPath, "/")
	if len(segments) != 2 {
		return "", "", false
	}

	owner, repo, err := NormalizeOwnerRepo(segments[0], segments[1])
	if err != nil {
		return "", "", false
	}
	return owner, repo, true
}
