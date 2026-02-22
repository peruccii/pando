# PRD - Git Panel (Source Control) para ORCH

## 1. Contexto

O botao "GitHub" da tela inicial do ORCH hoje abre um pane placeholder.
No v1 desta feature, o clique deve abrir uma tela dedicada do Git Panel (fora do mosaico de terminais).
O objetivo deste PRD e definir a implementacao de um painel de controle de Git local, inspirado em Git Extensions, com foco em:

- velocidade de staging
- clareza de diff
- operacao por teclado e mouse
- comportamento previsivel em repositorios grandes

Importante: apesar do nome visual atual ser "GitHub", o dominio desta funcionalidade e Git local (working tree, index, history, merge conflicts).

## 2. Objetivo do Produto

Entregar um painel de Source Control no ORCH que permita ao desenvolvedor:

- inspecionar historico em lista linear de alta performance
- montar commits atomicos com stage parcial (hunk/linhas)
- revisar alteracoes side-by-side com scroll sincronizado
- resolver conflitos de merge com acoes rapidas

Sem depender de polling continuo e sem comprometer responsividade do app.

## 3. Metas e Nao-Metas

### 3.1 Metas

- Carregamento inicial perceptivelmente rapido para historico local.
- Fluxo de stage/unstage/discard confiavel.
- UX de diff legivel e navegavel por teclado e mouse.
- Integracao com file watcher existente para refresh orientado a eventos.
- Execucao segura de comandos Git concorrentes via fila sequencial.

### 3.2 Nao-Metas (v1)

- Graph de commits visual complexo.
- Rebase interativo, reflog explorer, bisect wizard.
- Paridade total com clientes Git desktop maduros.
- Resolucao semiautomatica de conflitos em tres vias.

## 4. Publico-Alvo

- Desenvolvedor individual usando ORCH como command center.
- Tech Lead que precisa revisar mudancas locais rapidamente antes de commit/push.
- Grupo de desenvolvedores que precisam analisar o que cada integrante fez.

## 5. Escopo Funcional (v1)

### 5.1 Historico Linear (Performance First)

Descricao:

- lista unica de commits com infinite scroll
- sem renderizacao de grafo
- layout inspirado em "View commit log" do Git Extensions: branches/refs na esquerda e log no centro

Requisitos:

- Backend via `git log` com streaming e paginacao por cursor.
- Parsing robusto com separadores seguros (`%x1f` e `%x1e`) para evitar quebra por `|` em mensagem.
- Frontend com virtualizacao (ex.: react-window) para renderizar apenas viewport.
- UI moderna e profissional, sem copiar visual legado e sem adicionar graph de commits.

Criterios de aceite:

- primeira pagina visivel em ate 200ms para repositorio medio local
- scroll sem stutter perceptivel em listas longas

### 5.2 Stage Parcial (Hunk e Linhas)

Descricao:

- selecionar hunks e subconjuntos de linhas para stage

Requisitos:

- leitura base por `git diff -U3`
- geracao de patch parcial no frontend/backend
- aplicacao via `git apply --cached --unidiff-zero` (ou estrategia equivalente validada)
- suporte a unstage por arquivo e por trecho staged
- selecao multipla com `Shift+Click`

Criterios de aceite:

- usuario consegue criar commit atomico sem ir ao terminal
- patch parcial invalido mostra erro claro e opcao de retry/reset selecao

### 5.3 Diff Side-by-Side

Descricao:

- visualizacao "Original" e "Modified" lado a lado

Requisitos:

- parser de diff no backend (Git raw parsing; biblioteca opcional)
- scroll lock-step configuravel
- highlight de sintaxe para linguagens principais (JS/TS, Go, Rust, Python)
- lazy render para arquivos grandes

Criterios de aceite:

- navegacao por arquivo e por hunk sem travamento
- sincronizacao de scroll consistente entre os dois paines

### 5.4 Merge Conflict Management

