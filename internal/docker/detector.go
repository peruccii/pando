package docker

import (
	"os"
	"path/filepath"
)

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// DetectImage detecta automaticamente a imagem Docker pelo tipo de projeto.
// Se existir Dockerfile, retorna prefixo build:<path> para build local.
func (s *Service) DetectImage(projectPath string) string {
	if projectPath == "" {
		return "alpine:latest"
	}

	if fileExists(filepath.Join(projectPath, "Dockerfile")) {
		return "build:" + projectPath
	}

	detectors := map[string]string{
		"package.json":     "node:20-alpine",
		"go.mod":           "golang:1.22-alpine",
		"requirements.txt": "python:3.12-slim",
		"Cargo.toml":       "rust:1.75-slim",
		"Gemfile":          "ruby:3.3-slim",
	}

	for marker, image := range detectors {
		if fileExists(filepath.Join(projectPath, marker)) {
			return image
		}
	}

	return "alpine:latest"
}
