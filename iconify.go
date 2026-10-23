package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Iconify is a small client for the public Iconify API (https://api.iconify.design).
// It keeps the collection index in memory: the index is a few hundred kilobytes and changes slowly.
type Iconify struct {
	base string
	http *http.Client

	mu        sync.Mutex
	colls     map[string]Collection
	collsAt   time.Time
	cacheLife time.Duration
}

func NewIconify(base string, timeout time.Duration) *Iconify {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Iconify{
		base:      strings.TrimRight(base, "/"),
		http:      &http.Client{Timeout: timeout},
		cacheLife: 10 * time.Minute,
	}
}

type Author struct {
	Name string `json:"name,omitempty"`
	URL  string `json:"url,omitempty"`
}

type License struct {
	Title string `json:"title,omitempty"`
	SPDX  string `json:"spdx,omitempty"`
	URL   string `json:"url,omitempty"`
}

type Collection struct {
	Prefix   string   `json:"prefix,omitempty"`
	Name     string   `json:"name,omitempty"`
	Total    int      `json:"total"`
	Author   Author   `json:"author,omitempty"`
	License  License  `json:"license,omitempty"`
	Samples  []string `json:"samples,omitempty"`
	Height   int      `json:"height,omitempty"`
	Category string   `json:"category,omitempty"`
	Palette  bool     `json:"palette,omitempty"`
	Version  string   `json:"version,omitempty"`
}

type SearchResult struct {
	Query       string                `json:"query"`
	Total       int                   `json:"total"`
	Limit       int                   `json:"limit"`
	Start       int                   `json:"start"`
	Icons       []string              `json:"icons"`
	Collections map[string]Collection `json:"collections"`
}

type CollectionInfo struct {
	Prefix        string              `json:"prefix"`
	Total         int                 `json:"total"`
	Title         string              `json:"title"`
	Categories    map[string][]string `json:"categories"`
	Uncategorized []string            `json:"uncategorized"`
	Hidden        []string            `json:"hidden"`
	Aliases       map[string]string   `json:"aliases"`
}

type SVGOptions struct {
	Color  string
	Height int
	Width  int
}

type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Iconify API returned %d: %s", e.Status, e.Message)
}

// Search calls /search and returns icon identifiers such as "mdi:home".
// The API only accepts limits that are multiples of 32; the caller's limit is rounded up
// and the extra results are left in place (tools slice them to the requested count).
func (c *Iconify) Search(ctx context.Context, query string, limit int, prefix string) (*SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is empty")
	}
	apiLimit := NormalizeSearchLimit(limit)
	vals := url.Values{}
	vals.Set("query", query)
	vals.Set("limit", strconv.Itoa(apiLimit))
	if prefix != "" {
		vals.Set("prefix", prefix)
	}
	var raw struct {
		Icons       []string              `json:"icons"`
		Total       int                   `json:"total"`
		Limit       int                   `json:"limit"`
		Start       int                   `json:"start"`
		Collections map[string]Collection `json:"collections"`
	}
	if err := c.getJSON(ctx, "/search", vals, &raw); err != nil {
		return nil, err
	}
	for p, col := range raw.Collections {
		col.Prefix = p
		raw.Collections[p] = col
	}
	return &SearchResult{
		Query:       query,
		Total:       raw.Total,
		Limit:       raw.Limit,
		Start:       raw.Start,
		Icons:       raw.Icons,
		Collections: raw.Collections,
	}, nil
}

func NormalizeSearchLimit(n int) int {
	if n <= 0 {
		return 32
	}
	if n > 256 {
		n = 256
	}
	return ((n + 31) / 32) * 32
}

// Collections returns the full index from /collections, cached for ten minutes.
func (c *Iconify) Collections(ctx context.Context) (map[string]Collection, error) {
	c.mu.Lock()
	if c.colls != nil && time.Since(c.collsAt) < c.cacheLife {
		out := c.colls
		c.mu.Unlock()
		return out, nil
	}
	c.mu.Unlock()

	var raw map[string]Collection
	if err := c.getJSON(ctx, "/collections", nil, &raw); err != nil {
		return nil, err
	}
	for p, col := range raw {
		col.Prefix = p
		raw[p] = col
	}
	c.mu.Lock()
	c.colls = raw
	c.collsAt = time.Now()
	c.mu.Unlock()
	return raw, nil
}

func (c *Iconify) Collection(ctx context.Context, prefix string) (*CollectionInfo, error) {
	vals := url.Values{}
	vals.Set("prefix", prefix)
	var out CollectionInfo
	if err := c.getJSON(ctx, "/collection", vals, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Iconify) SVG(ctx context.Context, prefix, name string, opt SVGOptions) ([]byte, error) {
	vals := url.Values{}
	if opt.Color != "" {
		vals.Set("color", opt.Color)
	}
	if opt.Height > 0 {
		vals.Set("height", strconv.Itoa(opt.Height))
	}
	if opt.Width > 0 {
		vals.Set("width", strconv.Itoa(opt.Width))
	}
	path := "/" + url.PathEscape(prefix) + "/" + url.PathEscape(name) + ".svg"
	body, _, err := c.get(ctx, path, vals)
	if err != nil {
		return nil, err
	}
	if !strings.Contains(string(body[:min(len(body), 512)]), "<svg") {
		return nil, fmt.Errorf("Iconify did not return an SVG for %s:%s", prefix, name)
	}
	return body, nil
}

// IconNames flattens a /collection answer into "name" entries, without duplicates.
func (info *CollectionInfo) IconNames() []string {
	seen := make(map[string]struct{}, info.Total)
	out := make([]string, 0, info.Total)
	add := func(list []string) {
		for _, n := range list {
			if n == "" {
				continue
			}
			if _, ok := seen[n]; ok {
				continue
			}
			seen[n] = struct{}{}
			out = append(out, n)
		}
	}
	for _, list := range info.Categories {
		add(list)
	}
	add(info.Uncategorized)
	return out
}

func (info *CollectionInfo) CategoryCounts() map[string]int {
	out := make(map[string]int, len(info.Categories))
	for name, list := range info.Categories {
		out[name] = len(list)
	}
	if len(info.Uncategorized) > 0 {
		out["Uncategorized"] = len(info.Uncategorized)
	}
	return out
}

func (c *Iconify) getJSON(ctx context.Context, path string, vals url.Values, out any) error {
	body, _, err := c.get(ctx, path, vals)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decoding %s: %w", path, err)
	}
	return nil
}

func (c *Iconify) get(ctx context.Context, path string, vals url.Values) ([]byte, string, error) {
	u := c.base + path
	if len(vals) > 0 {
		u += "?" + vals.Encode()
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(attempt) * 350 * time.Millisecond):
			case <-ctx.Done():
				return nil, "", ctx.Err()
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("User-Agent", "iconplugin/1.0 (OpenCode MCP; Iconify API)")
		req.Header.Set("Accept", "application/json, image/svg+xml, */*")
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode == http.StatusOK {
			return body, resp.Header.Get("Content-Type"), nil
		}
		lastErr = &APIError{Status: resp.StatusCode, Message: snippet(body)}
		if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
			return nil, "", lastErr
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("request failed")
	}
	return nil, "", lastErr
}

func snippet(body []byte) string {
	s := strings.Join(strings.Fields(string(body)), " ")
	if s == "" {
		return "(empty body)"
	}
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}