Descricao:

- detectar e resolver conflitos comuns direto no painel

Requisitos:

- detectar merge ativo por `.git/MERGE_HEAD`
- listar arquivos em conflito via status porcelain (`UU`, etc.)
- acoes rapidas:
  - Accept Mine (`git checkout --ours -- <file>` + opcao de stage)
  - Accept Theirs (`git checkout --theirs -- <file>` + opcao de stage)
  - Open External Tool (configuravel)

Criterios de aceite:

- conflitos aparecem em ate 300ms apos evento do watcher
- acoes atualizam estado da UI sem reiniciar app

## 6. Requisitos Nao Funcionais

### 6.1 Performance

- startup da tela dedicada do Git Panel: < 300ms para primeira pintura
- primeira pagina de historico: < 200ms (repositorio medio)
- tempo de resposta para stage/unstage: < 150ms em arquivos pequenos/medios
- fallback para arquivos grandes:
  - sem preview automatico para arquivo > 1MB (configuravel)
  - sem syntax highlight para arquivo muito grande

### 6.2 Confiabilidade

- operacoes de escrita Git devem ser serializadas (fila unica por repositorio)
- nenhuma operacao de escrita executa em paralelo no mesmo repo
- erros de `index.lock` devem ser tratados com retry curto e mensagem clara

### 6.3 Observabilidade

- proibido polling de `git status` em loop
- refresh orientado por eventos do file watcher + debounce
- cada comando Git executado gera evento de diagnostico (duracao, exit code, stderr sanitizado)
- console de saida opcional na UI para transparencia tecnica

### 6.4 Seguranca

- sanitizacao de paths para evitar traversal (`..`, paths absolutos fora repo)
- comandos sempre com `--` antes de path quando aplicavel
- sem execucao shell concatenada; usar argumentos estruturados

## 7. Arquitetura Proposta

### 7.1 Frontend

- Reutilizar o botao/entrada visual `GitHub` para abrir uma tela dedicada do novo `GitPanel` (fora do Command Center).
- Modulos principais:
  - `BranchesSidebar` (contexto de branch atual, ahead/behind, refs/filtros quando disponivel)
  - `CommitLogList` (virtualizado + infinite scroll)
  - `InspectorPanel` (abas para WorkingTree, Commit, Diff, Conflicts)
  - `GitCommandConsole` (opcional)

### 7.2 Backend (Go / Wails)

- Novo service de dominio Git (ex.: `internal/gitpanel`) ou evolucao do `internal/gitactivity`.
- Responsabilidades:
  - leitura de status/historico/diff
  - aplicacao de stage parcial
  - queue sequencial de comandos write por repo
  - normalizacao de erros

### 7.3 Integracoes existentes

- `internal/filewatcher`: fonte primaria de invalidacao de cache/refresh.
- `internal/gitactivity`: pode ser mantido para timeline e audit trail.
- bindings Wails: expandir superficie de API para historico paginado e hunk staging.

## 8. Contrato de API (proposta inicial)

Leitura:

- `GitPanelGetStatus(repoPath) -> { staged[], unstaged[], conflicted[], branch, aheadBehind }`
- `GitPanelGetHistory(repoPath, cursor, limit) -> { items[], nextCursor }`
- `GitPanelGetDiff(repoPath, filePath, mode, contextLines) -> DiffModel`
- `GitPanelGetConflicts(repoPath) -> ConflictFile[]`

Escrita:

- `GitPanelStageFile(repoPath, filePath)`
- `GitPanelUnstageFile(repoPath, filePath)`
- `GitPanelStagePatch(repoPath, patchText)`
- `GitPanelUnstagePatch(repoPath, patchText)`
- `GitPanelDiscardFile(repoPath, filePath)`
- `GitPanelAcceptOurs(repoPath, filePath, autoStage)`
- `GitPanelAcceptTheirs(repoPath, filePath, autoStage)`

Eventos:

