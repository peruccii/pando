// GitHub Integration Types — mirrors Go backend structs

export interface GitHubUser {
  login: string
  avatarUrl: string
}

export interface GitHubLabel {
  name: string
  color: string
  description?: string
}

export interface Repository {
  id: string
  name: string
  fullName: string
  owner: string
  description: string
  isPrivate: boolean
  defaultBranch: string
  updatedAt: string
}

export interface PullRequest {
  id: string
  number: number
  title: string
  body: string
  state: 'OPEN' | 'CLOSED' | 'MERGED'
  author: GitHubUser
  reviewers: GitHubUser[]
  labels: GitHubLabel[]
  createdAt: string
  updatedAt: string
  mergeCommit?: string
  headBranch: string
  baseBranch: string
  additions: number
  deletions: number
  changedFiles: number
  isDraft: boolean
}

export interface Diff {
  files: DiffFile[]
  totalFiles: number
  pagination: DiffPagination
}

export interface DiffFile {
  filename: string
  status: 'added' | 'modified' | 'deleted' | 'renamed'
  additions: number
  deletions: number
  patch?: string
  hunks: DiffHunk[]
}

export interface DiffHunk {
  oldStart: number
  oldLines: number
  newStart: number
  newLines: number
  header: string
  lines: DiffLine[]
}

export interface DiffLine {
  type: 'add' | 'delete' | 'context'
  content: string
  oldLine?: number
  newLine?: number
}

export interface DiffPagination {
  first: number
  after?: string
  hasNextPage: boolean
  endCursor?: string
}

export interface Review {
  id: string
  author: GitHubUser
  state: 'APPROVED' | 'CHANGES_REQUESTED' | 'COMMENTED' | 'PENDING'
  body: string
  createdAt: string
}

export interface Comment {
  id: string
  author: GitHubUser
  body: string
  path?: string
  line?: number
  createdAt: string
  updatedAt: string
}

export interface Issue {
  id: string
  number: number
  title: string
  body: string
  state: 'OPEN' | 'CLOSED'
  author: GitHubUser
  assignees: GitHubUser[]
  labels: GitHubLabel[]
  createdAt: string
  updatedAt: string
}

export interface Branch {
  name: string
  prefix: string
  commit: string
}

export type MergeMethod = 'MERGE' | 'SQUASH' | 'REBASE'

export type PRState = 'OPEN' | 'CLOSED' | 'MERGED' | 'ALL'
export type IssueState = 'OPEN' | 'CLOSED' | 'ALL'

// Kanban Columns for Issue Board
export type KanbanColumn = 'backlog' | 'in_progress' | 'done'

export interface GitHubError {
  statusCode: number
  message: string
  type: 'auth' | 'ratelimit' | 'notfound' | 'permission' | 'conflict' | 'network' | 'graphql' | 'unknown'
}

// View modes
export type DiffViewMode = 'unified' | 'side-by-side'
export type GitHubView = 'prs' | 'issues' | 'branches'
