package gitactivity

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// CollectStagedFiles retorna resumo de arquivos staged com contagem de linhas.
func CollectStagedFiles(repoPath string) ([]EventFile, error) {
	repoPath = strings.TrimSpace(repoPath)
	if repoPath == "" {
		return nil, fmt.Errorf("repo path is required")
	}

	numstatOut, err := runGit(repoPath, "diff", "--cached", "--numstat")
	if err != nil {
		return nil, err
	}
	nameStatusOut, err := runGit(repoPath, "diff", "--cached", "--name-status")
	if err != nil {
		return nil, err
	}

	filesByPath := map[string]*EventFile{}

	for _, line := range strings.Split(strings.TrimSpace(numstatOut), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		path := strings.TrimSpace(parts[2])
		if path == "" {
			continue
		}

		added := parseNumstatValue(parts[0])
		removed := parseNumstatValue(parts[1])
		filesByPath[path] = &EventFile{
			Path:    path,
			Added:   added,
			Removed: removed,
		}
	}

	order := make([]string, 0)
	for _, line := range strings.Split(strings.TrimSpace(nameStatusOut), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		status := strings.TrimSpace(parts[0])
		path := strings.TrimSpace(parts[len(parts)-1])
		if path == "" {
			continue
		}
		file, exists := filesByPath[path]
		if !exists {
			file = &EventFile{Path: path}
			filesByPath[path] = file
		}
		file.Status = status
		order = append(order, path)
	}

	if len(order) == 0 {
		for path := range filesByPath {
			order = append(order, path)
		}
	}

	result := make([]EventFile, 0, len(order))
	seen := map[string]struct{}{}
	for _, path := range order {
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, *filesByPath[path])
	}

	return result, nil
}

// GetStagedDiff retorna diff staged de um arquivo (ou geral se filePath vazio).
func GetStagedDiff(repoPath, filePath string) (string, error) {
	repoPath = strings.TrimSpace(repoPath)
	if repoPath == "" {
		return "", fmt.Errorf("repo path is required")
	}

	args := []string{"diff", "--cached", "--unified=3"}
	if strings.TrimSpace(filePath) != "" {
		args = append(args, "--", filePath)
	}

	out, err := runGit(repoPath, args...)
	if err != nil {
		return "", err
	}

	const maxSize = 30000
	if len(out) > maxSize {
		return out[:maxSize] + "\n\n... (diff truncado)", nil
	}
	return out, nil
}

// UnstageFile remove arquivo do stage.
func UnstageFile(repoPath, filePath string) error {
	repoPath = strings.TrimSpace(repoPath)
	filePath = strings.TrimSpace(filePath)
	if repoPath == "" || filePath == "" {
		return fmt.Errorf("repo path and file path are required")
	}

	_, err := runGit(repoPath, "restore", "--staged", "--", filePath)
	return err
}

// DiscardFile descarta mudanças locais do arquivo.
func DiscardFile(repoPath, filePath string) error {
	repoPath = strings.TrimSpace(repoPath)
	filePath = strings.TrimSpace(filePath)
	if repoPath == "" || filePath == "" {
		return fmt.Errorf("repo path and file path are required")
	}

	_, err := runGit(repoPath, "checkout", "--", filePath)
	return err
}

func runGit(repoPath string, args ...string) (string, error) {
	cleanRepo := filepath.Clean(repoPath)
	cmd := exec.Command("git", args...)
	cmd.Dir = cleanRepo
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func parseNumstatValue(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "-" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return value
}
