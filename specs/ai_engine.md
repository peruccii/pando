# Spec: Motor de IA com Injeção de Contexto (Context-Aware AI)

> **Módulo**: 3 — AI Engine  
> **Status**: Draft  
> **PRD Ref**: Seção 9  
> **Última Atualização**: 12 de Fevereiro de 2
---

## 1. Objetivo

Implementar um serviço Go (Wails) que atue como **"Proxy Inteligente"** entre o terminal (xterm.js) e APIs de LLM (Gemini/OpenAI). Resolver o problema de "cegueira da CLI" onde a IA padrão não tem acesso ao estado da aplicação.

---

## 2. Problema

IAs em CLI são processos isolados. Não sabem qual PR, Issue ou arquivo o usuário está visualizando. Resultado: respostas genéricas e descontextualizadas.

**Solução**: Padrão **Prompt Augmentation** — injeção dinâmica de contexto antes da requisição à IA. Não executamos binários externos; usamos SDKs nativos de Go.

---

## 3. Arquitetura "Man-in-the-Middle"

```
Frontend (xterm.js)
    │ "Explique este PR"
    ▼
Interceptador (Wails/Go)
    │ Detecta comando de IA
    │ NÃO envia para o shell
    ▼
Context Builder
    │ Verifica SessionState:
    │  - PR aberto na lateral? → Recupera Diff + Descrição do cache
    │  - Erro no terminal? → Recupera LastStderr
    │  - Arquivo aberto? → Recupera path + conteúdo parcial
    ▼
Prompt Assembly
    │ Concatena SystemPrompt + Contexto + UserMessage
    ▼
LLM API (Gemini/OpenAI)
    │
    ▼
Streaming Response → Wails Event → xterm.js (simula digitação)
```

---

## 4. AIService (Backend Go)

### 4.1 Interface

```go
type IAIService interface {
    // Gera resposta com contexto injetado
    GenerateResponse(ctx context.Context, userMessage string, sessionID string) (<-chan string, error)

    // Configura provedor
    SetProvider(provider AIProvider) error

    // Lista provedores disponíveis
    ListProviders() []AIProvider

    // Cancela geração em andamento
    Cancel(sessionID string) error
}

type AIProvider struct {
    ID       string // "gemini", "openai", "ollama"
    Name     string // "Gemini Pro", "GPT-4", "Llama 3"
    Model    string // "gemini-pro", "gpt-4-turbo"
    APIKey   string // Em memória, nunca persistido inseguro
    Endpoint string // URL base (para Ollama local)
}
```

### 4.2 Struct Principal

```go
type AIService struct {
    providers    map[string]AIProvider
    githubSvc    IGitHubService    // Acesso a dados cacheados
    sessionStore ISessionStore     // Estado da sessão atual
    termHistory  ITerminalHistory  // Histórico do terminal
    tokenBudget  int               // Max tokens para contexto (default: 4000)
    sanitizer    *SecretSanitizer  // Remove segredos
}
```

### 4.3 Método Principal

```go
func (s *AIService) GenerateResponse(
    ctx context.Context,
    userMessage string,
    sessionID string,
) (<-chan string, error) {
    // 1. Montar contexto
    context := s.buildContext(sessionID)

    // 2. Montar prompt completo
    prompt := s.assemblePrompt(context, userMessage)

    // 3. Sanitizar (remover tokens/senhas)
    prompt = s.sanitizer.Clean(prompt)

    // 4. Verificar token budget
    prompt = s.truncateToFit(prompt, s.tokenBudget)

    // 5. Enviar para LLM (streaming)
    stream := make(chan string, 100)
    go func() {
        defer close(stream)
        err := s.streamFromProvider(ctx, prompt, stream)
        if err != nil {
            stream <- fmt.Sprintf("\n[Erro: %s]", err.Error())
        }
    }()

    return stream, nil
}
```

---

## 5. Context Builder

### 5.1 SessionState

