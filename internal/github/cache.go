package github

import (
	"strings"
	"sync"
	"time"
)

// Cache implements an in-memory cache with TTL for GitHub data
type Cache struct {
	mu        sync.RWMutex
	prs       map[string][]PullRequest // key: "owner/repo/prs?state=x&page=y&per_page=z"
	prDetail  map[string]*PullRequest  // key: "owner/repo/number"
	prCommits map[string]PRCommitPage  // key: "owner/repo/number/commits?page=y&per_page=z"
	prFiles   map[string]PRFilePage    // key: "owner/repo/number/files?page=y&per_page=z"
	prRawDiff map[string]string        // key: "owner/repo/number/raw-diff"
	prMerged  map[string]bool          // key: "owner/repo/number/merged"
	issues    map[string][]Issue       // key: "owner/repo"
	branches  map[string][]Branch      // key: "owner/repo"
	reviews   map[string][]Review      // key: "owner/repo/prNumber"
	comments  map[string][]Comment     // key: "owner/repo/prNumber"
	repos     []Repository
	updatedAt map[string]time.Time
	etags     map[string]string
	ttl       time.Duration
}

// NewCache cria um novo cache com TTL
func NewCache(ttl time.Duration) *Cache {
	return &Cache{
		prs:       make(map[string][]PullRequest),
		prDetail:  make(map[string]*PullRequest),
		prCommits: make(map[string]PRCommitPage),
		prFiles:   make(map[string]PRFilePage),
		prRawDiff: make(map[string]string),
		prMerged:  make(map[string]bool),
		issues:    make(map[string][]Issue),
		branches:  make(map[string][]Branch),
		reviews:   make(map[string][]Review),
		comments:  make(map[string][]Comment),
		updatedAt: make(map[string]time.Time),
		etags:     make(map[string]string),
		ttl:       ttl,
	}
}

// isExpired verifica se uma entrada do cache expirou
func (c *Cache) isExpired(key string) bool {
	t, ok := c.updatedAt[key]
	if !ok {
		return true
	}
	return time.Since(t) > c.ttl
}

// === Pull Requests ===

// GetPRs retorna PRs cacheados por filtro/paginação.
func (c *Cache) GetPRs(owner, repo, state string, page, perPage int) ([]PullRequest, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := prListKey(owner, repo, state, page, perPage)
	if c.isExpired(key) {
		return nil, false
	}
	prs, ok := c.prs[key]
	if !ok {
		return nil, false
	}
	return clonePullRequests(prs), true
}

// GetPRsStale retorna PRs cacheados mesmo quando TTL expirou.
func (c *Cache) GetPRsStale(owner, repo, state string, page, perPage int) ([]PullRequest, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := prListKey(owner, repo, state, page, perPage)
	prs, ok := c.prs[key]
	if !ok {
		return nil, false
	}
	return clonePullRequests(prs), true
}

// SetPRs armazena PRs no cache por filtro/paginação.
func (c *Cache) SetPRs(owner, repo, state string, page, perPage int, prs []PullRequest) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := prListKey(owner, repo, state, page, perPage)
	c.prs[key] = clonePullRequests(prs)
	c.updatedAt[key] = time.Now()
}

// GetPR retorna um PR específico cacheado
func (c *Cache) GetPR(owner, repo string, number int) (*PullRequest, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := prDetailKey(owner, repo, number)
	if c.isExpired(key) {
		return nil, false
	}
	pr, ok := c.prDetail[key]
	if !ok || pr == nil {
		return nil, false
	}
	prCopy := clonePullRequest(*pr)
	return &prCopy, true
}

// GetPRStale retorna um PR específico mesmo quando TTL expirou.
func (c *Cache) GetPRStale(owner, repo string, number int) (*PullRequest, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := prDetailKey(owner, repo, number)
	pr, ok := c.prDetail[key]
	if !ok || pr == nil {
		return nil, false
	}
	prCopy := clonePullRequest(*pr)
	return &prCopy, true
}