- `gitpanel:status_changed`
- `gitpanel:history_invalidated`
- `gitpanel:conflicts_changed`
- `gitpanel:command_result`

## 9. UX e Atalhos (macOS first)

Principios:

- minimalista
- baixo ruido visual
- alta densidade de informacao util
- referencia de layout: Git Extensions (View commit log), com acabamento moderno e profissional

Layout alvo (sem grafos):

- esquerda: branches/refs (com branch atual obrigatoria e filtros de contexto)
- centro: commit log linear com foco em leitura rapida
- direita: painel de detalhes (commit metadata, diff, estado staged/unstaged/conflicted)
- responsivo: em largura reduzida, painel da direita vira area inferior colapsavel

Atalhos alvo:

- `Cmd+Enter` commit
- `Cmd+S` stage selecao atual
- `Cmd+Shift+S` unstage selecao atual
- `Cmd+D` toggle diff
- `J/K` navegacao em lista
- `Shift+Click` range select

Feedback obrigatorio:

- status de acao (`Running`, `Success`, `Error`)
- comando Git executado (modo console opcional)
- erros com mensagem tecnica compacta + acao recomendada

## 10. Roadmap de Entrega

### Fase 1 - Foundation (P0)

- substituir placeholder atual por tela dedicada base do Git Panel acionada pelo botao GitHub
- implementar layout base inspirado no Git Extensions (sem grafos): branches/refs na esquerda, commit log no centro e inspector dedicado
- status staged/unstaged/conflicted
- historico linear paginado (sem grafos)
- virtualizacao da lista

### Fase 2 - Diff e staging avancado (P0)

- diff side-by-side com scroll sync
- stage/unstage por hunk
- stage multiplo por selecao de linhas

### Fase 3 - Merge e robustez (P1)

- painel de conflitos
- acoes Accept Mine/Theirs
- command queue sequencial por repo
- console de saida opcional

### Fase 4 - Polish (P1)

- afinacao de performance para repositorios grandes
- acessibilidade por teclado
- cobertura de testes de regressao

## 11. Criterios de Aceite (Definition of Done)

- O botao "GitHub" abre tela dedicada do Git Panel funcional (nao placeholder).
- Abrir/fechar a tela Git nao reinicia nem perde estado dos terminais ativos.
- O layout final remete ao fluxo de "View commit log" (branches/refs na esquerda e historico no centro), com visual moderno/profissional.
- Usuario consegue:
  - ver historico paginado
  - ver diff de arquivo
  - stage/unstage arquivo
  - stage parcial de hunk/linhas
  - resolver conflito basico com Mine/Theirs
- Nao existe polling continuo de `git status`.
- Operacoes write passam por fila sequencial e nao colidem em `index.lock`.
- Falhas exibem erro claro sem congelar UI.
- Testes criticos de parser de diff e patch parcial passam.

## 12. Riscos e Mitigacoes

Risco: patch parcial invalido em cenarios edge-case.

- Mitigacao: validacao previa de patch, fallback para stage por hunk completo, mensagens de erro acionaveis.

Risco: arquivos gigantes/binarios degradam UI.

- Mitigacao: limite de preview por tamanho, modo simplificado sem highlight.

Risco: corrida entre watcher e comandos locais.

- Mitigacao: invalida cache por evento + reconciliacao apos cada comando write concluido.

Risco: encoding heterogeneo.

- Mitigacao: leitura defensiva, fallback de exibicao e indicacao de encoding nao suportado.

## 13. Plano de Testes

Backend:

- parser de `git log` com delimitadores seguros
- parser de diff e geracao de patch parcial
- serializacao da fila de comandos write
- validacao/sanitizacao de path

Frontend:

- virtualizacao da lista e paginacao
- selecao de linhas/hunks
- estados de loading/erro/sucesso
- atalhos de teclado criticos

E2E:

- repo com alteracoes simples
- repo com conflito de merge
- arquivo grande e binario
- concorrencia de operacoes write

