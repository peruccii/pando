export type GitPanelPRListState = 'open' | 'closed' | 'all'

export interface GitPanelPRUser {
  login: string
  avatarUrl: string
}

export interface GitPanelPRLabel {
  name: string
  color: string
  description?: string
}

export interface GitPanelPullRequest {
  id: string
  number: number
  title: string
  body: string
  state: string
  author: GitPanelPRUser
  reviewers: GitPanelPRUser[]
  labels: GitPanelPRLabel[]
  createdAt: string
  updatedAt: string
  mergeCommit?: string
  headBranch: string
  baseBranch: string
  additions: number
  deletions: number
  changedFiles: number
  isDraft: boolean
  maintainerCanModify?: boolean
}

export interface GitPanelPRCommit {
  sha: string
  message: string
  additions?: number
  deletions?: number
  changedFiles?: number
  htmlUrl?: string
  authorName?: string
  authorEmail?: string
  authoredAt?: string
  committerName?: string
  committerEmail?: string
  committedAt?: string
  author?: GitPanelPRUser
  committer?: GitPanelPRUser
  parentShas?: string[]
}

export interface GitPanelPRCommitPage {
  items: GitPanelPRCommit[]
  page: number
  perPage: number
  hasNextPage: boolean
  nextPage?: number
}

export interface GitPanelPRFile {
  filename: string
  previousFilename?: string
  status: string
  additions: number
  deletions: number
  changes: number
  blobUrl?: string
  rawUrl?: string
  contentsUrl?: string
  patch?: string
  hasPatch: boolean
  patchState: string
  isBinary: boolean
  isPatchTruncated: boolean
}

export interface GitPanelPRFilePage {
  items: GitPanelPRFile[]
  page: number
  perPage: number
  hasNextPage: boolean
  nextPage?: number
}