// SetPR armazena um PR no cache
func (c *Cache) SetPR(owner, repo string, number int, pr *PullRequest) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := prDetailKey(owner, repo, number)
	if pr == nil {
		delete(c.prDetail, key)
		delete(c.updatedAt, key)
		delete(c.etags, key)
		return
	}
	prCopy := clonePullRequest(*pr)
	c.prDetail[key] = &prCopy
	c.updatedAt[key] = time.Now()
}

// GetPRCommitPage retorna commits paginados cacheados por PR.
func (c *Cache) GetPRCommitPage(owner, repo string, prNumber, page, perPage int) (*PRCommitPage, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := prCommitsKey(owner, repo, prNumber, page, perPage)
	if c.isExpired(key) {
		return nil, false
	}
	payload, ok := c.prCommits[key]
	if !ok {
		return nil, false
	}
	copyPayload := clonePRCommitPage(payload)
	return &copyPayload, true
}

// GetPRCommitPageStale retorna commits cacheados mesmo quando TTL expirou.
func (c *Cache) GetPRCommitPageStale(owner, repo string, prNumber, page, perPage int) (*PRCommitPage, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := prCommitsKey(owner, repo, prNumber, page, perPage)
	payload, ok := c.prCommits[key]
	if !ok {
		return nil, false
	}
	copyPayload := clonePRCommitPage(payload)
	return &copyPayload, true
}

// SetPRCommitPage armazena commits paginados no cache.
func (c *Cache) SetPRCommitPage(owner, repo string, prNumber int, page PRCommitPage) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := prCommitsKey(owner, repo, prNumber, page.Page, page.PerPage)
	c.prCommits[key] = clonePRCommitPage(page)
	c.updatedAt[key] = time.Now()
}

// GetPRFilePage retorna arquivos paginados cacheados por PR.
func (c *Cache) GetPRFilePage(owner, repo string, prNumber, page, perPage int) (*PRFilePage, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := prFilesKey(owner, repo, prNumber, page, perPage)
	if c.isExpired(key) {
		return nil, false
	}
	payload, ok := c.prFiles[key]
	if !ok {
		return nil, false
	}
	copyPayload := clonePRFilePage(payload)
	return &copyPayload, true
}

// GetPRFilePageStale retorna arquivos cacheados mesmo quando TTL expirou.
func (c *Cache) GetPRFilePageStale(owner, repo string, prNumber, page, perPage int) (*PRFilePage, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := prFilesKey(owner, repo, prNumber, page, perPage)
	payload, ok := c.prFiles[key]
	if !ok {
		return nil, false
	}
	copyPayload := clonePRFilePage(payload)
	return &copyPayload, true
}

// SetPRFilePage armazena arquivos paginados no cache.
func (c *Cache) SetPRFilePage(owner, repo string, prNumber int, page PRFilePage) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := prFilesKey(owner, repo, prNumber, page.Page, page.PerPage)
	c.prFiles[key] = clonePRFilePage(page)
	c.updatedAt[key] = time.Now()
}

// GetPRRawDiff retorna diff bruto cacheado por PR.
func (c *Cache) GetPRRawDiff(owner, repo string, prNumber int) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := prRawDiffKey(owner, repo, prNumber)
	if c.isExpired(key) {
		return "", false
	}
	value, ok := c.prRawDiff[key]
	return value, ok
}

// GetPRRawDiffStale retorna diff bruto cacheado mesmo quando TTL expirou.
func (c *Cache) GetPRRawDiffStale(owner, repo string, prNumber int) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := prRawDiffKey(owner, repo, prNumber)
	value, ok := c.prRawDiff[key]
	return value, ok
}

// SetPRRawDiff armazena diff bruto no cache.
func (c *Cache) SetPRRawDiff(owner, repo string, prNumber int, rawDiff string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := prRawDiffKey(owner, repo, prNumber)
	c.prRawDiff[key] = rawDiff
	c.updatedAt[key] = time.Now()
}

