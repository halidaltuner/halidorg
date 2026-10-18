// Package github syncs the site owner's public GitHub data every 6 hours
// (Phase 4.4, CONTENT_MODEL.md §4): repos with their languages, the 52-week
// contribution calendar, and notable activity (releases, new repos). The
// cron Worker enqueues the github.sync job; the container fetches GitHub's
// GraphQL API directly (ARCHITECTURE.md, outbound fetches) with the token
// the Worker passes as GITHUB_TOKEN.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Endpoint is GitHub's GraphQL API.
const Endpoint = "https://api.github.com/graphql"

// Client asks GitHub for the token owner's public data.
type Client struct {
	HTTP     *http.Client
	Endpoint string
	Token    string
}

// NewClient returns a client with a 20s timeout.
func NewClient(token string) *Client {
	return &Client{HTTP: &http.Client{Timeout: 20 * time.Second}, Endpoint: Endpoint, Token: token}
}

// ErrNoToken means GITHUB_TOKEN is not set: the sync has nothing to do.
var ErrNoToken = errors.New("github: GITHUB_TOKEN is not set")

// RateLimitError is GitHub refusing for now; Reset is when it lifts.
type RateLimitError struct{ Reset time.Time }

func (e RateLimitError) Error() string {
	return "github: rate limited until " + e.Reset.UTC().Format(time.RFC3339)
}

// Snapshot is what one sync read. Partial holds the errors GitHub returned
// alongside data (a section that failed); the rest is still written.
type Snapshot struct {
	Login    string
	Repos    []Repo
	Pinned   []string // repo ids, in profile order
	Weeks    []Week   // nil when the calendar failed
	Total    int
	Partial  []string
	Complete bool // every repo page was read
}

// Repo is one public repository.
type Repo struct {
	ID          string
	FullName    string
	Name        string
	Description string
	URL         string
	Stars       int
	Forks       int
	Language    string
	Topics      []string
	PushedAt    *time.Time
	CreatedAt   *time.Time
	Archived    bool
	Fork        bool
	Languages   map[string]int64
	Releases    []Release
}

// Release is one published release.
type Release struct {
	ID, Tag, Name, URL, Note string
	PublishedAt              time.Time
}

// Week is one week of the contribution calendar.
type Week struct {
	Start         time.Time
	Contributions int
}

const query = `query($after: String) {
  viewer {
    login
    pinnedItems(first: 6, types: REPOSITORY) { nodes { ... on Repository { id } } }
    contributionsCollection { contributionCalendar { totalContributions weeks { firstDay contributionDays { contributionCount } } } }
    repositories(first: 100, after: $after, privacy: PUBLIC, ownerAffiliations: OWNER, orderBy: {field: PUSHED_AT, direction: DESC}) {
      pageInfo { hasNextPage endCursor }
      nodes {
        id nameWithOwner name description url stargazerCount forkCount isArchived isFork pushedAt createdAt
        primaryLanguage { name }
        repositoryTopics(first: 10) { nodes { topic { name } } }
        languages(first: 10, orderBy: {field: SIZE, direction: DESC}) { edges { size node { name } } }
        releases(first: 5, orderBy: {field: CREATED_AT, direction: DESC}) { nodes { id tagName name url publishedAt description } }
      }
    }
  }
}`

