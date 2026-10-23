package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations map[string]any `json:"annotations,omitempty"`
}

var (
	prefixRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	nameRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	colorRe  = regexp.MustCompile(`^[A-Za-z0-9#(),.% -]{1,48}$`)
)

func prop(typ, desc string) map[string]any {
	return map[string]any{"type": typ, "description": desc}
}

func intProp(desc string, min, max int) map[string]any {
	return map[string]any{"type": "integer", "description": desc, "minimum": min, "maximum": max}
}

func object(props map[string]any, required ...string) map[string]any {
	m := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		m["required"] = required
	}
	return m
}

func readOnly() map[string]any {
	return map[string]any{"readOnlyHint": true, "openWorldHint": true}
}

func ToolDefinitions() []Tool {
	return []Tool{
		{
			Name:        "search_icons",
			Description: "Search Iconify's 200+ icon sets by keyword. Returns identifiers such as mdi:home, with the collection, category, license and palette flag. Use get_icon to fetch the SVG.",
			InputSchema: object(map[string]any{
				"query":  prop("string", "What to look for, e.g. database, arrow-left, shopping cart."),
				"limit":  intProp("Maximum results, 1-256. Default 32.", 1, 256),
				"prefix": prop("string", "Optional icon set prefix to search only that set, e.g. lucide, mdi, tabler."),
			}, "query"),
			Annotations: readOnly(),
		},
		{
			Name:        "list_collections",
			Description: "List or filter Iconify icon sets: prefix, name, total icons, category, palette flag, license and sample icon names. Use it to discover which palettes exist before searching.",
			InputSchema: object(map[string]any{
				"query":    prop("string", "Optional case-insensitive filter on prefix, name or category."),
				"category": prop("string", "Optional exact category, e.g. Material, UI 24px, Programming."),
				"palette":  prop("boolean", "true keeps only colored/palette icon sets, false only monochrome sets."),
				"limit":    intProp("Maximum collections returned, 1-500. Default 50.", 1, 500),
				"offset":   intProp("Skip this many matches. Default 0.", 0, 100000),
			}),
			Annotations: readOnly(),
		},
		{
			Name:        "get_collection",
			Description: "Describe one Iconify icon set and list its icon names. Returns categories, total count and ready-to-use identifiers such as lucide:home.",
			InputSchema: object(map[string]any{
				"prefix": prop("string", "Icon set prefix, e.g. lucide, mdi, tabler, ph."),
				"limit":  intProp("Maximum icon names returned, 1-1000. Default 100.", 1, 1000),
				"offset": intProp("Skip this many icon names. Default 0.", 0, 100000),
			}, "prefix"),
			Annotations: readOnly(),
		},
		{
			Name:        "get_icon",
			Description: "Fetch one icon from Iconify as raw SVG or as an SVG data URI. icon is prefix:name, as returned by search_icons or get_collection.",
			InputSchema: object(map[string]any{
				"icon":   prop("string", "Icon identifier, e.g. mdi:home or lucide:settings-2."),
				"format": map[string]any{"type": "string", "enum": []string{"svg", "data-uri"}, "description": "svg (default) returns the markup; data-uri returns data:image/svg+xml;base64,..."},
				"color":  prop("string", "Optional color override, e.g. #f5b64a or currentColor."),
				"height": intProp("Optional height in pixels, 1-4096.", 1, 4096),
				"width":  intProp("Optional width in pixels, 1-4096.", 1, 4096),
			}, "icon"),
			Annotations: readOnly(),
		},
	}
}

type IconHit struct {
	Icon       string `json:"icon"`
	Prefix     string `json:"prefix"`
	Name       string `json:"name"`
	Collection string `json:"collection,omitempty"`
	Category   string `json:"category,omitempty"`
	Palette    bool   `json:"palette"`
	License    string `json:"license,omitempty"`
	Height     int    `json:"height,omitempty"`
}

type SearchOutput struct {
	Query string    `json:"query"`
	Total int       `json:"total"`
	Count int       `json:"count"`
	Icons []IconHit `json:"icons"`
}