// GetPRMerged retorna status merged cacheado por PR.
func (c *Cache) GetPRMerged(owner, repo string, prNumber int) (bool, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := prMergeCheckKey(owner, repo, prNumber)
	if c.isExpired(key) {
		return false, false
	}
	value, ok := c.prMerged[key]
	return value, ok
}

// GetPRMergedStale retorna status merged cacheado mesmo quando TTL expirou.
func (c *Cache) GetPRMergedStale(owner, repo string, prNumber int) (bool, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := prMergeCheckKey(owner, repo, prNumber)
	value, ok := c.prMerged[key]
	return value, ok
}

// SetPRMerged armazena status merged no cache.
func (c *Cache) SetPRMerged(owner, repo string, prNumber int, merged bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := prMergeCheckKey(owner, repo, prNumber)
	c.prMerged[key] = merged
	c.updatedAt[key] = time.Now()
}

// GetETag retorna o ETag associado a uma chave de cache.
func (c *Cache) GetETag(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	etag, ok := c.etags[key]
	if !ok {
		return "", false
	}
	return etag, true
}

// SetETag define ou remove ETag para uma chave de cache.
func (c *Cache) SetETag(key, etag string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	normalizedKey := strings.TrimSpace(key)
	if normalizedKey == "" {
		return
	}

	normalizedETag := strings.TrimSpace(etag)
	if normalizedETag == "" {
		delete(c.etags, normalizedKey)
		return
	}
	c.etags[normalizedKey] = normalizedETag
}

// Touch marca uma chave de cache como atualizada agora.
func (c *Cache) Touch(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	normalizedKey := strings.TrimSpace(key)
	if normalizedKey == "" {
		return
	}
	c.updatedAt[normalizedKey] = time.Now()
}

// === Issues ===

// GetIssues retorna issues cacheadas
func (c *Cache) GetIssues(owner, repo string) ([]Issue, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := owner + "/" + repo + "/issues"
	if c.isExpired(key) {
		return nil, false
	}
	issues, ok := c.issues[key]
	return issues, ok
}

// SetIssues armazena issues no cache
func (c *Cache) SetIssues(owner, repo string, issues []Issue) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := owner + "/" + repo + "/issues"
	c.issues[key] = issues
	c.updatedAt[key] = time.Now()
}

// === Branches ===

// GetBranches retorna branches cacheadas
func (c *Cache) GetBranches(owner, repo string) ([]Branch, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := owner + "/" + repo + "/branches"
	if c.isExpired(key) {
		return nil, false
	}
	branches, ok := c.branches[key]
	return branches, ok
}

// SetBranches armazena branches no cache
func (c *Cache) SetBranches(owner, repo string, branches []Branch) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := owner + "/" + repo + "/branches"
	c.branches[key] = branches
	c.updatedAt[key] = time.Now()
}

// === Reviews ===

// GetReviews retorna reviews cacheados
func (c *Cache) GetReviews(owner, repo string, prNumber int) ([]Review, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := prDetailKey(owner, repo, prNumber) + "/reviews"
	if c.isExpired(key) {
		return nil, false
	}
	reviews, ok := c.reviews[key]
	return reviews, ok
}

// SetReviews armazena reviews no cache
func (c *Cache) SetReviews(owner, repo string, prNumber int, reviews []Review) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := prDetailKey(owner, repo, prNumber) + "/reviews"
	c.reviews[key] = reviews
	c.updatedAt[key] = time.Now()
}

// === Comments ===

// GetComments retorna comentários cacheados
func (c *Cache) GetComments(owner, repo string, prNumber int) ([]Comment, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := prDetailKey(owner, repo, prNumber) + "/comments"
	if c.isExpired(key) {
		return nil, false
	}
	comments, ok := c.comments[key]
	return comments, ok
}

