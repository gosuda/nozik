package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gg582/nozik/internal/db"
	"github.com/gg582/nozik/internal/model"
)

// Server is a tiny stdio MCP server.
type Server struct {
	DB *db.DB
}

// NewServer creates an MCP server backed by the taste DB.
func NewServer(database *db.DB) *Server { return &Server{DB: database} }

// Run reads JSON-RPC messages from stdin and writes responses to stdout.
func (s *Server) Run() error {
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		resp := s.handle(req)
		if resp != nil {
			b, _ := json.Marshal(resp)
			fmt.Println(string(b))
		}
	}
}

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *Server) handle(req jsonRPCRequest) *jsonRPCResponse {
	switch req.Method {
	case "initialize":
		return &jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo":      map[string]string{"name": "nozik-mcp", "version": "0.1.0"},
			"capabilities":    map[string]any{},
		}}
	case "tools/list":
		return &jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"tools": []map[string]any{
				{
					"name":        "nozik_db_search",
					"description": "Search Nozik's taste DB for tracks matching an emotion and discovery mode.",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"emotion": map[string]any{"type": "string", "description": "e.g. depressed, happy, excited"},
							"mode":    map[string]any{"type": "string", "enum": []string{"deep_diver", "pioneer"}},
							"limit":   map[string]any{"type": "integer", "default": 10},
						},
					},
				},
				{
					"name":        "nozik_gozik_installation",
					"description": "Find the Gozik binary and default gRPC address.",
					"inputSchema": map[string]any{
						"type":       "object",
						"properties": map[string]any{},
					},
				},
			},
		}}
	case "tools/call":
		return s.callTool(req.ID, req.Params)
	default:
		return nil
	}
}

func (s *Server) callTool(id json.RawMessage, params json.RawMessage) *jsonRPCResponse {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return errorResp(id, -32602, "invalid params")
	}
	switch p.Name {
	case "nozik_db_search":
		var args struct {
			Emotion string `json:"emotion"`
			Mode    string `json:"mode"`
			Limit   int    `json:"limit"`
		}
		_ = json.Unmarshal(p.Arguments, &args)
		if args.Limit <= 0 {
			args.Limit = 10
		}
		mode := model.ModeDeepDiver
		if args.Mode == string(model.ModePioneer) {
			mode = model.ModePioneer
		}
		recs, err := s.DB.Discover(model.Emotion(args.Emotion), mode, args.Limit)
		if err != nil {
			return errorResp(id, -32603, err.Error())
		}
		return &jsonRPCResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{"content": []map[string]any{{"type": "text", "text": toJSON(recs)}}}}
	case "nozik_gozik_installation":
		path, addr := detectGozik()
		return &jsonRPCResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{"content": []map[string]any{{"type": "text", "text": fmt.Sprintf(`{"binary":"%s","grpc_addr":"%s"}`, path, addr)}}}}
	default:
		return errorResp(id, -32601, "unknown tool")
	}
}

func errorResp(id json.RawMessage, code int, msg string) *jsonRPCResponse {
	return &jsonRPCResponse{JSONRPC: "2.0", ID: id, Error: &jsonRPCError{Code: code, Message: msg}}
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func detectGozik() (binary, addr string) {
	addr = os.Getenv("GOZIK_GRPC_ADDR")
	if addr == "" {
		addr = "127.0.0.1:50051"
	}
	candidates := []string{"gozik"}
	if runtime.GOOS == "windows" {
		candidates = append(candidates, "gozik.exe")
	}
	execDir, _ := os.Executable()
	if execDir != "" {
		candidates = append(candidates, filepath.Join(filepath.Dir(execDir), "gozik"))
	}
	// Also search next to nozik project root.
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "gozik", "gozik"))
	}
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil && !strings.Contains(p, "nozik") {
			return p, addr
		}
		if _, err := os.Stat(c); err == nil {
			return c, addr
		}
	}
	return "", addr
}