## 14. Dependencias

- Git CLI disponivel no host
- file watcher ja operacional (existente no ORCH)
- bindings Wails adicionais para Git Panel

## 15. Open Questions

- O nome visual do botao/tela permanece "GitHub" ou muda para "Git" no v1?
- Commit action entra no v1 ou fica para v1.1?
- External merge tool sera configuravel na primeira entrega ou fixo por ambiente?

## 16. Decisao de Escopo Recomendada

Para reduzir risco e entregar valor rapido:

- v1: historico linear + layout inspirado no Git Extensions (sem grafos) + diff + stage/unstage + conflitos basicos
- v1.1: line-level staging completo + console avancado + refinamentos de UX

## Codigos pra ajudar no desenvolvimento

React AI Commit

Show Git commits in your AI interface—perfect for code review assistants or version control tools. Displays commit hash, message, author, timestamp, and a collapsible list of changed files. Each file shows its status (added, modified, deleted, renamed) with color-coded indicators and line change counts. The compact design fits well in chat interfaces while the expandable details let users dig into what changed.

"
"use client"

import { CheckIcon, CopyIcon, FileIcon, GitCommitIcon, MinusIcon, PlusIcon } from "lucide-react"
import { type ComponentProps, type HTMLAttributes, useEffect, useRef, useState } from "react"
/**
 * @title React AI Commit
 * @credit {"name": "Vercel", "url": "https://ai-sdk.dev/elements", "license": {"name": "Apache License 2.0", "url": "https://www.apache.org/licenses/LICENSE-2.0"}}
 * @description React AI commit component for displaying Git commit information with file changes
 * @opening Show Git commits in your AI interface—perfect for code review assistants or version control tools. Displays commit hash, message, author, timestamp, and a collapsible list of changed files. Each file shows its status (added, modified, deleted, renamed) with color-coded indicators and line change counts. The compact design fits well in chat interfaces while the expandable details let users dig into what changed.
 * @related [
 *   {"href":"/ai/code-block","title":"React AI Code Block","description":"Syntax highlighted code"},
 *   {"href":"/ai/file-tree","title":"React AI File Tree","description":"File structure display"},
 *   {"href":"/ai/tool","title":"React AI Tool","description":"Tool execution display"},
 *   {"href":"/ai/artifact","title":"React AI Artifact","description":"Generated content container"},
 *   {"href":"/ai/context","title":"React AI Context","description":"File context display"},
 *   {"href":"/ai/message","title":"React AI Message","description":"Chat message bubbles"}
 * ]
 * @questions [
 *   {"id":"commit-status","title":"What file statuses are supported?","answer":"Four statuses: added (green A), modified (yellow M), deleted (red D), and renamed (blue R). Each gets appropriate color styling automatically."},
 *   {"id":"commit-changes","title":"How do I show line changes?","answer":"Use CommitFileAdditions and CommitFileDeletions with count props. They show +N and -N with green/red colors. Zero counts are hidden."},
 *   {"id":"commit-copy","title":"Can users copy the commit hash?","answer":"CommitCopyButton takes a hash prop and copies it to clipboard. Shows checkmark briefly after copying. Put it in CommitActions."},
 *   {"id":"commit-expand","title":"Is the file list collapsible?","answer":"Yes, the whole Commit is a Collapsible. CommitHeader is the trigger, CommitContent holds the file list. Defaults to collapsed."}
 * ]
 */
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"
import { cn } from "@/lib/utils"

export type CommitProps = ComponentProps<typeof Collapsible>

export const Commit = ({ className, children, ...props }: CommitProps) => (
  <Collapsible className={cn("rounded-lg border bg-background", className)} {...props}>
    {children}
  </Collapsible>
)

export type CommitHeaderProps = ComponentProps<typeof CollapsibleTrigger>