// SetComments armazena comentários no cache
func (c *Cache) SetComments(owner, repo string, prNumber int, comments []Comment) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := prDetailKey(owner, repo, prNumber) + "/comments"
	c.comments[key] = comments
	c.updatedAt[key] = time.Now()
}

// === Repositories ===

// GetRepos retorna repositórios cacheados
func (c *Cache) GetRepos() ([]Repository, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.isExpired("repos") {
		return nil, false
	}
	if c.repos == nil {
		return nil, false
	}
	return c.repos, true
}

// SetRepos armazena repositórios no cache
func (c *Cache) SetRepos(repos []Repository) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.repos = repos
	c.updatedAt["repos"] = time.Now()
}

// === Cache Management ===

// InvalidatePRLists remove entradas de listagem de PRs de um repositorio.
func (c *Cache) InvalidatePRLists(owner, repo string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.invalidatePRListsLocked(owner, repo)
}

// InvalidatePRDetail remove entradas cacheadas de detalhe e sub-recursos de uma PR.
func (c *Cache) InvalidatePRDetail(owner, repo string, prNumber int) {
	if prNumber <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.invalidatePRDetailLocked(owner, repo, prNumber)
}

// InvalidatePRMutation remove caches de listagem e detalhe para mutacoes de PR.
func (c *Cache) InvalidatePRMutation(owner, repo string, prNumber int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.invalidatePRListsLocked(owner, repo)
	if prNumber > 0 {
		c.invalidatePRDetailLocked(owner, repo, prNumber)
	}
}

// Invalidate remove todas as entradas de um repositório do cache
func (c *Cache) Invalidate(owner, repo string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	prefix := owner + "/" + repo
	// Limpar todas as entradas que começam com este prefix
	for key := range c.prs {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.prs, key)
			delete(c.updatedAt, key)
			delete(c.etags, key)
		}
	}
	for key := range c.prDetail {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.prDetail, key)
			delete(c.updatedAt, key)
			delete(c.etags, key)
		}
	}
	for key := range c.prCommits {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.prCommits, key)
			delete(c.updatedAt, key)
			delete(c.etags, key)
		}
	}
	for key := range c.prFiles {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.prFiles, key)
			delete(c.updatedAt, key)
			delete(c.etags, key)
		}
	}
	for key := range c.prRawDiff {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.prRawDiff, key)
			delete(c.updatedAt, key)
			delete(c.etags, key)
		}
	}
	for key := range c.prMerged {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.prMerged, key)
			delete(c.updatedAt, key)
			delete(c.etags, key)
		}
	}
	for key := range c.issues {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.issues, key)
			delete(c.updatedAt, key)
			delete(c.etags, key)
		}
	}
	for key := range c.branches {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.branches, key)
			delete(c.updatedAt, key)
			delete(c.etags, key)
		}
	}
	for key := range c.reviews {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.reviews, key)
			delete(c.updatedAt, key)
			delete(c.etags, key)
		}
	}
	for key := range c.comments {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.comments, key)
			delete(c.updatedAt, key)
			delete(c.etags, key)
		}
	}
}

// GetUpdatedAt retorna quando um recurso foi atualizado pela última vez
func (c *Cache) GetUpdatedAt(key string) time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if t, ok := c.updatedAt[key]; ok {
		return t
	}
	return time.Time{}
}

// === Helpers ===

func prDetailKey(owner, repo string, number int) string {
	return owner + "/" + repo + "/" + intToStr(number)
}

func prListKey(owner, repo, state string, page, perPage int) string {
	normalizedState := strings.ToLower(strings.TrimSpace(state))
	if normalizedState == "" {
		normalizedState = "open"
	}
	return owner + "/" + repo + "/prs?state=" + normalizedState + "&page=" + intToStr(page) + "&per_page=" + intToStr(perPage)
}

func prCommitsKey(owner, repo string, prNumber, page, perPage int) string {
	return prDetailKey(owner, repo, prNumber) + "/commits?page=" + intToStr(page) + "&per_page=" + intToStr(perPage)
}