type CollectionsOutput struct {
	Total       int          `json:"total"`
	Count       int          `json:"count"`
	Offset      int          `json:"offset"`
	Collections []Collection `json:"collections"`
}

type CollectionOutput struct {
	Prefix     string         `json:"prefix"`
	Title      string         `json:"title"`
	Total      int            `json:"total"`
	Count      int            `json:"count"`
	Offset     int            `json:"offset"`
	License    string         `json:"license,omitempty"`
	Palette    bool           `json:"palette"`
	Categories map[string]int `json:"categories,omitempty"`
	Icons      []string       `json:"icons"`
}

func (s *server) callTool(ctx context.Context, name string, raw json.RawMessage) (string, bool, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	switch name {
	case "search_icons":
		return s.searchIcons(ctx, raw)
	case "list_collections":
		return s.listCollections(ctx, raw)
	case "get_collection":
		return s.getCollection(ctx, raw)
	case "get_icon":
		return s.getIcon(ctx, raw)
	default:
		return "unknown tool: " + name, true, nil
	}
}

func (s *server) searchIcons(ctx context.Context, raw json.RawMessage) (string, bool, error) {
	var a struct {
		Query  string `json:"query"`
		Limit  int    `json:"limit"`
		Prefix string `json:"prefix"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return "invalid arguments: " + err.Error(), true, nil
	}
	if strings.TrimSpace(a.Query) == "" {
		return "query is required", true, nil
	}
	if a.Prefix != "" && !prefixRe.MatchString(a.Prefix) {
		return "invalid prefix: " + a.Prefix, true, nil
	}
	want := a.Limit
	if want <= 0 {
		want = 32
	}
	if want > 256 {
		want = 256
	}
	res, err := s.api.Search(ctx, a.Query, want, a.Prefix)
	if err != nil {
		return "search failed: " + err.Error(), true, nil
	}
	out := SearchOutput{Query: res.Query, Total: res.Total, Icons: make([]IconHit, 0, want)}
	for _, id := range res.Icons {
		if len(out.Icons) >= want {
			break
		}
		prefix, name, err := SplitIcon(id)
		if err != nil {
			continue
		}
		col := res.Collections[prefix]
		hit := IconHit{
			Icon:       id,
			Prefix:     prefix,
			Name:       name,
			Collection: col.Name,
			Category:   col.Category,
			Palette:    col.Palette,
			Height:     col.Height,
		}
		if col.License.SPDX != "" {
			hit.License = col.License.SPDX
		} else {
			hit.License = col.License.Title
		}
		out.Icons = append(out.Icons, hit)
	}
	out.Count = len(out.Icons)
	return mustJSON(out), false, nil
}

func (s *server) listCollections(ctx context.Context, raw json.RawMessage) (string, bool, error) {
	var a struct {
		Query    string `json:"query"`
		Category string `json:"category"`
		Palette  *bool  `json:"palette"`
		Limit    int    `json:"limit"`
		Offset   int    `json:"offset"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return "invalid arguments: " + err.Error(), true, nil
	}
	all, err := s.api.Collections(ctx)
	if err != nil {
		return "could not list collections: " + err.Error(), true, nil
	}
	query := strings.ToLower(strings.TrimSpace(a.Query))
	category := strings.ToLower(strings.TrimSpace(a.Category))
	list := make([]Collection, 0, len(all))
	for prefix, col := range all {
		if query != "" {
			hay := strings.ToLower(prefix + " " + col.Name + " " + col.Category)
			if !strings.Contains(hay, query) {
				continue
			}
		}
		if category != "" && strings.ToLower(col.Category) != category {
			continue
		}
		if a.Palette != nil && col.Palette != *a.Palette {
			continue
		}
		col.Prefix = prefix
		list = append(list, col)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Name != list[j].Name {
			return list[i].Name < list[j].Name
		}
		return list[i].Prefix < list[j].Prefix
	})
	total := len(list)
	limit := a.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	offset := a.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	out := CollectionsOutput{Total: total, Offset: offset, Collections: list[offset:end]}
	out.Count = len(out.Collections)
	return mustJSON(out), false, nil
}