export const CommitHeader = ({ className, children, ...props }: CommitHeaderProps) => (
  <CollapsibleTrigger asChild {...props}>
    <div
      className={cn(
        "group flex cursor-pointer items-center justify-between gap-4 p-3 text-left transition-colors hover:opacity-80",
        className,
      )}
    >
      {children}
    </div>
  </CollapsibleTrigger>
)

export type CommitHashProps = HTMLAttributes<HTMLSpanElement>

export const CommitHash = ({ className, children, ...props }: CommitHashProps) => (
  <span className={cn("font-mono text-xs", className)} {...props}>
    <GitCommitIcon className="mr-1 inline-block size-3" />
    {children}
  </span>
)

export type CommitMessageProps = HTMLAttributes<HTMLSpanElement>

export const CommitMessage = ({ className, children, ...props }: CommitMessageProps) => (
  <span className={cn("font-medium text-sm", className)} {...props}>
    {children}
  </span>
)

export type CommitMetadataProps = HTMLAttributes<HTMLDivElement>

export const CommitMetadata = ({ className, children, ...props }: CommitMetadataProps) => (
  <div
    className={cn("flex items-center gap-2 text-muted-foreground text-xs", className)}
    {...props}
  >
    {children}
  </div>
)

export type CommitSeparatorProps = HTMLAttributes<HTMLSpanElement>

export const CommitSeparator = ({ className, children, ...props }: CommitSeparatorProps) => (
  <span className={className} {...props}>
    {children ?? "•"}
  </span>
)

export type CommitInfoProps = HTMLAttributes<HTMLDivElement>

export const CommitInfo = ({ className, children, ...props }: CommitInfoProps) => (
  <div className={cn("flex flex-1 flex-col", className)} {...props}>
    {children}
  </div>
)

export type CommitAuthorProps = HTMLAttributes<HTMLDivElement>

export const CommitAuthor = ({ className, children, ...props }: CommitAuthorProps) => (
  <div className={cn("flex items-center", className)} {...props}>
    {children}
  </div>
)

export type CommitAuthorAvatarProps = ComponentProps<typeof Avatar> & {
  initials: string
}

export const CommitAuthorAvatar = ({ initials, className, ...props }: CommitAuthorAvatarProps) => (
  <Avatar className={cn("size-8", className)} {...props}>
    <AvatarFallback className="text-xs">{initials}</AvatarFallback>
  </Avatar>
)

export type CommitTimestampProps = HTMLAttributes<HTMLTimeElement> & {
  date: Date
}

export const CommitTimestamp = ({ date, className, children, ...props }: CommitTimestampProps) => {
  const formatted = new Intl.RelativeTimeFormat("en", { numeric: "auto" }).format(
    Math.round((date.getTime() - Date.now()) / (1000 * 60 * 60 * 24)),
    "day",
  )

  return (
    <time
      className={cn("text-xs", className)}
      dateTime={date.toISOString()}
      suppressHydrationWarning
      {...props}
    >
      {children ?? formatted}
    </time>
  )
}

export type CommitActionsProps = HTMLAttributes<HTMLDivElement>

export const CommitActions = ({ className, children, ...props }: CommitActionsProps) => (
  <div
    className={cn("flex items-center gap-1", className)}
    onClick={e => e.stopPropagation()}
    onKeyDown={e => e.stopPropagation()}
    role="group"
    {...props}
  >
    {children}
  </div>
)

export type CommitCopyButtonProps = ComponentProps<typeof Button> & {
  hash: string
  onCopy?: () => void
  onError?: (error: Error) => void
  timeout?: number
}

