package docker

import "io"

// IDockerService define a interface para gerenciamento de containers Docker.
type IDockerService interface {
	CreateContainer(config ContainerConfig) (containerID string, err error)
	StartContainer(containerID string) error
	StopContainer(containerID string) error
	RemoveContainer(containerID string) error
	RestartContainer(containerID string) error

	IsDockerAvailable() bool
	GetContainerStatus(containerID string) (string, error)
	ListContainers() ([]ContainerInfo, error)

	ExecInContainer(containerID string, cmd []string) (io.ReadWriteCloser, error)
	DetectImage(projectPath string) string
}

// ContainerConfig define configuração de execução segura para sessões.
type ContainerConfig struct {
	Image       string   `json:"image"`
	ProjectPath string   `json:"projectPath"`
	Memory      string   `json:"memory"`
	CPUs        string   `json:"cpus"`
	Shell       string   `json:"shell"`
	Ports       []string `json:"ports,omitempty"`
	EnvVars     []string `json:"envVars,omitempty"`
	ReadOnly    bool     `json:"readOnly"`
	NetworkMode string   `json:"networkMode"`
}

// ContainerInfo representa dados resumidos de um container.
type ContainerInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Image  string `json:"image"`
	State  string `json:"state"`
	Status string `json:"status"`
}
