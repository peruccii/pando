import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import * as AppBindings from '../../../../wailsjs/go/main/App'
import { PR_ERROR_CODES } from '../../github/types/prBindingError'
import type {
  GitPanelPRCommit,
  GitPanelPRFile,
  GitPanelPRFilePage,
  GitPanelPullRequest,
} from '../types/pullRequests'
import { useGitPanelPRStore } from '../stores/gitPanelPRStore'
import { GitPanelPRView } from './GitPanelPRView'

vi.mock('../../../../wailsjs/go/main/App', () => ({
  AuthLogin: vi.fn(),
  GHListBranches: vi.fn(),
  GitPanelGetStatus: vi.fn(),
  GitPanelPRCreate: vi.fn(),
  GitPanelPRCreateLabel: vi.fn(),
  GitPanelPRPushLocalBranch: vi.fn(),
  GitPanelPRUpdate: vi.fn(),
  GitPanelPRResolveRepository: vi.fn(),
  SetPollingContext: vi.fn(),
  StartPolling: vi.fn(),
  StopPolling: vi.fn(),
  GitPanelPRGet: vi.fn(),
  GitPanelPRGetCommits: vi.fn(),
  GitPanelPRGetCommitRawDiff: vi.fn(),
  GitPanelPRGetFiles: vi.fn(),
  GitPanelPRGetRawDiff: vi.fn(),
  GitPanelPRList: vi.fn(),
}))

function createPullRequest(number: number): GitPanelPullRequest {
  const timestamp = '2026-02-24T11:00:00.000Z'
  return {
    id: `pr-${number}`,
    number,
    title: 'feat: PR principal',
    body: 'Descricao da mudanca',
    state: 'open',
    author: { login: 'alice', avatarUrl: '' },
    reviewers: [],
    labels: [{ name: 'backend', color: 'blue' }],
    createdAt: timestamp,
    updatedAt: timestamp,
    headBranch: 'feature/principal',
    baseBranch: 'main',
    additions: 16,
    deletions: 3,
    changedFiles: 2,
    isDraft: false,
    maintainerCanModify: true,
  }
}

function createFile(
  overrides: Partial<GitPanelPRFile> & Pick<GitPanelPRFile, 'filename'>,
): GitPanelPRFile {
  return {
    filename: overrides.filename,
    status: overrides.status ?? 'modified',
    additions: overrides.additions ?? 8,
    deletions: overrides.deletions ?? 2,
    changes: overrides.changes ?? 10,
    patch: overrides.patch ?? '@@ -1 +1 @@\n-old\n+new',
    hasPatch: overrides.hasPatch ?? true,
    patchState: overrides.patchState ?? 'ready',
    isBinary: overrides.isBinary ?? false,
    isPatchTruncated: overrides.isPatchTruncated ?? false,
    previousFilename: overrides.previousFilename,
    blobUrl: overrides.blobUrl,
    rawUrl: overrides.rawUrl,
    contentsUrl: overrides.contentsUrl,
  }
}

function createFilesPage(items: GitPanelPRFile[]): GitPanelPRFilePage {
  return {
    items,
    page: 1,
    perPage: 25,
    hasNextPage: false,
  }
}

function createCommit(sha: string, message: string): GitPanelPRCommit {
  return {
    sha,
    message,
    authorName: 'alice',
    authoredAt: '2026-02-24T10:30:00.000Z',
  }
}

