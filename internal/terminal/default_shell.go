package terminal

import (
	"bufio"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

var (
	defaultShellOnce sync.Once
	defaultShellPath string
)

// DefaultShell retorna o shell padrão do usuário atual com fallback por sistema.
func DefaultShell() string {
	defaultShellOnce.Do(func() {
		defaultShellPath = detectDefaultShell()
	})
	return defaultShellPath
}

// GetAvailableShells retorna uma lista de todos os shells disponíveis no sistema.
// No macOS/Linux, lê de /etc/shells.
func GetAvailableShells() []string {
	shells := make([]string, 0)
	uniqueShells := make(map[string]bool)

	// Adiciona o shell padrão primeiro
	def := DefaultShell()
	shells = append(shells, def)
	uniqueShells[def] = true

	// No macOS e Linux, tenta ler /etc/shells
	if runtime.GOOS != "windows" {
		file, err := os.Open("/etc/shells")
		if err == nil {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				if _, exists := uniqueShells[line]; !exists {
					// Verifica se o arquivo existe e é executável
					if info, err := os.Stat(line); err == nil && !info.IsDir() {
						shells = append(shells, line)
						uniqueShells[line] = true
					}
				}
			}
		}
	}

	// Candidatos comuns se não estiverem no /etc/shells
	candidates := []string{"/bin/bash", "/bin/zsh", "/bin/sh", "/usr/bin/zsh", "/usr/bin/bash", "/usr/local/bin/fish"}
	if runtime.GOOS == "windows" {
		candidates = []string{"cmd.exe", "powershell.exe", "pwsh.exe"}
	}

	for _, candidate := range candidates {
		if resolved, ok := normalizeShell(candidate); ok {
			if _, exists := uniqueShells[resolved]; !exists {
				shells = append(shells, resolved)
				uniqueShells[resolved] = true
			}
		}
	}

	return shells
}

func detectDefaultShell() string {
	if shell, ok := lookupLoginShell(); ok {
		return shell
	}

	if shell, ok := normalizeShell(os.Getenv("SHELL")); ok {
		return shell
	}

	if runtime.GOOS == "windows" {
		if shell, ok := normalizeShell("pwsh"); ok {
			return shell
		}
		if shell, ok := normalizeShell("powershell"); ok {
			return shell
		}
		return "cmd.exe"
	}

	for _, candidate := range []string{"/bin/zsh", "/bin/bash", "/bin/sh"} {
		if shell, ok := normalizeShell(candidate); ok {
			return shell
		}
	}

	if shell, ok := normalizeShell("sh"); ok {
		return shell
	}

	return "/bin/sh"
}

func lookupLoginShell() (string, bool) {
	currentUser, err := user.Current()
	if err != nil || currentUser == nil || strings.TrimSpace(currentUser.Username) == "" {
		return "", false
	}

	username := strings.TrimSpace(currentUser.Username)

	switch runtime.GOOS {
	case "darwin":
		return lookupShellFromDSCL(username)
	case "linux":
		if shell, ok := lookupShellFromGetent(username); ok {
			return shell, true
		}
		return lookupShellFromPasswd(username)
	default:
		return "", false
	}
}

func lookupShellFromDSCL(username string) (string, bool) {
	out, err := exec.Command("dscl", ".", "-read", "/Users/"+username, "UserShell").Output()
	if err != nil {
		return "", false
	}

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "UserShell:") {
			return normalizeShell(strings.TrimSpace(strings.TrimPrefix(line, "UserShell:")))
		}
	}

	return "", false
}

func lookupShellFromGetent(username string) (string, bool) {
	out, err := exec.Command("getent", "passwd", username).Output()
	if err != nil {
		return "", false
	}

	return parsePasswdEntry(strings.TrimSpace(string(out)))
}

func lookupShellFromPasswd(username string) (string, bool) {
	file, err := os.Open("/etc/passwd")
	if err != nil {
		return "", false
	}
	defer file.Close()

	prefix := username + ":"
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, prefix) {
			return parsePasswdEntry(line)
		}
	}

	return "", false
}

func parsePasswdEntry(line string) (string, bool) {
	fields := strings.Split(line, ":")
	if len(fields) < 7 {
		return "", false
	}
	return normalizeShell(fields[6])
}

func normalizeShell(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}

	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return "", false
	}

	shell := parts[0]
	if filepath.IsAbs(shell) {
		info, err := os.Stat(shell)
		if err != nil || info.IsDir() {
			return "", false
		}
		return shell, true
	}

	resolved, err := exec.LookPath(shell)
	if err != nil {
		return "", false
	}
	return resolved, true
}
