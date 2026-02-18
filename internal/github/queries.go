package github

// GraphQL queries para a GitHub API v4

// QueryListPullRequests busca PRs de um repositório
const QueryListPullRequests = `
query ListPullRequests($owner: String!, $repo: String!, $first: Int!, $after: String, $states: [PullRequestState!]) {
  repository(owner: $owner, name: $repo) {
    pullRequests(first: $first, after: $after, states: $states, orderBy: {field: UPDATED_AT, direction: DESC}) {
      totalCount
      pageInfo {
        hasNextPage
        endCursor
      }
      nodes {
        id
        number
        title
        body
        state
        isDraft
        createdAt
        updatedAt
        additions
        deletions
        changedFiles
        headRefName
        baseRefName
        mergeCommit {
          oid
        }
        author {
          login
          avatarUrl
        }
        labels(first: 10) {
          nodes {
            name
            color
          }
        }
        reviewRequests(first: 10) {
          nodes {
            requestedReviewer {
              ... on User {
                login
                avatarUrl
              }
            }
          }
        }
      }
    }
  }
}
`

// QueryGetPullRequest busca um PR específico
const QueryGetPullRequest = `
query GetPullRequest($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      id
      number
      title
      body
      state
      isDraft
      createdAt
      updatedAt
      additions
      deletions
      changedFiles
      headRefName
      baseRefName
      mergeCommit {
        oid
      }
      author {
        login
        avatarUrl
      }
      labels(first: 20) {
        nodes {
          name
          color
        }
      }
      reviewRequests(first: 10) {
        nodes {
          requestedReviewer {
            ... on User {
              login
              avatarUrl
            }
          }
        }
      }
    }
  }
}
`

// QueryGetPRDiff busca o diff de um PR
const QueryGetPRDiff = `
query GetPRDiff($owner: String!, $repo: String!, $number: Int!, $first: Int!, $after: String) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      files(first: $first, after: $after) {
        totalCount
        pageInfo {
          hasNextPage
          endCursor
        }
        nodes {
          path
          additions
          deletions
          changeType
          patch
        }
      }
    }
  }
}
`

// QueryListReviews busca reviews de um PR
const QueryListReviews = `
query ListReviews($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      reviews(first: 50, orderBy: {field: CREATED_AT, direction: DESC}) {
        nodes {
          id
          state
          body
          createdAt
          author {
            login
            avatarUrl
          }
        }
      }
    }
  }
}
`

// QueryListComments busca comentários de um PR
const QueryListComments = `
query ListComments($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      comments(first: 100, orderBy: {field: CREATED_AT, direction: ASC}) {
        nodes {
          id
          body
          createdAt
          updatedAt
          author {
            login
            avatarUrl
          }
        }
      }
      reviewThreads(first: 100) {
        nodes {
          comments(first: 50) {
            nodes {
              id
              body
              path
              line
              createdAt
              updatedAt
              author {
                login
                avatarUrl
              }
            }
          }
        }
      }
    }
  }
}
`

// QueryListIssues busca issues de um repositório
const QueryListIssues = `
query ListIssues($owner: String!, $repo: String!, $first: Int!, $after: String, $states: [IssueState!], $labels: [String!]) {
  repository(owner: $owner, name: $repo) {
    issues(first: $first, after: $after, states: $states, labels: $labels, orderBy: {field: UPDATED_AT, direction: DESC}) {
      totalCount
      pageInfo {
        hasNextPage
        endCursor
      }
      nodes {
        id
        number
        title
        body
        state
        createdAt
        updatedAt
        author {
          login
          avatarUrl
        }
        assignees(first: 5) {
          nodes {
            login
            avatarUrl
          }
        }
        labels(first: 10) {
          nodes {
            name
            color
          }
        }
      }
    }
  }
}
`

// QueryListBranches busca branches de um repositório
const QueryListBranches = `
query ListBranches($owner: String!, $repo: String!, $first: Int!) {
  repository(owner: $owner, name: $repo) {
    refs(refPrefix: "refs/heads/", first: $first, orderBy: {field: TAG_COMMIT_DATE, direction: DESC}) {
      nodes {
        name
        prefix
        target {
          ... on Commit {
            oid
          }
        }
      }
    }
  }
}
`

// QueryListRepositories busca repositórios do usuário autenticado
const QueryListRepositories = `
query ListRepositories($first: Int!, $after: String) {
  viewer {
    repositories(first: $first, after: $after, orderBy: {field: UPDATED_AT, direction: DESC}, affiliations: [OWNER, COLLABORATOR, ORGANIZATION_MEMBER]) {
      totalCount
      pageInfo {
        hasNextPage
        endCursor
      }
      nodes {
        id
        name
        nameWithOwner
        owner {
          login
        }
        description
        isPrivate
        defaultBranchRef {
          name
        }
        updatedAt
      }
    }
  }
}
`

// === Mutations ===

// MutationCreatePR cria um novo Pull Request
const MutationCreatePR = `
mutation CreatePullRequest($input: CreatePullRequestInput!) {
  createPullRequest(input: $input) {
    pullRequest {
      id
      number
      title
      state
      url
      headRefName
      baseRefName
    }
  }
}
`

// MutationMergePR faz merge de um Pull Request
const MutationMergePR = `
mutation MergePullRequest($input: MergePullRequestInput!) {
  mergePullRequest(input: $input) {
    pullRequest {
      id
      number
      state
      merged
    }
  }
}
`

// MutationClosePR fecha um Pull Request
const MutationClosePR = `
mutation ClosePullRequest($input: ClosePullRequestInput!) {
  closePullRequest(input: $input) {
    pullRequest {
      id
      number
      state
    }
  }
}
`

// MutationCreateReview cria um review em um PR
const MutationCreateReview = `
mutation AddPullRequestReview($input: AddPullRequestReviewInput!) {
  addPullRequestReview(input: $input) {
    pullRequestReview {
      id
      state
      body
      createdAt
      author {
        login
        avatarUrl
      }
    }
  }
}
`

// MutationCreateComment cria um comentário em um PR
const MutationCreateComment = `
mutation AddComment($input: AddCommentInput!) {
  addComment(input: $input) {
    commentEdge {
      node {
        id
        body
        createdAt
        author {
          login
          avatarUrl
        }
      }
    }
  }
}
`

// MutationCreateIssue cria uma nova issue
const MutationCreateIssue = `
mutation CreateIssue($input: CreateIssueInput!) {
  createIssue(input: $input) {
    issue {
      id
      number
      title
      state
      createdAt
    }
  }
}
`

// MutationUpdateIssue atualiza uma issue
const MutationUpdateIssue = `
mutation UpdateIssue($input: UpdateIssueInput!) {
  updateIssue(input: $input) {
    issue {
      id
      number
      title
      state
      updatedAt
    }
  }
}
`

// MutationCreateBranch cria uma nova branch
const MutationCreateBranch = `
mutation CreateRef($input: CreateRefInput!) {
  createRef(input: $input) {
    ref {
      name
      prefix
      target {
        ... on Commit {
          oid
        }
      }
    }
  }
}
`
