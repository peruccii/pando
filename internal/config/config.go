package config

import (
	"os"
	"path/filepath"
)

const (
	// AppName é o nome do aplicativo
	AppName = "ORCH"

	// AppVersion é a versão atual
	AppVersion = "1.0.0"

	// AppBundleID é o bundle identifier macOS
	AppBundleID = "com.orch.app"

	// DeepLinkScheme é o scheme para deep links (orch://)
	DeepLinkScheme = "orch"

	// DBFileName é o nome do arquivo SQLite
	DBFileName = "orch_data.db"

	// TerminalRingBufferSize é o tamanho do ring buffer do terminal (64KB)
	TerminalRingBufferSize = 64 * 1024

	// MaxAgents é o número máximo de agentes simultâneos
	MaxAgents = 20

	// DefaultPollInterval é o intervalo padrão de polling GitHub (30s)
	DefaultPollInterval = 30

	// TokenBudget é o orçamento de tokens para contexto de IA
	TokenBudget = 4000
)

// DataDir retorna o diretório raiz de dados do app
// ~/Library/Application Support/ORCH/
func DataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", "ORCH")
}

// DBPath retorna o caminho do arquivo SQLite
func DBPath() string {
	return filepath.Join(DataDir(), DBFileName)
}

// LogDir retorna o diretório de logs
func LogDir() string {
	return filepath.Join(DataDir(), "logs")
}

// CacheDir retorna o diretório de cache
func CacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Caches", "ORCH")
}

// EnsureDataDirs cria os diretórios necessários se não existirem
func EnsureDataDirs() error {
	dirs := []string{
		DataDir(),
		LogDir(),
		CacheDir(),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	return nil
}