describe('GitPanelPRView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useGitPanelPRStore.getState().reset()

    vi.mocked(AppBindings.GitPanelPRResolveRepository).mockResolvedValue({
      owner: 'acme',
      repo: 'orch',
    } as never)
    vi.mocked(AppBindings.StartPolling).mockResolvedValue(undefined)
    vi.mocked(AppBindings.StopPolling).mockResolvedValue(undefined)
    vi.mocked(AppBindings.SetPollingContext).mockResolvedValue(undefined)
    vi.mocked(AppBindings.AuthLogin).mockResolvedValue(undefined)
    vi.mocked(AppBindings.GitPanelPRPushLocalBranch).mockResolvedValue(undefined)
    vi.mocked(AppBindings.GitPanelPRCreateLabel).mockResolvedValue({
      name: 'bug',
      color: 'd73a4a',
      description: '',
    } as never)
    vi.mocked(AppBindings.GitPanelGetStatus).mockResolvedValue({
      branch: 'feature/principal',
      ahead: 2,
      behind: 0,
      staged: [],
      unstaged: [],
      conflicted: [],
    } as never)
    vi.mocked(AppBindings.GHListBranches).mockResolvedValue([
      { name: 'main', prefix: 'refs/heads/', commit: 'abc1234' },
      { name: 'feature/principal', prefix: 'refs/heads/', commit: 'def5678' },
    ] as never)

    vi.mocked(AppBindings.GitPanelPRGetCommits).mockResolvedValue({
      items: [],
      page: 1,
      perPage: 25,
      hasNextPage: false,
    } as never)
    vi.mocked(AppBindings.GitPanelPRGetCommitRawDiff).mockResolvedValue('diff --git a b')
    vi.mocked(AppBindings.GitPanelPRGetRawDiff).mockResolvedValue('diff --git a b')
  })

  afterEach(() => {
    cleanup()
    useGitPanelPRStore.getState().reset()
  })

  it('renderiza lista, detalhe e degradacao de arquivos truncados/binarios', async () => {
    const pr = createPullRequest(101)
    vi.mocked(AppBindings.GitPanelPRList).mockResolvedValue([pr] as never)
    vi.mocked(AppBindings.GitPanelPRGet).mockResolvedValue(pr as never)
    vi.mocked(AppBindings.GitPanelPRGetFiles).mockResolvedValue(
      createFilesPage([
        createFile({
          filename: 'src/feature.ts',
          isPatchTruncated: true,
          patch: '@@ -1 +1 @@\n-old\n+new\n+extra',
        }),
        createFile({
          filename: 'assets/logo.png',
          hasPatch: false,
          isBinary: true,
          patch: '',
          patchState: 'binary',
        }),
      ]) as never,
    )

    render(<GitPanelPRView repoPath="/tmp/repo" />)

    await screen.findByRole('listbox', { name: 'Pull Requests' })
    await screen.findByText('Autor: @alice')
    fireEvent.click(screen.getByRole('button', { name: /files changed/i }))
    await screen.findByText('2 arquivos carregados')
    expect(screen.getByText('1 binário(s)')).toBeInTheDocument()
    expect(screen.getByText('1 truncado(s)')).toBeInTheDocument()
    expect(screen.getByText('1 sem patch renderizável')).toBeInTheDocument()
    expect(screen.getByText('Arquivo binário sem patch renderizável.')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Exibir patch' }))
    expect(await screen.findByText('Patch truncado pelo GitHub (exibindo parcial).')).toBeInTheDocument()
  })

  it('abre diff ao clicar em commit do detalhe', async () => {
    const pr = createPullRequest(188)
    vi.mocked(AppBindings.GitPanelPRList).mockResolvedValue([pr] as never)
    vi.mocked(AppBindings.GitPanelPRGet).mockResolvedValue(pr as never)
    vi.mocked(AppBindings.GitPanelPRGetFiles).mockResolvedValue(createFilesPage([
      createFile({ filename: 'src/core.ts' }),
    ]) as never)
    vi.mocked(AppBindings.GitPanelPRGetCommits).mockResolvedValue({
      items: [createCommit('abcdef1234567890', 'feat: aplicar fluxo de diff')],
      page: 1,
      perPage: 25,
      hasNextPage: false,
    } as never)
    vi.mocked(AppBindings.GitPanelPRGetCommitRawDiff).mockResolvedValue('diff --git a/src/core.ts b/src/core.ts')

    render(<GitPanelPRView repoPath="/tmp/repo" />)

    fireEvent.click(await screen.findByRole('button', { name: /commits/i }))
    fireEvent.click(await screen.findByRole('button', { name: /feat: aplicar fluxo de diff/i }))

    expect(await screen.findByText('Diff do commit abcdef1')).toBeInTheDocument()
    expect(screen.getByText('diff --git a/src/core.ts b/src/core.ts')).toBeInTheDocument()
    expect(AppBindings.GitPanelPRGetCommitRawDiff).toHaveBeenCalledWith('/tmp/repo', 188, 'abcdef1234567890')
  })

  it('exibe erro acionavel quando listagem falha', async () => {
    vi.mocked(AppBindings.GitPanelPRList).mockRejectedValue({
      code: PR_ERROR_CODES.forbidden,
      message: 'Sem permissao para listar PRs.',
      details: 'secondary rate limit',
    })

    render(<GitPanelPRView repoPath="/tmp/repo" />)

    expect(await screen.findByText('Falha ao carregar lista de PRs')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Tentar novamente' })).toBeInTheDocument()
    expect(
      screen.getByText(
        'Permissao insuficiente ou limite secundario ativo. Aguarde alguns segundos e tente novamente com menor frequencia.',
      ),
    ).toBeInTheDocument()
  })

  it('degrada para preview quando patch excede limite de render completo', async () => {
    const pr = createPullRequest(303)
    const oversizedPatch = `@@ -1 +1 @@\n+${'x'.repeat(81_000)}`
    vi.mocked(AppBindings.GitPanelPRList).mockResolvedValue([pr] as never)
    vi.mocked(AppBindings.GitPanelPRGet).mockResolvedValue(pr as never)
    vi.mocked(AppBindings.GitPanelPRGetFiles).mockResolvedValue(
      createFilesPage([
        createFile({
          filename: 'src/big-diff.ts',
          patch: oversizedPatch,
        }),
      ]) as never,
    )

    render(<GitPanelPRView repoPath="/tmp/repo" />)

    fireEvent.click(await screen.findByRole('button', { name: /files changed/i }))
    await screen.findAllByText('src/big-diff.ts')
    fireEvent.click(screen.getByRole('button', { name: 'Exibir patch' }))

    expect(await screen.findByText('Patch grande detectado. Exibindo preview para preservar responsividade.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Renderizar patch completo' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Renderizar patch completo' }))
    await waitFor(() => {
      expect(screen.queryByText('Patch grande detectado. Exibindo preview para preservar responsividade.')).not.toBeInTheDocument()
    })
  })

  it('preenche head automaticamente com branch atual (read-only) e base default main', async () => {
    const pr = createPullRequest(404)
    vi.mocked(AppBindings.GitPanelPRList).mockResolvedValue([pr] as never)
    vi.mocked(AppBindings.GitPanelPRGet).mockResolvedValue(pr as never)
    vi.mocked(AppBindings.GitPanelPRGetFiles).mockResolvedValue(createFilesPage([]) as never)

    render(<GitPanelPRView repoPath="/tmp/repo" />)

    await screen.findByRole('listbox', { name: 'Pull Requests' })
    fireEvent.click(screen.getByRole('button', { name: 'Nova PR' }))

    const headInput = await screen.findByLabelText('Head (branch atual) *')
    expect(headInput).toHaveValue('feature/principal')
    expect(headInput).toHaveAttribute('readonly')
    expect(screen.getByLabelText('Base *')).toHaveValue('main')
  })

  it('botao inteligente faz push -u e cria PR quando branch ainda nao existe no remoto', async () => {
    const pr = createPullRequest(405)
    const createdPR = createPullRequest(406)
    vi.mocked(AppBindings.GitPanelPRList).mockResolvedValue([pr] as never)
    vi.mocked(AppBindings.GitPanelPRGet).mockResolvedValue(pr as never)
    vi.mocked(AppBindings.GitPanelPRGetFiles).mockResolvedValue(createFilesPage([]) as never)
    vi.mocked(AppBindings.GHListBranches).mockResolvedValue([
      { name: 'main', prefix: 'refs/heads/', commit: 'abc1234' },
    ] as never)
    vi.mocked(AppBindings.GitPanelPRCreate).mockResolvedValue(createdPR as never)

    render(<GitPanelPRView repoPath="/tmp/repo" />)

    await screen.findByRole('listbox', { name: 'Pull Requests' })
    fireEvent.click(screen.getByRole('button', { name: 'Nova PR' }))
    await waitFor(() => {
      expect(screen.getByLabelText('Head (branch atual) *')).toHaveValue('feature/principal')
    })
    fireEvent.change(screen.getByLabelText('Titulo *'), { target: { value: 'feat: publicar e abrir PR' } })
    fireEvent.click(screen.getByRole('button', { name: 'Criar PR' }))

    await waitFor(() => {
      expect(AppBindings.GitPanelPRPushLocalBranch).toHaveBeenCalledWith('/tmp/repo', 'feature/principal')
    })
    expect(AppBindings.GitPanelPRCreate).toHaveBeenCalledWith('/tmp/repo', expect.objectContaining({
      head: 'feature/principal',
      base: 'main',
    }))
  })

  it('bloqueia submit quando ahead=0 com mensagem de preflight', async () => {
    const pr = createPullRequest(407)
    vi.mocked(AppBindings.GitPanelPRList).mockResolvedValue([pr] as never)
    vi.mocked(AppBindings.GitPanelPRGet).mockResolvedValue(pr as never)
    vi.mocked(AppBindings.GitPanelPRGetFiles).mockResolvedValue(createFilesPage([]) as never)
    vi.mocked(AppBindings.GitPanelGetStatus).mockResolvedValue({
      branch: 'feature/principal',
      ahead: 0,
      behind: 0,
      staged: [],
      unstaged: [],
      conflicted: [],
    } as never)

    render(<GitPanelPRView repoPath="/tmp/repo" />)

    await screen.findByRole('listbox', { name: 'Pull Requests' })
    fireEvent.click(screen.getByRole('button', { name: 'Nova PR' }))
    await waitFor(() => {
      expect(screen.getByLabelText('Head (branch atual) *')).toHaveValue('feature/principal')
    })
    fireEvent.change(screen.getByLabelText('Titulo *'), { target: { value: 'feat: sem commits' } })
    fireEvent.click(screen.getByRole('button', { name: 'Criar PR' }))

    expect(await screen.findByText('sem commits para abrir PR')).toBeInTheDocument()
    expect(AppBindings.GitPanelPRCreate).not.toHaveBeenCalled()
  })

  it('modo avancado aceita owner:branch e override manual no payload', async () => {
    const pr = createPullRequest(408)
    const createdPR = createPullRequest(409)
    vi.mocked(AppBindings.GitPanelPRList).mockResolvedValue([pr] as never)
    vi.mocked(AppBindings.GitPanelPRGet).mockResolvedValue(pr as never)
    vi.mocked(AppBindings.GitPanelPRGetFiles).mockResolvedValue(createFilesPage([]) as never)
    vi.mocked(AppBindings.GitPanelPRCreate).mockResolvedValue(createdPR as never)

    render(<GitPanelPRView repoPath="/tmp/repo" />)

    await screen.findByRole('listbox', { name: 'Pull Requests' })
    fireEvent.click(screen.getByRole('button', { name: 'Nova PR' }))
    fireEvent.click(screen.getByLabelText('Modo avancado (fork + override manual)'))
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Atualizar contexto' })).not.toBeDisabled()
    })
    fireEvent.change(screen.getByLabelText('Titulo *'), { target: { value: 'feat: fork pr' } })
    fireEvent.change(screen.getByLabelText('Head *'), { target: { value: 'fork-owner:feature/principal' } })
    fireEvent.change(screen.getByLabelText('Owner destino (override opcional)'), { target: { value: 'fork-owner' } })
    fireEvent.change(screen.getByLabelText('Repo destino (override opcional)'), { target: { value: 'orch-fork' } })
    fireEvent.click(screen.getByLabelText('Confirmar override manual de owner/repo'))
    fireEvent.click(screen.getByRole('button', { name: 'Criar PR' }))

    await waitFor(() => {
      expect(AppBindings.GitPanelPRCreate).toHaveBeenCalledWith('/tmp/repo', expect.objectContaining({
        head: 'fork-owner:feature/principal',
        manualOwner: 'fork-owner',
        manualRepo: 'orch-fork',
        allowTargetOverride: true,
      }))
    })
  })

  it('nao quebra quando create/detalhe retornam arrays nulos no payload de PR', async () => {
    const pr = createPullRequest(510)
    const createdPR = {
      ...createPullRequest(511),
      title: 'feat: payload parcial',
      labels: null,
      reviewers: null,
    } as unknown as GitPanelPullRequest

    let listCallCount = 0
    vi.mocked(AppBindings.GitPanelPRList).mockImplementation(async () => {
      listCallCount++
      if (listCallCount === 1) {
        return [pr] as never
      }
      return [createdPR, pr] as never
    })

    vi.mocked(AppBindings.GitPanelPRGet).mockImplementation(async (_repoPath, prNumber) => {
      return (prNumber === 511 ? createdPR : pr) as never
    })
    vi.mocked(AppBindings.GitPanelPRGetFiles).mockResolvedValue(createFilesPage([]) as never)
    vi.mocked(AppBindings.GitPanelPRCreate).mockResolvedValue(createdPR as never)

    render(<GitPanelPRView repoPath="/tmp/repo" />)

    await screen.findByRole('listbox', { name: 'Pull Requests' })
    fireEvent.click(screen.getByRole('button', { name: 'Nova PR' }))
    await waitFor(() => {
      expect(screen.getByLabelText('Head (branch atual) *')).toHaveValue('feature/principal')
    })
    fireEvent.change(screen.getByLabelText('Titulo *'), { target: { value: 'feat: payload parcial' } })
    fireEvent.click(screen.getByRole('button', { name: 'Criar PR' }))

    await waitFor(() => {
      expect(AppBindings.GitPanelPRCreate).toHaveBeenCalledTimes(1)
    })
    const detailRegion = await screen.findByRole('region', { name: 'Detalhe da Pull Request' })
    expect(within(detailRegion).getByText('#511')).toBeInTheDocument()
  })

  it('cria etiqueta rapida com nome/cor obrigatorios', async () => {
    const pr = createPullRequest(520)
    vi.mocked(AppBindings.GitPanelPRList).mockResolvedValue([pr] as never)
    vi.mocked(AppBindings.GitPanelPRGet).mockResolvedValue(pr as never)
    vi.mocked(AppBindings.GitPanelPRGetFiles).mockResolvedValue(createFilesPage([]) as never)

    render(<GitPanelPRView repoPath="/tmp/repo" />)

    await screen.findByRole('listbox', { name: 'Pull Requests' })
    fireEvent.click(screen.getByRole('button', { name: 'Nova etiqueta' }))

    fireEvent.click(screen.getByRole('button', { name: 'Criar etiqueta' }))
    expect(await screen.findByText('Nome obrigatorio.')).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('Nome *'), { target: { value: 'bug' } })
    fireEvent.change(screen.getByLabelText('Cor *'), { target: { value: '#d73a4a' } })
    fireEvent.change(screen.getByLabelText('Descricao (opcional)'), { target: { value: 'Erro critico de produção' } })
    fireEvent.click(screen.getByRole('button', { name: 'Criar etiqueta' }))

    await waitFor(() => {
      expect(AppBindings.GitPanelPRCreateLabel).toHaveBeenCalledWith('/tmp/repo', expect.objectContaining({
        name: 'bug',
        color: '#d73a4a',
        description: 'Erro critico de produção',
      }))
    })
    expect(await screen.findByText('Etiqueta criada')).toBeInTheDocument()
  })
})