```go
type SessionState struct {
    ProjectName   string
    CurrentBranch string
    CurrentFile   *string   // Arquivo aberto no visualizador

    // GitHub Context
    ActivePR      *PRContext
    ActiveIssue   *IssueContext

    // Terminal Context
    LastCommand   string
    LastStdout    string    // Últimas 50 linhas
    LastStderr    string    // Saída de erro (se houver)
    ShellType     string    // "zsh", "bash"
}

type PRContext struct {
    Number  int
    Title   string
    Body    string
    Diff    string   // Truncado via TokenBudget
    Author  string
    Branch  string
}
```

### 5.2 Montagem do Contexto

```go
func (s *AIService) buildContext(sessionID string) SessionState {
    state := s.sessionStore.Get(sessionID)

    // Enriquecer com dados do GitHub (do cache, sem bater na API)
    if state.ActivePR != nil {
        pr, _ := s.githubSvc.GetCachedPR(state.ActivePR.Number)
        if pr != nil {
            state.ActivePR.Diff = s.truncateDiff(pr.Diff, 2000)
            state.ActivePR.Body = pr.Body
        }
    }

    // Enriquecer com histórico do terminal
    history := s.termHistory.GetLast(sessionID, 50)
    state.LastCommand = history.LastCommand
    state.LastStderr = history.LastStderr

    return state
}
```

---

## 6. System Prompt Template

```
--- SYSTEM CONTEXT (INJECTED) ---
[ROLE]
Você é um Arquiteto de Software Sênior assistindo um desenvolvedor dentro de um terminal.
Seja conciso, técnico e direto. Evite markdown complexo que quebre em terminais TTY.

[CURRENT APP STATE]
- Projeto: {{ProjectName}}
- Branch Atual: {{CurrentBranch}}
- Arquivo Aberto (Visualizador): {{CurrentFile}}

[GITHUB CONTEXT — ACTIVE PR]
{{#if ActivePR}}
- PR ID: #{{ActivePR.Number}}
- Título: {{ActivePR.Title}}
- Diff (Resumo):
  {{ActivePR.Diff}}
{{else}}
(Nenhum PR ativo na lateral)
{{/if}}

[TERMINAL HISTORY]
- Último comando: {{LastCommand}}
- Saída de erro: {{LastStderr}}
---------------------------------

[USER INPUT]
{{UserMessage}}
```

---

## 7. Token Budget & Truncamento

### 7.1 Orçamento por Seção

| Seção              | Max Tokens | Prioridade |
| ------------------- | ---------- | ---------- |
| System Role         | 200        | 1 (fixo)   |
| App State           | 100        | 2          |
| PR Diff             | 2000       | 3          |
| Terminal History    | 500        | 4          |
| User Message        | 1000       | 1 (fixo)   |
| **Total**           | **~4000**  |            |

### 7.2 Truncamento Inteligente de Diffs

```go
func (s *AIService) truncateDiff(diff string, maxTokens int) string {
    files := parseDiffFiles(diff)

    // 1. Priorizar por extensão
    priority := map[string]int{
        ".go": 1, ".ts": 1, ".tsx": 1, ".js": 1, ".jsx": 1,
        ".py": 2, ".rs": 2, ".java": 2,
        ".css": 3, ".html": 3,
        ".json": 4, ".yaml": 4, ".yml": 4,
    }

    // 2. Ignorar sempre
    ignore := []string{
        "package-lock.json", "yarn.lock", "go.sum",
        "*.min.js", "*.min.css", "*.map",
    }

    // 3. Ordenar por prioridade
    sort.Slice(files, func(i, j int) bool {
        return priority[ext(files[i])] < priority[ext(files[j])]
    })

    // 4. Acumular até atingir maxTokens
    var result strings.Builder
    tokens := 0
    for _, f := range files {
        if shouldIgnore(f, ignore) { continue }
        fileTokens := estimateTokens(f.Content)
        if tokens + fileTokens > maxTokens { break }
        result.WriteString(f.Content)
        tokens += fileTokens
    }

    return result.String()
}
```

---

## 8. Sanitização de Segredos

