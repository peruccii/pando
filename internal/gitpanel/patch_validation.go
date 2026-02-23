package gitpanel

import (
	"fmt"
	"strconv"
	"strings"
)

func validatePatchText(repoRoot string, patchText string) error {
	paths, err := extractPatchPaths(patchText)
	if err != nil {
		return NewBindingError(
			CodePatchInvalid,
			"Patch inválido para operação parcial.",
			err.Error(),
		)
	}

	for _, patchPath := range paths {
		if _, pathErr := ensurePathWithinRepo(repoRoot, patchPath); pathErr != nil {
			if bindingErr := AsBindingError(pathErr); bindingErr != nil {
				return NewBindingError(
					CodePatchInvalid,
					"Patch contém caminho fora do escopo permitido.",
					fmt.Sprintf("path=%q | %s", patchPath, nonEmpty(bindingErr.Details, bindingErr.Message)),
				)
			}
			return NewBindingError(
				CodePatchInvalid,
				"Patch contém caminho inválido.",
				fmt.Sprintf("path=%q | %s", patchPath, strings.TrimSpace(pathErr.Error())),
			)
		}
	}
	return nil
}

func extractPatchPaths(patchText string) ([]string, error) {
	lines := strings.Split(patchText, "\n")
	paths := make([]string, 0, 4)
	seen := make(map[string]struct{})

	addPath := func(raw string) {
		path, ok := decodePatchPathToken(raw)
		if !ok {
			return
		}
		if _, exists := seen[path]; exists {
			return
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "diff --git "):
			left, right, ok := parseDiffGitPathPair(strings.TrimPrefix(line, "diff --git "))
			if !ok {
				return nil, fmt.Errorf("header diff --git malformado")
			}
			addPath(left)
			addPath(right)
		case strings.HasPrefix(line, "--- "):
			token, ok := parsePatchHeaderPath(strings.TrimPrefix(line, "--- "))
			if ok {
				addPath(token)
			}
		case strings.HasPrefix(line, "+++ "):
			token, ok := parsePatchHeaderPath(strings.TrimPrefix(line, "+++ "))
			if ok {
				addPath(token)
			}
		case strings.HasPrefix(line, "rename from "):
			addPath(strings.TrimSpace(strings.TrimPrefix(line, "rename from ")))
		case strings.HasPrefix(line, "rename to "):
			addPath(strings.TrimSpace(strings.TrimPrefix(line, "rename to ")))
		case strings.HasPrefix(line, "copy from "):
			addPath(strings.TrimSpace(strings.TrimPrefix(line, "copy from ")))
		case strings.HasPrefix(line, "copy to "):
			addPath(strings.TrimSpace(strings.TrimPrefix(line, "copy to ")))
		}
	}

	if len(paths) == 0 {
		return nil, fmt.Errorf("não foi possível identificar caminhos válidos no patch")
	}
	return paths, nil
}

func parseDiffGitPathPair(raw string) (string, string, bool) {
	left, rest, ok := consumePatchToken(raw)
	if !ok {
		return "", "", false
	}
	right, _, ok := consumePatchToken(rest)
	if !ok {
		return "", "", false
	}
	return left, right, true
}

func parsePatchHeaderPath(raw string) (string, bool) {
	token, _, ok := consumePatchToken(raw)
	if !ok {
		return "", false
	}
	return token, true
}

func consumePatchToken(raw string) (string, string, bool) {
	trimmed := strings.TrimLeft(raw, " \t")
	if trimmed == "" {
		return "", "", false
	}

	if trimmed[0] != '"' {
		if idx := strings.IndexAny(trimmed, " \t"); idx >= 0 {
			return trimmed[:idx], trimmed[idx:], true
		}
		return trimmed, "", true
	}

	escaped := false
	for i := 1; i < len(trimmed); i++ {
		switch {
		case escaped:
			escaped = false
		case trimmed[i] == '\\':
			escaped = true
		case trimmed[i] == '"':
			return trimmed[:i+1], trimmed[i+1:], true
		}
	}
	return "", "", false
}

func decodePatchPathToken(raw string) (string, bool) {
	token := strings.TrimSpace(raw)
	if token == "" || token == "/dev/null" {
		return "", false
	}

	if strings.HasPrefix(token, "\"") {
		unquoted, err := strconv.Unquote(token)
		if err != nil {
			return "", false
		}
		token = unquoted
	}

	token = strings.TrimSpace(token)
	if token == "" || token == "/dev/null" {
		return "", false
	}
	if strings.HasPrefix(token, "a/") || strings.HasPrefix(token, "b/") {
		token = token[2:]
	}
	if token == "" {
		return "", false
	}

	return token, true
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