func prFilesKey(owner, repo string, prNumber, page, perPage int) string {
	return prDetailKey(owner, repo, prNumber) + "/files?page=" + intToStr(page) + "&per_page=" + intToStr(perPage)
}

func prRawDiffKey(owner, repo string, prNumber int) string {
	return prDetailKey(owner, repo, prNumber) + "/raw-diff"
}

func prMergeCheckKey(owner, repo string, prNumber int) string {
	return prDetailKey(owner, repo, prNumber) + "/merged"
}

func clonePullRequests(source []PullRequest) []PullRequest {
	if source == nil {
		return nil
	}
	result := make([]PullRequest, len(source))
	for index := range source {
		result[index] = clonePullRequest(source[index])
	}
	return result
}

func clonePullRequest(source PullRequest) PullRequest {
	result := source
	if source.Reviewers != nil {
		result.Reviewers = append([]User(nil), source.Reviewers...)
	}
	if source.Labels != nil {
		result.Labels = append([]Label(nil), source.Labels...)
	}
	if source.MergeCommit != nil {
		mergeCommit := strings.TrimSpace(*source.MergeCommit)
		result.MergeCommit = &mergeCommit
	}
	return result
}

func clonePRCommitPage(source PRCommitPage) PRCommitPage {
	result := source
	if source.Items == nil {
		return result
	}
	result.Items = make([]PRCommit, len(source.Items))
	for index := range source.Items {
		result.Items[index] = clonePRCommit(source.Items[index])
	}
	return result
}

func clonePRCommit(source PRCommit) PRCommit {
	result := source
	if source.ParentSHAs != nil {
		result.ParentSHAs = append([]string(nil), source.ParentSHAs...)
	}
	if source.Author != nil {
		author := *source.Author
		result.Author = &author
	}
	if source.Committer != nil {
		committer := *source.Committer
		result.Committer = &committer
	}
	return result
}

func clonePRFilePage(source PRFilePage) PRFilePage {
	result := source
	if source.Items == nil {
		return result
	}
	result.Items = append([]PRFile(nil), source.Items...)
	return result
}

func (c *Cache) deleteCacheMetadataLocked(key string) {
	delete(c.updatedAt, key)
	delete(c.etags, key)
}

func (c *Cache) invalidatePRListsLocked(owner, repo string) {
	prefix := owner + "/" + repo + "/prs?"
	for key := range c.prs {
		if strings.HasPrefix(key, prefix) {
			delete(c.prs, key)
			c.deleteCacheMetadataLocked(key)
		}
	}
}

func (c *Cache) invalidatePRDetailLocked(owner, repo string, prNumber int) {
	detailKey := prDetailKey(owner, repo, prNumber)
	delete(c.prDetail, detailKey)
	c.deleteCacheMetadataLocked(detailKey)

	rawDiffKey := prRawDiffKey(owner, repo, prNumber)
	delete(c.prRawDiff, rawDiffKey)
	c.deleteCacheMetadataLocked(rawDiffKey)

	mergedKey := prMergeCheckKey(owner, repo, prNumber)
	delete(c.prMerged, mergedKey)
	c.deleteCacheMetadataLocked(mergedKey)

	reviewsKey := detailKey + "/reviews"
	delete(c.reviews, reviewsKey)
	c.deleteCacheMetadataLocked(reviewsKey)

	commentsKey := detailKey + "/comments"
	delete(c.comments, commentsKey)
	c.deleteCacheMetadataLocked(commentsKey)

	commitsPrefix := detailKey + "/commits?"
	for key := range c.prCommits {
		if strings.HasPrefix(key, commitsPrefix) {
			delete(c.prCommits, key)
			c.deleteCacheMetadataLocked(key)
		}
	}

	filesPrefix := detailKey + "/files?"
	for key := range c.prFiles {
		if strings.HasPrefix(key, filesPrefix) {
			delete(c.prFiles, key)
			c.deleteCacheMetadataLocked(key)
		}
	}
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	if neg {
		result = "-" + result
	}
	return result
}