export const CommitCopyButton = ({
  hash,
  onCopy,
  onError,
  timeout = 2000,
  children,
  className,
  ...props
}: CommitCopyButtonProps) => {
  const [isCopied, setIsCopied] = useState(false)
  const timeoutRef = useRef<number>(0)

  const copyToClipboard = async () => {
    if (typeof window === "undefined" || !navigator?.clipboard?.writeText) {
      onError?.(new Error("Clipboard API not available"))
      return
    }

    try {
      if (!isCopied) {
        await navigator.clipboard.writeText(hash)
        setIsCopied(true)
        onCopy?.()
        timeoutRef.current = window.setTimeout(() => setIsCopied(false), timeout)
      }
    } catch (error) {
      onError?.(error as Error)
    }
  }

  useEffect(
    () => () => {
      window.clearTimeout(timeoutRef.current)
    },
    [],
  )

  const Icon = isCopied ? CheckIcon : CopyIcon

  return (
    <Button
      className={cn("size-7 shrink-0", className)}
      onClick={copyToClipboard}
      size="icon"
      variant="ghost"
      {...props}
    >
      {children ?? <Icon size={14} />}
    </Button>
  )
}

export type CommitContentProps = ComponentProps<typeof CollapsibleContent>

export const CommitContent = ({ className, children, ...props }: CommitContentProps) => (
  <CollapsibleContent className={cn("border-t p-3", className)} {...props}>
    {children}
  </CollapsibleContent>
)

export type CommitFilesProps = HTMLAttributes<HTMLDivElement>

export const CommitFiles = ({ className, children, ...props }: CommitFilesProps) => (
  <div className={cn("space-y-1", className)} {...props}>
    {children}
  </div>
)

export type CommitFileProps = HTMLAttributes<HTMLDivElement>

export const CommitFile = ({ className, children, ...props }: CommitFileProps) => (
  <div
    className={cn(
      "flex items-center justify-between gap-2 rounded px-2 py-1 text-sm hover:bg-muted/50",
      className,
    )}
    {...props}
  >
    {children}
  </div>
)

export type CommitFileInfoProps = HTMLAttributes<HTMLDivElement>

export const CommitFileInfo = ({ className, children, ...props }: CommitFileInfoProps) => (
  <div className={cn("flex min-w-0 items-center gap-2", className)} {...props}>
    {children}
  </div>
)

const fileStatusStyles = {
  added: "text-green-600 dark:text-green-400",
  modified: "text-yellow-600 dark:text-yellow-400",
  deleted: "text-red-600 dark:text-red-400",
  renamed: "text-blue-600 dark:text-blue-400",
}

const fileStatusLabels = {
  added: "A",
  modified: "M",
  deleted: "D",
  renamed: "R",
}

export type CommitFileStatusProps = HTMLAttributes<HTMLSpanElement> & {
  status: "added" | "modified" | "deleted" | "renamed"
}

export const CommitFileStatus = ({
  status,
  className,
  children,
  ...props
}: CommitFileStatusProps) => (
  <span
    className={cn("font-medium font-mono text-xs", fileStatusStyles[status], className)}
    {...props}
  >
    {children ?? fileStatusLabels[status]}
  </span>
)

export type CommitFileIconProps = ComponentProps<typeof FileIcon>

export const CommitFileIcon = ({ className, ...props }: CommitFileIconProps) => (
  <FileIcon className={cn("size-3.5 shrink-0 text-muted-foreground", className)} {...props} />
)

export type CommitFilePathProps = HTMLAttributes<HTMLSpanElement>

export const CommitFilePath = ({ className, children, ...props }: CommitFilePathProps) => (
  <span className={cn("truncate font-mono text-xs", className)} {...props}>
    {children}
  </span>
)

export type CommitFileChangesProps = HTMLAttributes<HTMLDivElement>

export const CommitFileChanges = ({ className, children, ...props }: CommitFileChangesProps) => (
  <div className={cn("flex shrink-0 items-center gap-1 font-mono text-xs", className)} {...props}>
    {children}
  </div>
)

export type CommitFileAdditionsProps = HTMLAttributes<HTMLSpanElement> & {
  count: number
}

