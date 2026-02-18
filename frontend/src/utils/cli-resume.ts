/**
 * CLI Resume Commands — mapeia CLIs de IA para seus comandos de retomada de sessão.
 *
 * Quando o ORCH é reaberto, terminais que tinham CLIs de IA ativas
 * são restaurados usando estes comandos para retomar a última sessão.
 */

const CLI_RESUME_COMMANDS: Record<string, string> = {
  gemini: 'gemini --resume',
  claude: 'claude --continue',
  codex: 'codex resume --last',
  opencode: 'opencode --continue',
}

/**
 * Retorna o comando de resume para uma CLI de IA.
 * @param cliType - Tipo da CLI (gemini, claude, codex, opencode)
 * @returns Comando de resume ou null se não for uma CLI conhecida
 */
export function getResumeCommand(cliType: string): string | null {
  return CLI_RESUME_COMMANDS[cliType] || null
}

/**
 * Verifica se um tipo de CLI é suportado para resume.
 */
export function isSupportedCLI(cliType: string): boolean {
  return cliType in CLI_RESUME_COMMANDS
}
