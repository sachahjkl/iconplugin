package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type demoHit struct {
	Icon       string
	Prefix     string
	Name       string
	Collection string
	License    string
	Palette    bool
	Query      string
	SVG        []byte
	Err        error
}

var demoQueries = []string{
	"emoji", "logos", "home", "settings", "database", "terminal",
	"cloud", "heart", "play", "search", "github", "weather",
}

func runDemo(ctx context.Context, api *Iconify, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	var picks []demoHit
	seenPrefix := map[string]bool{}
	for _, query := range demoQueries {
		res, err := api.Search(ctx, query, 96, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "demo: search %q: %v\n", query, err)
			continue
		}
		used := map[string]bool{}
		chosen := 0
		for pass := 0; pass < 2 && chosen < 6; pass++ {
			for _, id := range res.Icons {
				if chosen >= 6 {
					break
				}
				prefix, name, err := SplitIcon(id)
				if err != nil || used[prefix] {
					continue
				}
				if pass == 0 && seenPrefix[prefix] {
					continue
				}
				col := res.Collections[prefix]
				license := col.License.SPDX
				if license == "" {
					license = col.License.Title
				}
				picks = append(picks, demoHit{
					Icon:       id,
					Prefix:     prefix,
					Name:       name,
					Collection: col.Name,
					License:    license,
					Palette:    col.Palette,
					Query:      query,
				})
				used[prefix] = true
				seenPrefix[prefix] = true
				chosen++
			}
		}
	}

	// fetch in parallel, bounded: the public API is happy with a handful of requests at a time
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < 6; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				p := &picks[i]
				opts := SVGOptions{}
				if !p.Palette {
					opts.Color = "#e8eaed"
				}
				svg, err := api.SVG(ctx, p.Prefix, p.Name, opts)
				p.SVG, p.Err = svg, err
			}
		}()
	}
	for i := range picks {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	collections := map[string]bool{}
	var totalBytes int
	for _, p := range picks {
		if p.Err == nil {
			collections[p.Prefix] = true
			totalBytes += len(p.SVG)
		}
	}

	htmlPath := filepath.Join(dir, "gallery.html")
	if err := os.WriteFile(htmlPath, []byte(renderGallery(picks, collections, totalBytes, time.Since(started))), 0o644); err != nil {
		return err
	}

	manifest := struct {
		Generated   string `json:"generated"`
		Count       int    `json:"count"`
		Collections int    `json:"collections"`
		Bytes       int    `json:"bytes"`
		Icons       []any  `json:"icons"`
	}{
		Generated:   time.Now().Format(time.RFC3339),
		Count:       len(picks),
		Collections: len(collections),
		Bytes:       totalBytes,
	}
	for _, p := range picks {
		entry := map[string]any{
			"icon":       p.Icon,
			"collection": p.Collection,
			"license":    p.License,
			"palette":    p.Palette,
			"bytes":      len(p.SVG),
		}
		if p.Err != nil {
			entry["error"] = p.Err.Error()
		}
		manifest.Icons = append(manifest.Icons, entry)
	}
	manifestPath := filepath.Join(dir, "icons.json")
	b, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(manifestPath, b, 0o644); err != nil {
		return err
	}

	fmt.Printf("demo: %s\n", htmlPath)
	fmt.Printf("demo: %d icons, %d collections, %d KB of SVG, %s\n", len(picks), len(collections), totalBytes/1024, time.Since(started).Round(time.Millisecond))
	return nil
}

