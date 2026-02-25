package github

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	gitHubOwnerRegex = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$`)
	gitHubRepoRegex  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
)

func normalizeOwnerRepoForPR(owner, repo string) (string, string, error) {
	normalizedOwner := strings.TrimSpace(owner)
	normalizedRepo := strings.TrimSpace(repo)

	if normalizedOwner == "" || normalizedRepo == "" {
		return "", "", &GitHubError{
			StatusCode: 422,
			Message:    "owner/repo required",
			Type:       "validation",
		}
	}

	normalizedRepo = strings.TrimSuffix(normalizedRepo, ".git")
	normalizedOwner = strings.TrimSpace(normalizedOwner)
	normalizedRepo = strings.TrimSpace(normalizedRepo)

	if normalizedOwner == "" || normalizedRepo == "" {
		return "", "", &GitHubError{
			StatusCode: 422,
			Message:    "owner/repo required",
			Type:       "validation",
		}
	}

	if !gitHubOwnerRegex.MatchString(normalizedOwner) {
		return "", "", &GitHubError{
			StatusCode: 422,
			Message:    fmt.Sprintf("invalid owner %q", normalizedOwner),
			Type:       "validation",
		}
	}
	if !gitHubRepoRegex.MatchString(normalizedRepo) || strings.Contains(normalizedRepo, "..") {
		return "", "", &GitHubError{
			StatusCode: 422,
			Message:    fmt.Sprintf("invalid repo %q", normalizedRepo),
			Type:       "validation",
		}
	}

	return normalizedOwner, normalizedRepo, nil
}
