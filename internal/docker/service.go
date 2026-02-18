package docker

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/creack/pty"
	"github.com/google/uuid"
)

// Service implementa operações Docker para sessões sandbox.
type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) IsDockerAvailable() bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}

	cmd := exec.Command("docker", "info", "--format", "{{.ServerVersion}}")
	return cmd.Run() == nil
}

func (s *Service) ImageExists(imageName string) bool {
	cmd := exec.Command("docker", "image", "inspect", imageName)
	return cmd.Run() == nil
}

func (s *Service) CreateContainer(config ContainerConfig) (string, error) {
	cfg := withDefaults(config)
	image := cfg.Image

	if strings.HasPrefix(image, "build:") {
		projectPath := strings.TrimPrefix(image, "build:")
		tag := "orch-local-" + uuid.NewString()[:8]
		build := exec.Command("docker", "build", "-t", tag, projectPath)
		if out, err := build.CombinedOutput(); err != nil {
			return "", fmt.Errorf("docker build failed: %w: %s", err, strings.TrimSpace(string(out)))
		}
		image = tag
	}

	args := s.buildRunArgs(cfg, image)
	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker create failed: %w: %s", err, strings.TrimSpace(string(out)))
	}

	containerID := strings.TrimSpace(string(out))
	if containerID == "" {
		return "", fmt.Errorf("docker create returned empty container id")
	}

	return containerID, nil
}

func (s *Service) StartContainer(containerID string) error {
	return runDocker("start", containerID)
}

func (s *Service) StopContainer(containerID string) error {
	return runDocker("stop", "-t", "3", containerID)
}

func (s *Service) RemoveContainer(containerID string) error {
	return runDocker("rm", "-f", containerID)
}

func (s *Service) RestartContainer(containerID string) error {
	return runDocker("restart", containerID)
}

func (s *Service) ExecInContainer(containerID string, cmdArgs []string) (io.ReadWriteCloser, error) {
	if len(cmdArgs) == 0 {
		cmdArgs = []string{"/bin/sh"}
	}

	args := append([]string{"exec", "-it", containerID}, cmdArgs...)
	cmd := exec.Command("docker", args...)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("docker exec failed: %w", err)
	}
	return ptmx, nil
}

func (s *Service) GetContainerStatus(containerID string) (string, error) {
	cmd := exec.Command("docker", "inspect", "--format", "{{.State.Status}}", containerID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker inspect failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func (s *Service) ListContainers() ([]ContainerInfo, error) {
	cmd := exec.Command("docker", "ps", "-a", "--filter", "name=orch-session-", "--format", "{{.ID}}|{{.Names}}|{{.Image}}|{{.State}}|{{.Status}}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker ps failed: %w: %s", err, strings.TrimSpace(string(out)))
	}

	containers := make([]ContainerInfo, 0)
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 5)
		if len(parts) != 5 {
			continue
		}
		containers = append(containers, ContainerInfo{
			ID:     parts[0],
			Name:   parts[1],
			Image:  parts[2],
			State:  parts[3],
			Status: parts[4],
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return containers, nil
}

func withDefaults(cfg ContainerConfig) ContainerConfig {
	if cfg.Memory == "" {
		cfg.Memory = "2g"
	}
	if cfg.CPUs == "" {
		cfg.CPUs = "2"
	}
	if cfg.Shell == "" {
		cfg.Shell = "/bin/sh"
	}
	if cfg.NetworkMode == "" {
		cfg.NetworkMode = "none"
	}
	if cfg.Image == "" {
		cfg.Image = "alpine:latest"
	}
	if !cfg.ReadOnly {
		cfg.ReadOnly = true
	}
	if cfg.ProjectPath != "" {
		cfg.ProjectPath = filepath.Clean(cfg.ProjectPath)
	}
	return cfg
}

func runDocker(args ...string) error {
	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func shortID() string {
	return strings.ReplaceAll(uuid.NewString()[:8], "-", "")
}

// buildRunArgs monta os argumentos de criação com isolamento de segurança.
func (s *Service) buildRunArgs(config ContainerConfig, image string) []string {
	name := "orch-session-" + shortID()

	args := []string{
		"create",
		"-i",
		"--name", name,
		"--memory", config.Memory,
		"--cpus", config.CPUs,
		"--pids-limit", "256",
		"--security-opt", "no-new-privileges",
	}

	if config.ProjectPath != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/workspace", config.ProjectPath))
		args = append(args, "-w", "/workspace")
	}

	for _, env := range config.EnvVars {
		if strings.TrimSpace(env) == "" {
			continue
		}
		args = append(args, "-e", env)
	}

	for _, port := range config.Ports {
		if strings.TrimSpace(port) == "" {
			continue
		}
		args = append(args, "-p", port)
	}

	if config.ReadOnly {
		args = append(args,
			"--read-only",
			"--tmpfs", "/tmp:rw,noexec,nosuid,size=512m",
			"--tmpfs", "/var/tmp:rw,noexec,nosuid,size=256m",
		)
	}

	if config.NetworkMode != "" {
		args = append(args, "--network", config.NetworkMode)
	}

	args = append(args, image, config.Shell)
	return args
}

// WaitUntilRunning aguarda o container ficar running por até timeout.
func (s *Service) WaitUntilRunning(containerID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, err := s.GetContainerStatus(containerID)
		if err == nil && status == "running" {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting container %s to start", containerID)
}