```go
type SecretSanitizer struct {
    patterns []*regexp.Regexp
}

func NewSecretSanitizer() *SecretSanitizer {
    return &SecretSanitizer{
        patterns: []*regexp.Regexp{
            regexp.MustCompile(`(?i)(api[_-]?key|token|secret|password|auth)\s*[:=]\s*['"]?[\w\-\.]+['"]?`),
            regexp.MustCompile(`ghp_[a-zA-Z0-9]{36}`),           // GitHub PAT
            regexp.MustCompile(`gho_[a-zA-Z0-9]{36}`),           // GitHub OAuth
            regexp.MustCompile(`sk-[a-zA-Z0-9]{48}`),            // OpenAI
            regexp.MustCompile(`AIza[a-zA-Z0-9_\-]{35}`),        // Google API
            regexp.MustCompile(`Bearer\s+[\w\-\.]+`),            // Bearer tokens
        },
    }
}

func (s *SecretSanitizer) Clean(text string) string {
    for _, p := range s.patterns {
        text = p.ReplaceAllString(text, "[REDACTED]")
    }
    return text
}
```

---

## 9. Streaming de Resposta

```go
// Backend: Emite chunks via Wails Events
func (s *AIService) streamToFrontend(ctx context.Context, sessionID string, stream <-chan string) {
    for chunk := range stream {
        runtime.EventsEmit(ctx, "ai:response:chunk", map[string]string{
            "sessionID": sessionID,
            "chunk":     chunk,
        })
    }
    runtime.EventsEmit(ctx, "ai:response:done", sessionID)
}
```

```typescript
// Frontend: Escreve no xterm.js simulando digitação
wails.Events.On('ai:response:chunk', (event) => {
    const { sessionID, chunk } = event.data
    const term = getTerminal(sessionID)
    term.write(chunk)
})

wails.Events.On('ai:response:done', (sessionID) => {
    const term = getTerminal(sessionID)
    term.write('\r\n')
})
```

---

## 10. Interceptador de Comandos

```go
// Detecta se o input é um comando de IA ou comando de shell
func (s *AIService) IsAICommand(input string) bool {
    prefixes := []string{
        "/ai ", "/ask ", "/explain ", "/fix ",
        "@ai ", "@orch ",
    }
    trimmed := strings.TrimSpace(strings.ToLower(input))
    for _, p := range prefixes {
        if strings.HasPrefix(trimmed, p) { return true }
    }
    return false
}

func (s *AIService) ExtractMessage(input string) string {
    // Remove o prefixo e retorna a mensagem do usuário
    for _, p := range prefixes {
        if strings.HasPrefix(input, p) {
            return strings.TrimPrefix(input, p)
        }
    }
    return input
}
```

---

## 11. Casos de Uso (Test Cases)

| Cenário              | Input                    | Contexto Injetado          | Resposta Esperada                        |
| --------------------- | ------------------------ | -------------------------- | ---------------------------------------- |
| Explique o PR         | `/ai O que esse PR faz?` | PR.Body + PR.Diff          | Resumo das mudanças baseado no diff       |
| Correção de Erro      | `/fix Como arrumo isso?` | LastStderr                 | Análise do stack trace + sugestão         |
| Sem Contexto          | `/ai Gere soma em Go`   | PR e Logs vazios           | Resposta genérica de chatbot              |
| Arquivo Aberto        | `/ai Explique este file` | CurrentFile + conteúdo     | Explicação do arquivo aberto              |
| Multi-contexto        | `/ai O que mudou?`      | PR.Diff + CurrentBranch    | Resumo das mudanças na branch/PR          |

---

## 12. Providers Suportados

| Provider   | SDK Go                          | Streaming | Modelo Padrão    |
| ---------- | ------------------------------- | --------- | ---------------- |
| **Gemini** | `google.golang.org/genai`       | ✅        | gemini-pro        |
| **OpenAI** | `github.com/sashabaranov/go-openai` | ✅   | gpt-4-turbo       |
| **Ollama** | HTTP API (`localhost:11434`)     | ✅        | llama3            |

---

## 13. Dependências

| Dependência               | Tipo       | Spec Relacionada     |
| -------------------------- | ---------- | -------------------- |
| GitHubService (cache)      | Bloqueador | github_integration   |
| Terminal History            | Bloqueador | terminal_sharing     |
| Session Store               | Bloqueador | auth_and_persistence |
| Wails Events (streaming)   | Bloqueador | —                    |