type response struct {
	Data struct {
		Viewer *struct {
			Login       string `json:"login"`
			PinnedItems struct {
				Nodes []struct {
					ID string `json:"id"`
				} `json:"nodes"`
			} `json:"pinnedItems"`
			Contributions *struct {
				Calendar struct {
					Total int `json:"totalContributions"`
					Weeks []struct {
						FirstDay string `json:"firstDay"`
						Days     []struct {
							Count int `json:"contributionCount"`
						} `json:"contributionDays"`
					} `json:"weeks"`
				} `json:"contributionCalendar"`
			} `json:"contributionsCollection"`
			Repositories *struct {
				PageInfo struct {
					HasNext bool   `json:"hasNextPage"`
					End     string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []repoNode `json:"nodes"`
			} `json:"repositories"`
		} `json:"viewer"`
	} `json:"data"`
	Errors []struct {
		Type    string   `json:"type"`
		Message string   `json:"message"`
		Path    []string `json:"path"`
	} `json:"errors"`
}

type repoNode struct {
	ID          string     `json:"id"`
	FullName    string     `json:"nameWithOwner"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	URL         string     `json:"url"`
	Stars       int        `json:"stargazerCount"`
	Forks       int        `json:"forkCount"`
	Archived    bool       `json:"isArchived"`
	Fork        bool       `json:"isFork"`
	PushedAt    *time.Time `json:"pushedAt"`
	CreatedAt   *time.Time `json:"createdAt"`
	Primary     *struct {
		Name string `json:"name"`
	} `json:"primaryLanguage"`
	Topics struct {
		Nodes []struct {
			Topic struct {
				Name string `json:"name"`
			} `json:"topic"`
		} `json:"nodes"`
	} `json:"repositoryTopics"`
	Languages struct {
		Edges []struct {
			Size int64 `json:"size"`
			Node struct {
				Name string `json:"name"`
			} `json:"node"`
		} `json:"edges"`
	} `json:"languages"`
	Releases struct {
		Nodes []struct {
			ID          string     `json:"id"`
			Tag         string     `json:"tagName"`
			Name        *string    `json:"name"`
			URL         string     `json:"url"`
			PublishedAt *time.Time `json:"publishedAt"`
			Description *string    `json:"description"`
		} `json:"nodes"`
	} `json:"releases"`
}

// maxPages bounds the repo pages read (100 repos each).
const maxPages = 5

// Fetch reads the snapshot, paging through the repos.
func (c *Client) Fetch(ctx context.Context) (Snapshot, error) {
	if c.Token == "" {
		return Snapshot{}, ErrNoToken
	}
	var s Snapshot
	after := ""
	for page := 0; page < maxPages; page++ {
		r, err := c.do(ctx, after)
		if err != nil {
			return s, err
		}
		v := r.Data.Viewer
		if v == nil {
			return s, fmt.Errorf("github: no data: %v", r.Errors)
		}
		for _, e := range r.Errors {
			if e.Type == "RATE_LIMITED" {
				return s, RateLimitError{Reset: time.Now().Add(time.Hour)}
			}
			s.Partial = append(s.Partial, e.Message)
		}
		if page == 0 {
			s.Login = v.Login
			for _, n := range v.PinnedItems.Nodes {
				if n.ID != "" {
					s.Pinned = append(s.Pinned, n.ID)
				}
			}
			if v.Contributions != nil {
				s.Total = v.Contributions.Calendar.Total
				s.Weeks = []Week{}
				for _, w := range v.Contributions.Calendar.Weeks {
					start, err := time.Parse("2006-01-02", w.FirstDay)
					if err != nil {
						continue
					}
					n := 0
					for _, d := range w.Days {
						n += d.Count
					}
					s.Weeks = append(s.Weeks, Week{Start: start, Contributions: n})
				}
			}
		}
		if v.Repositories == nil {
			return s, nil // the repos failed: Partial says why
		}
		for _, n := range v.Repositories.Nodes {
			s.Repos = append(s.Repos, n.repo())
		}
		if !v.Repositories.PageInfo.HasNext {
			s.Complete = true
			return s, nil
		}
		after = v.Repositories.PageInfo.End
	}
	return s, nil
}

func (n repoNode) repo() Repo {
	r := Repo{ID: n.ID, FullName: n.FullName, Name: n.Name, URL: n.URL, Stars: n.Stars, Forks: n.Forks, Archived: n.Archived,
		Fork: n.Fork, PushedAt: n.PushedAt, CreatedAt: n.CreatedAt, Languages: map[string]int64{}}
	if n.Description != nil {
		r.Description = *n.Description
	}
	if n.Primary != nil {
		r.Language = n.Primary.Name
	}
	for _, t := range n.Topics.Nodes {
		r.Topics = append(r.Topics, t.Topic.Name)
	}
	for _, e := range n.Languages.Edges {
		r.Languages[e.Node.Name] = e.Size
	}
	for _, x := range n.Releases.Nodes {
		if x.PublishedAt == nil {
			continue // drafts
		}
		rel := Release{ID: x.ID, Tag: x.Tag, URL: x.URL, PublishedAt: *x.PublishedAt}
		if x.Name != nil {
			rel.Name = *x.Name
		}
		if x.Description != nil {
			rel.Note = firstLine(*x.Description)
		}
		r.Releases = append(r.Releases, rel)
	}
	return r
}

func firstLine(s string) string {
	if i := bytes.IndexByte([]byte(s), '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func (c *Client) do(ctx context.Context, after string) (*response, error) {
	vars := map[string]any{}
	if after != "" {
		vars["after"] = after
	}
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "halid.org-sync")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusTooManyRequests || (res.StatusCode == http.StatusForbidden && res.Header.Get("X-RateLimit-Remaining") == "0") {
		return nil, RateLimitError{Reset: resetOf(res.Header)}
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github: %s: %.200s", res.Status, raw)
	}
	var r response
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("github: decode: %w", err)
	}
	return &r, nil
}

// resetOf reads when a rate limit lifts: X-RateLimit-Reset (unix) or
// Retry-After (seconds), else an hour from now.
func resetOf(h http.Header) time.Time {
	if v, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		return time.Unix(v, 0)
	}
	if v, err := strconv.Atoi(h.Get("Retry-After")); err == nil {
		return time.Now().Add(time.Duration(v) * time.Second)
	}
	return time.Now().Add(time.Hour)
}