func (s *server) getCollection(ctx context.Context, raw json.RawMessage) (string, bool, error) {
	var a struct {
		Prefix string `json:"prefix"`
		Limit  int    `json:"limit"`
		Offset int    `json:"offset"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return "invalid arguments: " + err.Error(), true, nil
	}
	if !prefixRe.MatchString(a.Prefix) {
		return "invalid prefix: " + a.Prefix, true, nil
	}
	info, err := s.api.Collection(ctx, a.Prefix)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Status == 404 {
			return fmt.Sprintf("unknown icon set %q; use list_collections to find a prefix", a.Prefix), true, nil
		}
		return "could not read the collection: " + err.Error(), true, nil
	}
	names := info.IconNames()
	sort.Strings(names)
	total := len(names)
	limit := a.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	offset := a.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	out := CollectionOutput{
		Prefix:     a.Prefix,
		Title:      info.Title,
		Total:      total,
		Offset:     offset,
		Categories: info.CategoryCounts(),
		Icons:      make([]string, 0, end-offset),
	}
	for _, n := range names[offset:end] {
		out.Icons = append(out.Icons, a.Prefix+":"+n)
	}
	out.Count = len(out.Icons)
	if all, err := s.api.Collections(ctx); err == nil {
		if col, ok := all[a.Prefix]; ok {
			if out.Title == "" {
				out.Title = col.Name
			}
			out.Palette = col.Palette
			if col.License.SPDX != "" {
				out.License = col.License.SPDX
			} else {
				out.License = col.License.Title
			}
		}
	}
	if out.Title == "" {
		out.Title = a.Prefix
	}
	return mustJSON(out), false, nil
}

func (s *server) getIcon(ctx context.Context, raw json.RawMessage) (string, bool, error) {
	var a struct {
		Icon   string `json:"icon"`
		Format string `json:"format"`
		Color  string `json:"color"`
		Height int    `json:"height"`
		Width  int    `json:"width"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return "invalid arguments: " + err.Error(), true, nil
	}
	prefix, name, err := SplitIcon(a.Icon)
	if err != nil {
		return err.Error(), true, nil
	}
	format := strings.ToLower(strings.TrimSpace(a.Format))
	if format == "" {
		format = "svg"
	}
	if format != "svg" && format != "data-uri" {
		return "format must be svg or data-uri", true, nil
	}
	if a.Color != "" && !colorRe.MatchString(a.Color) {
		return "invalid color: " + a.Color, true, nil
	}
	if a.Height < 0 || a.Height > 4096 || a.Width < 0 || a.Width > 4096 {
		return "height and width must be between 1 and 4096", true, nil
	}
	svg, err := s.api.SVG(ctx, prefix, name, SVGOptions{Color: a.Color, Height: a.Height, Width: a.Width})
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Status == 404 {
			return fmt.Sprintf("icon %q not found; use search_icons to find an existing identifier", a.Icon), true, nil
		}
		return "could not fetch the icon: " + err.Error(), true, nil
	}
	if format == "data-uri" {
		return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(svg), false, nil
	}
	return string(svg), false, nil
}

func SplitIcon(s string) (prefix, name string, err error) {
	s = strings.TrimSpace(s)
	prefix, name, ok := strings.Cut(s, ":")
	if !ok || prefix == "" || name == "" {
		return "", "", fmt.Errorf("invalid icon %q: expected prefix:name, e.g. mdi:home", s)
	}
	if strings.Contains(name, ":") {
		return "", "", fmt.Errorf("invalid icon %q: more than one colon", s)
	}
	if !prefixRe.MatchString(prefix) || !nameRe.MatchString(name) {
		return "", "", fmt.Errorf("invalid icon %q: prefix or name has characters Iconify does not use", s)
	}
	return prefix, name, nil
}

func mustJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(b)
}
