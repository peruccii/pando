package github

import (
	"sync"
	"time"
)

// Cache implements an in-memory cache with TTL for GitHub data
type Cache struct {
	mu        sync.RWMutex
	prs       map[string][]PullRequest // key: "owner/repo"
	prDetail  map[string]*PullRequest  // key: "owner/repo/number"
	issues    map[string][]Issue       // key: "owner/repo"
	branches  map[string][]Branch      // key: "owner/repo"
	reviews   map[string][]Review      // key: "owner/repo/prNumber"
	comments  map[string][]Comment     // key: "owner/repo/prNumber"
	repos     []Repository
	updatedAt map[string]time.Time
	ttl       time.Duration
}

// NewCache cria um novo cache com TTL
func NewCache(ttl time.Duration) *Cache {
	return &Cache{
		prs:       make(map[string][]PullRequest),
		prDetail:  make(map[string]*PullRequest),
		issues:    make(map[string][]Issue),
		branches:  make(map[string][]Branch),
		reviews:   make(map[string][]Review),
		comments:  make(map[string][]Comment),
		updatedAt: make(map[string]time.Time),
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

// GetPRs retorna PRs cacheados para um repositório
func (c *Cache) GetPRs(owner, repo string) ([]PullRequest, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := owner + "/" + repo + "/prs"
	if c.isExpired(key) {
		return nil, false
	}
	prs, ok := c.prs[key]
	return prs, ok
}

// SetPRs armazena PRs no cache
func (c *Cache) SetPRs(owner, repo string, prs []PullRequest) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := owner + "/" + repo + "/prs"
	c.prs[key] = prs
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
	return pr, ok
}

// SetPR armazena um PR no cache
func (c *Cache) SetPR(owner, repo string, number int, pr *PullRequest) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := prDetailKey(owner, repo, number)
	c.prDetail[key] = pr
	c.updatedAt[key] = time.Now()
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
		}
	}
	for key := range c.prDetail {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.prDetail, key)
			delete(c.updatedAt, key)
		}
	}
	for key := range c.issues {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.issues, key)
			delete(c.updatedAt, key)
		}
	}
	for key := range c.branches {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.branches, key)
			delete(c.updatedAt, key)
		}
	}
	for key := range c.reviews {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.reviews, key)
			delete(c.updatedAt, key)
		}
	}
	for key := range c.comments {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.comments, key)
			delete(c.updatedAt, key)
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
