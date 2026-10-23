package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNormalizeSearchLimit(t *testing.T) {
	cases := map[int]int{
		-1: 32, 0: 32, 1: 32, 31: 32, 32: 32, 33: 64,
		64: 64, 65: 96, 100: 128, 256: 256, 999: 256,
	}
	for in, want := range cases {
		if got := NormalizeSearchLimit(in); got != want {
			t.Errorf("NormalizeSearchLimit(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestSplitIcon(t *testing.T) {
	p, n, err := SplitIcon("lucide:settings-2")
	if err != nil || p != "lucide" || n != "settings-2" {
		t.Fatalf("SplitIcon = %q, %q, %v", p, n, err)
	}
	for _, bad := range []string{"", "home", "mdi:", ":home", "mdi:home:extra", "Mdi:Home", "mdi:bad/name"} {
		if _, _, err := SplitIcon(bad); err == nil {
			t.Errorf("SplitIcon(%q) accepted an invalid icon", bad)
		}
	}
}

func fakeAPI(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("prefix") != "lucide" {
			t.Errorf("prefix not forwarded: %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("limit") != "32" {
			t.Errorf("limit not rounded: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{
			"icons": ["lucide:home", "lucide:house"],
			"total": 2, "limit": 32, "start": 0,
			"collections": {"lucide": {"name": "Lucide", "total": 1903, "category": "UI 24px",
				"palette": false, "license": {"title": "ISC", "spdx": "ISC"}}}
		}`)
	})
	mux.HandleFunc("/collections", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{
			"lucide": {"name": "Lucide", "total": 1903, "category": "UI 24px", "palette": false,
				"license": {"spdx": "ISC"}, "samples": ["home", "settings"]},
			"logos": {"name": "SVG Logos", "total": 1600, "category": "Logos", "palette": true,
				"license": {"spdx": "CC0-1.0"}, "samples": ["github"]}
		}`)
	})
	mux.HandleFunc("/collection", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{
			"prefix": "lucide", "total": 3, "title": "Lucide",
			"categories": {"Navigation": ["home", "house"]},
			"uncategorized": ["settings"], "aliases": {}
		}`)
	})
	mux.HandleFunc("/lucide/home.svg", func(w http.ResponseWriter, r *http.Request) {
		if c := r.URL.Query().Get("color"); c != "" && c != "#f5b64a" {
			t.Errorf("color not forwarded: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		io.WriteString(w, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="M3 12l9-9 9 9"/></svg>`)
	})
	mux.HandleFunc("/lucide/missing.svg", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"error":"Icon not found"}`)
	})
	return httptest.NewServer(mux)
}

func TestSearchAndSVG(t *testing.T) {
	ts := fakeAPI(t)
	defer ts.Close()
	api := NewIconify(ts.URL, 5*time.Second)
	res, err := api.Search(context.Background(), "home", 5, "lucide")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Icons) != 2 || res.Collections["lucide"].Name != "Lucide" {
		t.Fatalf("unexpected search: %+v", res)
	}
	svg, err := api.SVG(context.Background(), "lucide", "home", SVGOptions{Color: "#f5b64a"})
	if err != nil || !strings.Contains(string(svg), "<svg") {
		t.Fatalf("SVG = %q, %v", svg, err)
	}
}

func TestToolSearchRespectsRequestedLimit(t *testing.T) {
	ts := fakeAPI(t)
	defer ts.Close()
	s := &server{api: NewIconify(ts.URL, 5*time.Second), log: log.New(io.Discard, "", 0)}
	text, isErr, err := s.callTool(context.Background(), "search_icons", json.RawMessage(`{"query":"home","limit":1,"prefix":"lucide"}`))
	if err != nil || isErr {
		t.Fatalf("callTool: %q, %v, %v", text, isErr, err)
	}
	var out SearchOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatal(err)
	}
	if out.Count != 1 || out.Icons[0].Icon != "lucide:home" || out.Icons[0].License != "ISC" {
		t.Fatalf("unexpected output: %s", text)
	}
}

func TestToolGetIconNotFound(t *testing.T) {
	ts := fakeAPI(t)
	defer ts.Close()
	s := &server{api: NewIconify(ts.URL, 5*time.Second), log: log.New(io.Discard, "", 0)}
	text, isErr, _ := s.callTool(context.Background(), "get_icon", json.RawMessage(`{"icon":"lucide:missing"}`))
	if !isErr || !strings.Contains(text, "not found") {
		t.Fatalf("expected a tool error, got %q (isError=%v)", text, isErr)
	}
}

func TestToolListCollections(t *testing.T) {
	ts := fakeAPI(t)
	defer ts.Close()
	s := &server{api: NewIconify(ts.URL, 5*time.Second), log: log.New(io.Discard, "", 0)}
	text, isErr, _ := s.callTool(context.Background(), "list_collections", json.RawMessage(`{"palette":true,"limit":10}`))
	if isErr {
		t.Fatalf("list_collections failed: %s", text)
	}
	var out CollectionsOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatal(err)
	}
	if out.Count != 1 || out.Collections[0].Prefix != "logos" {
		t.Fatalf("unexpected collections: %s", text)
	}
}

func TestProtocolHandshakeAndCalls(t *testing.T) {
	ts := fakeAPI(t)
	defer ts.Close()
	s := &server{api: NewIconify(ts.URL, 5*time.Second), log: log.New(io.Discard, "", 0)}
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search_icons","arguments":{"query":"home","prefix":"lucide"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"get_icon","arguments":{"icon":"lucide:home"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"nope"}`,
	}, "\n") + "\n"
	var out strings.Builder
	if err := s.serve(strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("got %d responses, want 5:\n%s", len(lines), out.String())
	}
	var init map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &init); err != nil {
		t.Fatal(err)
	}
	result := init["result"].(map[string]any)
	if result["protocolVersion"] != "2025-06-18" {
		t.Fatalf("protocolVersion = %v", result["protocolVersion"])
	}
	var list struct {
		Result struct {
			Tools []Tool `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Result.Tools) != 4 {
		t.Fatalf("got %d tools, want 4", len(list.Result.Tools))
	}
	if !strings.Contains(lines[2], "lucide:home") || !strings.Contains(lines[3], "<svg") {
		t.Fatalf("tool calls did not return the expected content:\n%s\n%s", lines[2], lines[3])
	}
	if !strings.Contains(lines[4], `"code":-32601`) {
		t.Fatalf("unknown method did not return -32601: %s", lines[4])
	}
}