export const CommitFileAdditions = ({
  count,
  className,
  children,
  ...props
}: CommitFileAdditionsProps) => {
  if (count <= 0) return null
  return (
    <span className={cn("text-green-600 dark:text-green-400", className)} {...props}>
      {children ?? (
        <>
          <PlusIcon className="inline-block size-3" />
          {count}
        </>
      )}
    </span>
  )
}

export type CommitFileDeletionsProps = HTMLAttributes<HTMLSpanElement> & {
  count: number
}

export const CommitFileDeletions = ({
  count,
  className,
  children,
  ...props
}: CommitFileDeletionsProps) => {
  if (count <= 0) return null
  return (
    <span className={cn("text-red-600 dark:text-red-400", className)} {...props}>
      {children ?? (
        <>
          <MinusIcon className="inline-block size-3" />
          {count}
        </>
      )}
    </span>
  )
}

/** Demo component for preview */
export default function CommitDemo() {
  return (
    <div className="w-full max-w-lg p-4">
      <Commit defaultOpen>
        <CommitHeader>
          <CommitInfo>
            <CommitMessage>Add user authentication flow</CommitMessage>
            <CommitMetadata>
              <CommitHash>a1b2c3d</CommitHash>
              <CommitSeparator />
              <span>John Doe</span>
              <CommitSeparator />
              <CommitTimestamp date={new Date(Date.now() - 86400000)} />
            </CommitMetadata>
          </CommitInfo>
          <CommitActions>
            <CommitCopyButton hash="a1b2c3d4e5f6" />
          </CommitActions>
        </CommitHeader>
        <CommitContent>
          <CommitFiles>
            <CommitFile>
              <CommitFileInfo>
                <CommitFileStatus status="added" />
                <CommitFileIcon />
                <CommitFilePath>src/auth/login.tsx</CommitFilePath>
              </CommitFileInfo>
              <CommitFileChanges>
                <CommitFileAdditions count={45} />
              </CommitFileChanges>
            </CommitFile>
            <CommitFile>
              <CommitFileInfo>
                <CommitFileStatus status="modified" />
                <CommitFileIcon />
                <CommitFilePath>src/app.tsx</CommitFilePath>
              </CommitFileInfo>
              <CommitFileChanges>
                <CommitFileAdditions count={12} />
                <CommitFileDeletions count={3} />
              </CommitFileChanges>
            </CommitFile>
          </CommitFiles>
        </CommitContent>
      </Commit>
    </div>
  )
}
"

React AI Code Block
React AI code block component with Shiki syntax highlighting, copy button, and dark mode support for chat interfaces

If your AI generates code (and let's be honest, that's like 90% of what people use AI for), you need a solid code block. This one uses Shiki under the hood, so you get proper syntax highlighting for pretty much any language—TypeScript, Python, Rust, whatever. It handles dark mode automatically by rendering both themes and showing the right one based on your app's theme class. There's a copy button built in because obviously users want to copy the code. The highlighting is async so it won't block rendering while Shiki does its thing, which matters a lot when you're streaming code in real-time. Just pass your code and language, and it looks good.

"use client"

