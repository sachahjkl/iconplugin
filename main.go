package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"
)

const (
	serverName = "iconplugin"
	defaultAPI = "https://api.iconify.design"
)

// serverVersion is replaced at build time: -X main.serverVersion=1.2.3
var serverVersion = "1.0.0"

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type server struct {
	api *Iconify
	log *log.Logger
}

func main() {
	var (
		demoDir     = flag.String("demo", "", "write an icon gallery to this directory and exit")
		showVersion = flag.Bool("version", false, "print the version and exit")
		apiBase     = flag.String("api", "", "Iconify API base URL (default "+defaultAPI+")")
		timeout     = flag.Duration("timeout", 30*time.Second, "HTTP timeout")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("%s %s\n", serverName, serverVersion)
		return
	}
	base := *apiBase
	if base == "" {
		base = os.Getenv("ICONIFY_API")
	}
	if base == "" {
		base = defaultAPI
	}
	api := NewIconify(base, *timeout)

	if *demoDir != "" {
		if err := runDemo(context.Background(), api, *demoDir); err != nil {
			log.Fatalf("demo: %v", err)
		}
		return
	}

	s := &server{api: api, log: log.New(os.Stderr, "iconplugin: ", log.LstdFlags)}
	if err := s.serve(os.Stdin, os.Stdout); err != nil {
		s.log.Printf("stopped: %v", err)
		os.Exit(1)
	}
}

func (s *server) serve(in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)

	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			_ = enc.Encode(rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{Code: -32700, Message: "parse error: " + err.Error()}})
			continue
		}
		if len(req.ID) == 0 {
			// notifications (initialized, cancelled, ...) need no answer
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		res := s.handle(ctx, &req)
		cancel()
		if err := enc.Encode(res); err != nil {
			return err
		}
	}
	return sc.Err()
}

func (s *server) handle(ctx context.Context, req *rpcRequest) rpcResponse {
	res := rpcResponse{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		version := p.ProtocolVersion
		switch version {
		case "2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25":
		default:
			version = "2025-06-18"
		}
		res.Result = map[string]any{
			"protocolVersion": version,
			"capabilities": map[string]any{
				"tools": map[string]any{"listChanged": false},
			},
			"serverInfo": serverInfo{Name: serverName, Version: serverVersion},
			"instructions": "Iconify icons for UI work. search_icons finds identifiers such as lucide:home; " +
				"list_collections and get_collection browse the available sets and palettes; " +
				"get_icon returns the SVG or a data URI. Fetch only the icons you use.",
		}
	case "ping":
		res.Result = map[string]any{}
	case "tools/list":
		res.Result = map[string]any{"tools": ToolDefinitions()}
	case "resources/list":
		res.Result = map[string]any{"resources": []any{}}
	case "resources/templates/list":
		res.Result = map[string]any{"resourceTemplates": []any{}}
	case "prompts/list":
		res.Result = map[string]any{"prompts": []any{}}
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			res.Error = &rpcError{Code: -32602, Message: "invalid tools/call params: " + err.Error()}
			return res
		}
		text, isErr, err := s.callTool(ctx, p.Name, p.Arguments)
		if err != nil {
			text = err.Error()
			isErr = true
		}
		result := map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
		}
		if isErr {
			result["isError"] = true
		}
		res.Result = result
	default:
		res.Error = &rpcError{Code: -32601, Message: "method not found: " + req.Method}
	}
	return res
}