func renderGallery(picks []demoHit, collections map[string]bool, totalBytes int, elapsed time.Duration) string {
	var b strings.Builder
	b.WriteString(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>IconPlugin — Iconify gallery</title>
<style>
  :root { color-scheme: dark; }
  * { box-sizing: border-box; }
  body { margin: 0; padding: 42px 36px 60px; background: #0c0e12; color: #e8eaed;
         font: 15px/1.45 system-ui, "Segoe UI", sans-serif; }
  header, main, footer { max-width: 1180px; margin: 0 auto; }
  .brand { font-size: 38px; font-weight: 800; letter-spacing: -.03em; }
  .brand span { color: #f5b64a; }
  .lead { color: #9aa3af; margin: 4px 0 18px; font-size: 16px; }
  .stats { display: flex; gap: 10px; flex-wrap: wrap; margin-bottom: 10px; }
  .chip { border: 1px solid #2a303a; background: #14171d; color: #aeb7c4; border-radius: 999px;
          padding: 4px 11px; font-size: 12px; }
  .chip b { color: #f5b64a; font-weight: 600; }
  section { margin-top: 34px; }
  h2 { font-size: 12px; letter-spacing: .14em; text-transform: uppercase; color: #77808d;
       margin: 0 0 12px; font-weight: 600; }
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(178px, 1fr)); gap: 12px; }
  .card { background: #14171d; border: 1px solid #252b34; border-radius: 13px; padding: 14px 12px 12px;
          display: flex; flex-direction: column; align-items: center; min-height: 150px;
          transition: border-color .12s, transform .12s, background .12s; }
  .card:hover { border-color: #f5b64a; background: #171b22; transform: translateY(-2px); }
  .glyph { height: 62px; display: flex; align-items: center; justify-content: center; margin-bottom: 8px; }
  .glyph img { width: 50px; height: 50px; image-rendering: auto; }
  .glyph .missing { width: 50px; height: 50px; border: 1px dashed #454d59; border-radius: 8px;
                    display: flex; align-items: center; justify-content: center; color: #6b7480; }
  code { font: 12px/1.35 ui-monospace, "Cascadia Mono", Consolas, monospace; color: #cfd6df;
         text-align: center; word-break: break-word; }
  .meta { margin-top: 5px; font-size: 11px; color: #6f7885; text-align: center; }
  .meta .pal { color: #8fc1ff; }
  footer { margin-top: 42px; color: #6b7480; font-size: 12px; border-top: 1px solid #20252d; padding-top: 16px; }
  footer code { color: #aeb7c4; }
</style>
</head>
<body>
<header>
  <div class="brand">Icon<span>Plugin</span></div>
  <p class="lead">MCP server for Iconify — search 200+ icon sets, fetch the SVG.</p>
  <div class="stats">
    <span class="chip"><b>`)
	fmt.Fprintf(&b, "%d", len(picks))
	b.WriteString(`</b> icons</span>
    <span class="chip"><b>`)
	fmt.Fprintf(&b, "%d", len(collections))
	b.WriteString(`</b> collections</span>
    <span class="chip"><b>`)
	fmt.Fprintf(&b, "%d", totalBytes/1024)
	b.WriteString(`</b> KB of SVG</span>
    <span class="chip"><b>`)
	fmt.Fprintf(&b, "%d", elapsed.Milliseconds())
	b.WriteString(`</b> ms</span>
    <span class="chip">search_icons</span>
    <span class="chip">list_collections</span>
    <span class="chip">get_collection</span>
    <span class="chip">get_icon</span>
  </div>
</header>
<main>
`)

	byQuery := map[string][]demoHit{}
	order := []string{}
	for _, p := range picks {
		if _, ok := byQuery[p.Query]; !ok {
			order = append(order, p.Query)
		}
		byQuery[p.Query] = append(byQuery[p.Query], p)
	}
	for _, query := range order {
		fmt.Fprintf(&b, "<section><h2>%s</h2><div class=\"grid\">\n", html.EscapeString(query))
		for _, p := range byQuery[query] {
			b.WriteString("<article class=\"card\">")
			if p.Err != nil || len(p.SVG) == 0 {
				b.WriteString("<div class=\"glyph\"><div class=\"missing\">?</div></div>")
			} else {
				uri := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(p.SVG)
				fmt.Fprintf(&b, "<div class=\"glyph\"><img src=\"%s\" alt=\"%s\"></div>", uri, html.EscapeString(p.Icon))
			}
			fmt.Fprintf(&b, "<code>%s</code>", html.EscapeString(p.Icon))
			fmt.Fprintf(&b, "<div class=\"meta\">%s", html.EscapeString(p.Collection))
			if p.License != "" {
				fmt.Fprintf(&b, " · %s", html.EscapeString(p.License))
			}
			if p.Palette {
				b.WriteString(" · <span class=\"pal\">palette</span>")
			}
			b.WriteString("</div></article>\n")
		}
		b.WriteString("</div></section>\n")
	}

	b.WriteString(`</main>
<footer>
  Generated by <code>iconplugin -demo</code> from <code>api.iconify.design</code>. No icon set is bundled:
  the server fetches only what you ask for, and caches the collection index for ten minutes.
</footer>
</body>
</html>
`)
	return b.String()
}