import { CheckIcon, CopyIcon } from "lucide-react"
import {
  type ComponentProps,
  createContext,
  type HTMLAttributes,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react"
import { type BundledLanguage, codeToHtml, type ShikiTransformer } from "shiki"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

type CodeBlockProps = HTMLAttributes<HTMLDivElement> & {
  code: string
  language: BundledLanguage
  showLineNumbers?: boolean
}

interface CodeBlockContextType {
  code: string
}

const CodeBlockContext = createContext<CodeBlockContextType>({
  code: "",
})

const lineNumberTransformer: ShikiTransformer = {
  name: "line-numbers",
  line(node, line) {
    node.children.unshift({
      type: "element",
      tagName: "span",
      properties: {
        className: [
          "inline-block",
          "min-w-10",
          "mr-4",
          "text-right",
          "select-none",
          "text-muted-foreground",
        ],
      },
      children: [{ type: "text", value: String(line) }],
    })
  },
}

export async function highlightCode(
  code: string,
  language: BundledLanguage,
  showLineNumbers = false,
) {
  const transformers: ShikiTransformer[] = showLineNumbers ? [lineNumberTransformer] : []

  return await Promise.all([
    codeToHtml(code, {
      lang: language,
      theme: "one-light",
      transformers,
    }),
    codeToHtml(code, {
      lang: language,
      theme: "one-dark-pro",
      transformers,
    }),
  ])
}

export const CodeBlock = ({
  code,
  language,
  showLineNumbers = false,
  className,
  children,
  ...props
}: CodeBlockProps) => {
  const [html, setHtml] = useState<string>("")
  const [darkHtml, setDarkHtml] = useState<string>("")
  const mounted = useRef(false)

  useEffect(() => {
    highlightCode(code, language, showLineNumbers).then(([light, dark]) => {
      if (!mounted.current) {
        setHtml(light)
        setDarkHtml(dark)
        mounted.current = true
      }
    })

    return () => {
      mounted.current = false
    }
  }, [code, language, showLineNumbers])

  return (
    <CodeBlockContext.Provider value={{ code }}>
      <div
        className={cn(
          "group relative w-full overflow-hidden rounded-md border bg-background text-foreground",
          className,
        )}
        {...props}
      >
        <div className="relative">
          <div
            className="overflow-auto dark:hidden [&>pre]:m-0 [&>pre]:bg-background! [&>pre]:p-4 [&>pre]:text-foreground! [&>pre]:text-sm [&_code]:font-mono [&_code]:text-sm"
            dangerouslySetInnerHTML={{ __html: html }}
          />
          <div
            className="hidden overflow-auto dark:block [&>pre]:m-0 [&>pre]:bg-background! [&>pre]:p-4 [&>pre]:text-foreground! [&>pre]:text-sm [&_code]:font-mono [&_code]:text-sm"
            dangerouslySetInnerHTML={{ __html: darkHtml }}
          />
          {children && (
            <div className="absolute top-2 right-2 flex items-center gap-2">{children}</div>
          )}
        </div>
      </div>
    </CodeBlockContext.Provider>
  )
}

export type CodeBlockCopyButtonProps = ComponentProps<typeof Button> & {
  onCopy?: () => void
  onError?: (error: Error) => void
  timeout?: number
}

export const CodeBlockCopyButton = ({
  onCopy,
  onError,
  timeout = 2000,
  children,
  className,
  ...props
}: CodeBlockCopyButtonProps) => {
  const [isCopied, setIsCopied] = useState(false)
  const { code } = useContext(CodeBlockContext)

  const copyToClipboard = async () => {
    if (typeof window === "undefined" || !navigator?.clipboard?.writeText) {
      onError?.(new Error("Clipboard API not available"))
      return
    }

    try {
      await navigator.clipboard.writeText(code)
      setIsCopied(true)
      onCopy?.()
      setTimeout(() => setIsCopied(false), timeout)
    } catch (error) {
      onError?.(error as Error)
    }
  }

  const Icon = isCopied ? CheckIcon : CopyIcon

  return (
    <Button
      className={cn("shrink-0", className)}
      onClick={copyToClipboard}
      size="icon"
      variant="ghost"
      {...props}
    >
      {children ?? <Icon size={14} />}
    </Button>
  )
}

/** Demo component for preview */
export default function CodeBlockDemo() {
  const code = `function MyComponent(props) {
  return (
    <div>
      <h1>Hello, {props.name}!</h1>
      <p>This is an example React component.</p>
    </div>
  );
}`

  return (
    <div className="w-full max-w-2xl p-6">
      <CodeBlock code={code} language="jsx">
        <CodeBlockCopyButton
          onCopy={() => console.log("Copied code to clipboard")}
          onError={() => console.error("Failed to copy code to clipboard")}
        />
      </CodeBlock>
    </div>
  )
}
