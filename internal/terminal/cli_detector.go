package terminal

import (
	"log"
	"os/exec"
	"strconv"
	"strings"
)

// CLIType representa uma CLI de IA conhecida.
type CLIType string

const (
	CLINone     CLIType = ""
	CLIGemini   CLIType = "gemini"
	CLIClaude   CLIType = "claude"
	CLICodex    CLIType = "codex"
	CLIOpenCode CLIType = "opencode"
)

// CLIResumeCommands mapeia cada CLI para seu comando de resume/continue.
var CLIResumeCommands = map[CLIType]string{
	CLIGemini:   "gemini --resume",
	CLIClaude:   "claude --continue",
	CLICodex:    "codex resume --last",
	CLIOpenCode: "opencode --continue",
}

// knownCLIBinaries mapeia nomes de processos para CLIType.
// Inclui variações comuns (binários npm podem ter nomes diferentes).
var knownCLIBinaries = map[string]CLIType{
	"gemini":   CLIGemini,
	"claude":   CLIClaude,
	"codex":    CLICodex,
	"opencode": CLIOpenCode,
}

// DetectCLI verifica qual CLI de IA está rodando como processo filho do PID dado.
// Busca recursivamente nos filhos. Retorna CLINone se nenhuma CLI conhecida for encontrada.
func DetectCLI(pid int) CLIType {
	if pid <= 0 {
		return CLINone
	}

	// Obter PIDs filhos via pgrep (macOS/Linux)
	childPIDs := getChildPIDs(pid)
	if len(childPIDs) == 0 {
		return CLINone
	}

	for _, childPID := range childPIDs {
		// Obter nome do processo
		procName := getProcessName(childPID)
		if procName == "" {
			continue
		}

		// Normalizar: pegar apenas o basename (sem path)
		parts := strings.Split(procName, "/")
		baseName := strings.ToLower(parts[len(parts)-1])

		if cliType, ok := knownCLIBinaries[baseName]; ok {
			log.Printf("[CLI-Detector] Found %s (PID %d) as child of PID %d", cliType, childPID, pid)
			return cliType
		}

		// Busca recursiva nos filhos do filho
		if found := DetectCLI(childPID); found != CLINone {
			return found
		}
	}

	return CLINone
}

// GetResumeCommand retorna o comando de resume para uma CLIType.
// Retorna string vazia se não for uma CLI conhecida.
func GetResumeCommand(cliType CLIType) string {
	return CLIResumeCommands[cliType]
}

// getChildPIDs retorna os PIDs filhos diretos de um processo via pgrep.
func getChildPIDs(parentPID int) []int {
	out, err := exec.Command("pgrep", "-P", strconv.Itoa(parentPID)).Output()
	if err != nil {
		return nil
	}

	var pids []int
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if pid, err := strconv.Atoi(line); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids
}

// getProcessName retorna o nome (comm) de um processo via ps (compatível macOS).
func getProcessName(pid int) string {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// GetProcessCwd retorna o diretório de trabalho atual de um processo (macOS via lsof).
// Fallback: retorna string vazia em caso de erro.
func GetProcessCwd(pid int) string {
	if pid <= 0 {
		return ""
	}

	// lsof combina filtros em OR por padrão; precisamos de -a para AND
	// (pid E fd=cwd), senão podemos ler cwd de outro processo e retornar "/" incorretamente.
	out, err := exec.Command("lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn").Output()
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "n") {
			cwd := strings.TrimPrefix(line, "n")
			return strings.TrimSpace(cwd)
		}
	}
	return ""
}
