package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"orch/internal/config"
)

// StackConfig define as ferramentas a serem instaladas na imagem base.
type StackConfig struct {
	ImageName string            `json:"imageName"`
	Tools     map[string]string `json:"tools"` // Mapa de ferramenta -> versão (ex: "node": "20")
}

// GenerateDockerfile gera o conteúdo do Dockerfile baseado na configuração.
// Estratégia de cache: apt update/install primeiro, depois ferramentas específicas.
func (s *Service) GenerateDockerfile(cfg StackConfig) string {
	var sb strings.Builder

	// Base image e configurações iniciais
	sb.WriteString(`FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive

# --- System Dependencies ---
RUN apt-get update && apt-get install -y \
    git \
    curl \
    wget \
    ca-certificates \
    unzip \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

`)

	// 2. Ferramentas APT adicionais (jq, ffmpeg)
	aptTools := []string{}
	for tool := range cfg.Tools {
		switch tool {
		case "jq":
			aptTools = append(aptTools, "jq")
		case "ffmpeg":
			aptTools = append(aptTools, "ffmpeg")
		case "python": // python3 e pip via apt é seguro no debian
			aptTools = append(aptTools, "python3", "python3-pip", "python3-venv")
		}
	}

	if len(aptTools) > 0 {
		sort.Strings(aptTools) // Garantir ordem determinística para cache do Docker
		toolsList := strings.Join(aptTools, " ")
		sb.WriteString(fmt.Sprintf(`# --- APT Tools ---
RUN apt-get update && apt-get install -y \
    %s \
    && rm -rf /var/lib/apt/lists/*

`, toolsList))
	}

	// 3. Linguagens e Ferramentas Complexas (Instalações Manuais)

	// Node.js
	if version, ok := cfg.Tools["node"]; ok {
		// Default to 20 if empty or invalid
		nodeVer := "20"
		if version != "" {
			nodeVer = version
		}
		
		// Extrair apenas o número major se vier "20 (LTS)" por exemplo
		if parts := strings.Fields(nodeVer); len(parts) > 0 {
			nodeVer = parts[0]
		}
		// Se for numérico, assume setup_X.x
		
		sb.WriteString(fmt.Sprintf(`# --- Node.js %s ---
RUN curl -fsSL https://deb.nodesource.com/setup_%s.x | bash - && \
    apt-get install -y nodejs && \
    npm install -g yarn pnpm && \
    rm -rf /var/lib/apt/lists/*

`, nodeVer, nodeVer))
	}

	// Go
	if version, ok := cfg.Tools["go"]; ok {
		// Default to 1.22 if empty
		goVer := "1.22.0"
		if version != "" {
			// Se o usuário passar apenas "1.22", adicionar o .0 patch se necessário ou assumir que o usuário sabe o que faz.
			// Vamos assumir que o frontend manda algo como "1.22" e nós transformamos em "1.22.0" se for curto, 
			// ou usamos direto se for completo.
			// Para simplificar, vamos garantir que seja um formato válido para URL.
			goVer = version
			if strings.Count(goVer, ".") == 1 {
				goVer += ".0"
			}
		}

		sb.WriteString(fmt.Sprintf(`# --- Go %s ---
RUN wget -q https://go.dev/dl/go%s.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go%s.linux-amd64.tar.gz && \
    rm go%s.linux-amd64.tar.gz
ENV PATH=$PATH:/usr/local/go/bin

`, version, goVer, goVer, goVer))
	}

	// Rust
	if _, ok := cfg.Tools["rust"]; ok {
		sb.WriteString(`# --- Rust ---
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
ENV PATH="/root/.cargo/bin:${PATH}"

`)
	}

	// AWS CLI v2
	if _, ok := cfg.Tools["aws"]; ok {
		sb.WriteString(`# --- AWS CLI ---
RUN curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip" && \
    unzip awscliv2.zip && \
    ./aws/install && \
    rm -rf aws awscliv2.zip

`)
	}

	// Docker CLI
	if _, ok := cfg.Tools["docker-cli"]; ok {
		sb.WriteString(`# --- Docker CLI ---
RUN curl -fsSL https://get.docker.com -o get-docker.sh && \
    sh get-docker.sh && \
    rm get-docker.sh

`)
	}

	// Configuração final
	sb.WriteString(`
WORKDIR /workspace
CMD ["/bin/bash"]
`)

	return sb.String()
}

// BuildStackImage gera o Dockerfile e executa o build, streamando logs.
func (s *Service) BuildStackImage(ctx context.Context, cfg StackConfig, logFn func(string)) error {
	// 1. Gerar conteúdo
	dockerfileContent := s.GenerateDockerfile(cfg)
	logFn(fmt.Sprintf("Gerando Dockerfile com ferramentas: %v", cfg.Tools))

	// 2. Criar diretório temporário para contexto do build
	tmpDir, err := os.MkdirTemp("", "orch-stack-build-*")
	if err != nil {
		return fmt.Errorf("erro ao criar temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir) // Cleanup

	// 3. Escrever Dockerfile
	dockerfilePath := filepath.Join(tmpDir, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644); err != nil {
		return fmt.Errorf("erro ao escrever Dockerfile: %w", err)
	}

	// 4. Executar docker build
	imageTag := "orch-custom-stack:latest"
	if cfg.ImageName != "" {
		imageTag = cfg.ImageName
	} else {
		cfg.ImageName = imageTag // Garante que a struct tenha o nome final para salvar
	}

	logFn(fmt.Sprintf("Iniciando build da imagem: %s", imageTag))
	logFn("Contexto: " + tmpDir)

	cmd := exec.CommandContext(ctx, "docker", "build", "-t", imageTag, ".")
	cmd.Dir = tmpDir

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("falha ao iniciar docker build: %w", err)
	}

	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			logFn(scanner.Text())
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			logFn(scanner.Text())
		}
	}()

	if err := cmd.Wait(); err != nil {
		logFn(fmt.Sprintf("❌ Erro no build: %v", err))
		return fmt.Errorf("docker build falhou: %w", err)
	}

	logFn("✅ Build concluído com sucesso!")
	logFn(fmt.Sprintf("Imagem criada: %s", imageTag))

	// 5. Salvar configuração para persistência
	if err := s.saveStackConfig(cfg); err != nil {
		logFn(fmt.Sprintf("⚠️ Aviso: Falha ao salvar configuração: %v", err))
	} else {
		logFn("💾 Configuração salva com sucesso.")
	}

	return nil
}

func (s *Service) saveStackConfig(cfg StackConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(config.DataDir(), "stack.json")
	return os.WriteFile(path, data, 0644)
}

// LoadStackConfig carrega a configuração salva, se existir.
func (s *Service) LoadStackConfig() (StackConfig, error) {
	path := filepath.Join(config.DataDir(), "stack.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return StackConfig{}, nil // Retorna vazio se não existir
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return StackConfig{}, err
	}
	var cfg StackConfig
	err = json.Unmarshal(data, &cfg)
	return cfg, err
}
