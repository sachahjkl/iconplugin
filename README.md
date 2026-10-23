# IconPlugin

[![CI](https://github.com/sachahjkl/iconplugin/actions/workflows/ci.yml/badge.svg)](https://github.com/sachahjkl/iconplugin/actions/workflows/ci.yml)
[![Release](https://github.com/sachahjkl/iconplugin/actions/workflows/release.yml/badge.svg)](https://github.com/sachahjkl/iconplugin/actions/workflows/release.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A tiny MCP server for [Iconify](https://iconify.design) in Go: one static binary, no Node, no Python,
no API key. It wraps the public Iconify HTTP API and exposes four tools over MCP stdio:

| Tool | What it does |
| --- | --- |
| `search_icons` | Search 200+ icon sets by keyword. Returns `mdi:home`-style identifiers with set, category, license and palette flag. Optional `prefix` and `limit`. |
| `list_collections` | List or filter icon sets by name, category and palette. Reports totals, license and sample icons. |
| `get_collection` | Describe one set and list its icon names, grouped by category. |
| `get_icon` | Fetch one icon as raw SVG or as an SVG data URI, with optional color, height and width. |

## Install

Download the binary for your platform from
[Releases](https://github.com/sachahjkl/iconplugin/releases):

```
iconplugin-windows-amd64.exe
iconplugin-linux-amd64
iconplugin-linux-arm64
iconplugin-darwin-amd64
iconplugin-darwin-arm64
SHA256SUMS.txt
```

Or, with Go 1.22+:

```sh
go install github.com/sachahjkl/iconplugin@latest
```

The binary has no runtime dependency. Rename the Windows asset to `iconplugin.exe` if you like.

## OpenCode V2

Add the server under `mcp.servers` in `~/.config/opencode/opencode.jsonc`:

```jsonc
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "servers": {
      "iconplugin": {
        "type": "local",
        "command": ["C:\\Tools\\iconplugin.exe"]
      }
    }
  }
}
```

OpenCode watches the config and reloads the server without a restart. The tools appear as
`iconplugin_search_icons` and friends, or under `tools.iconplugin.*` in Code Mode. Check the
connection with:

```sh
opencode mcp list
```

## Examples

```json
{"name":"search_icons","arguments":{"query":"database","limit":5}}
{"name":"list_collections","arguments":{"query":"lucide","palette":true,"limit":10}}
{"name":"get_collection","arguments":{"prefix":"lucide","limit":5}}
{"name":"get_icon","arguments":{"icon":"lucide:database","color":"#f5b64a"}}
{"name":"get_icon","arguments":{"icon":"mdi:home","format":"data-uri"}}
```

`get_icon` returns the SVG as text, ready to write to a file or inline in a page. A missing icon is a
tool error (`isError: true`) with a pointer back to `search_icons`.

## Demo

Generate a gallery of 60+ icons taken from as many collections:

```sh
iconplugin -demo ./demo
```

It writes `demo/gallery.html` and `demo/icons.json`. The gallery is self-contained (icons are data
URIs), so it opens directly in a browser. A tiny dev server is included for previewing:

```sh
go build -o preview ./tools/preview
./preview -addr 127.0.0.1:8791 -dir demo
```

Then open <http://127.0.0.1:8791/gallery.html>.

The full MCP transcript (initialize, tools/list, the four tools, a 404) can be replayed with:

```sh
pwsh -File ./demo/client.ps1
```

## Build

```sh
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o iconplugin .
```

Tests are offline (`httptest`): protocol handshake, tool listing, every tool, a 404 and invalid
arguments.

```sh
go test ./...
```

## Releases

`.github/workflows/release.yml` runs on every `v*` tag:

1. `go vet` and `go test ./...`
2. cross-compile Windows amd64, Linux amd64/arm64 and macOS amd64/arm64 (`CGO_ENABLED=0`)
3. inject the tag as `main.serverVersion`
4. compute `SHA256SUMS.txt`
5. create the GitHub Release and upload every binary

Pushing a tag is the whole release:

```sh
git tag -a v1.2.3 -m "IconPlugin v1.2.3"
git push origin v1.2.3
```

`workflow_dispatch` builds the same artifacts without publishing a release.

## Design notes

- **API**: `GET /search`, `/collections`, `/collection` and `/{prefix}/{name}.svg` on
  `api.iconify.design`.
- **Cache**: the collection index (a few hundred KB) stays in memory for ten minutes.
- **Network**: 30 s HTTP timeout, 60 s per MCP call, two retries on 429/5xx, 16 MB body cap.
- **Validation**: prefixes `[a-z0-9-]`, names `[A-Za-z0-9._-]`, colors limited to 48 safe
  characters. No arbitrary URL is ever fetched.
- **Override**: `ICONIFY_API` or `-api` for a proxy or tests.
- **Version**: `-version`.

## Layout

```
main.go            JSON-RPC, initialize, tools/list, tools/call, stdio
iconify.go         HTTP client, cache, search, collections, SVG
tools.go           tool schemas and call handling
demo.go            gallery generator
main_test.go       offline tests (httptest)
tools/preview      small file server for the gallery
demo/client.ps1    MCP demo client
```

MIT licensed.
