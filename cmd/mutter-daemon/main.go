// Command mutter-daemon is the backend binary for mutter.
// It provides the indexer, router, session management, and HTTP/JSON API server.
//
// After running `go generate ./tools/...`, rebuild with -tags connectrpc to
// switch to the full ConnectRPC API with streaming support.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/sidisinsane/mutter/internal/config"
	"github.com/sidisinsane/mutter/internal/hashfm/indexer"
	"github.com/sidisinsane/mutter/internal/router/embedder"
	"github.com/sidisinsane/mutter/internal/router/matcher"
	"github.com/sidisinsane/mutter/internal/router/normalizer"
	"github.com/sidisinsane/mutter/internal/session"
)

// Server holds the daemon's state and dependencies.
type Server struct {
	cfg         *config.Config
	index       *indexer.Index
	session     *session.Session
	normalizer  *normalizer.Normalizer
	emb         embedder.Embedder
	matcher     *matcher.Matcher
	scriptPaths []string
}

func main() {
	workspaceRoot, err := os.Getwd()
	if err != nil {
		log.Fatalf("get working directory: %v", err)
	}

	cfg, err := config.Load(workspaceRoot)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	emb, err := embedder.New(embedder.Config{
		ModelPath:          cfg.Model.Path,
		LibraryPath:        cfg.Model.LibraryPath,
		ExpectedDimensions: cfg.Model.Dimensions,
	})
	if err != nil {
		log.Fatalf("failed to create embedder: %v", err)
	}

	srv := &Server{
		cfg:        cfg,
		session:    session.New(cfg.Session.BufferSize),
		normalizer: normalizer.New(),
		emb:        emb,
		matcher:    matcher.New(cfg.ConfidenceThreshold),
	}

	if err := srv.reindex(); err != nil {
		log.Printf("initial index: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/index", srv.handleIndex)
	mux.HandleFunc("/api/route", srv.handleRoute)
	mux.HandleFunc("/api/execute", srv.handleExecute)
	mux.HandleFunc("/api/query", srv.handleQuery)

	addr := ":8080"
	log.Printf("mutter-daemon starting on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// jsonError writes a JSON-encoded error response with the given status code.
func jsonError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]any{"error": message})
}

// reindex rebuilds the script index from the configured discovery paths.
func (s *Server) reindex() error {
	log.Printf("building index — workspace: %s paths: %v recursive: %v",
		s.cfg.WorkspaceRoot, s.cfg.Discovery.Paths, s.cfg.Discovery.Recursive)

	var (
		idx *indexer.Index
		err error
	)
	if s.emb != nil {
		log.Printf("building index with embeddings")
		idx, err = indexer.BuildWithEmbedder(s.cfg, s.emb)
	} else {
		log.Printf("building index without embeddings (keyword matching only)")
		idx, err = indexer.Build(s.cfg)
	}
	if err != nil {
		return fmt.Errorf("build index: %w", err)
	}

	s.index = idx
	s.scriptPaths = make([]string, 0, len(idx.Entries))
	for path := range idx.Entries {
		s.scriptPaths = append(s.scriptPaths, path)
		log.Printf("indexed: %s", path)
	}
	log.Printf("indexed %d scripts total", len(idx.Entries))
	return nil
}

// handleIndex handles POST /api/index — triggers a full re-index.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.reindex(); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"scripts_found": len(s.index.Entries),
		"errors":        []string{},
	})
}

// handleRoute handles POST /api/route — matches a query to indexed scripts.
func (s *Server) handleRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	normalized, hasChain := s.normalizer.Normalize(req.Query)
	matches := s.findMatches(req.Query)

	resp := map[string]any{
		"query_normalized": normalized,
		"has_chain":        hasChain,
		"matches":          toMatchMaps(matches),
	}
	if hasChain {
		parts := strings.Split(normalized, "|")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		resp["chain_commands"] = parts
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleExecute handles POST /api/execute — runs a script by path.
func (s *Server) handleExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ScriptPath string            `json:"script_path"`
		Arguments  map[string]string `json:"arguments"`
		Chain      []string          `json:"chain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	execID := s.session.Add("").ID

	if len(req.Chain) > 0 {
		s.executeChain(w, execID, req.Chain)
		return
	}

	result := s.executeScript(execID, req.ScriptPath)
	if err := s.session.MarkExecuted(execID); err != nil {
		log.Printf("mark executed: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleQuery handles POST /api/query — routes a natural language prompt and executes.
func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	execID := s.session.Add("").ID
	normalized, hasChain := s.normalizer.Normalize(req.Query)

	var chain []string
	if hasChain {
		for _, part := range strings.Split(normalized, "|") {
			part = strings.TrimSpace(part)
			if matches := s.findMatches(part); len(matches) > 0 {
				chain = append(chain, matches[0].scriptPath)
			}
		}
	} else {
		if matches := s.findMatches(req.Query); len(matches) > 0 {
			chain = append(chain, matches[0].scriptPath)
		}
	}

	if len(chain) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{
			"error":   "no matching scripts found",
			"query":   req.Query,
			"matches": []any{},
		})
		return
	}

	s.executeChain(w, execID, chain)
}

// executeScript runs a single script via the configured shell and returns
// a result map containing execution_id, exit_code, stdout, stderr, and error.
func (s *Server) executeScript(execID, scriptPath string) map[string]any {
	cmd := exec.Command(s.cfg.Shell(), scriptPath) //nolint:gosec
	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	runErr := cmd.Run()
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	errMsg := ""
	if runErr != nil && exitCode == 0 {
		errMsg = runErr.Error()
	}

	return map[string]any{
		"execution_id": execID,
		"exit_code":    exitCode,
		"stdout":       stdoutBuf.String(),
		"stderr":       stderrBuf.String(),
		"error":        errMsg,
	}
}

// executeChain runs a sequence of scripts, piping stdout between them.
func (s *Server) executeChain(w http.ResponseWriter, execID string, chain []string) {
	var results []map[string]any
	var prevStdout string

	for i, scriptPath := range chain {
		cmd := exec.Command(s.cfg.Shell(), scriptPath) //nolint:gosec
		if i > 0 && prevStdout != "" {
			cmd.Stdin = strings.NewReader(prevStdout)
		}
		var stdoutBuf, stderrBuf strings.Builder
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf

		runErr := cmd.Run()
		exitCode := 0
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}
		errMsg := ""
		if runErr != nil && exitCode == 0 {
			errMsg = runErr.Error()
		}

		result := map[string]any{
			"script_path": scriptPath,
			"exit_code":   exitCode,
			"stdout":      stdoutBuf.String(),
			"stderr":      stderrBuf.String(),
			"error":       errMsg,
		}
		results = append(results, result)
		prevStdout = stdoutBuf.String()

		if runErr != nil {
			break
		}
	}

	if err := s.session.MarkExecuted(execID); err != nil {
		log.Printf("mark executed: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"execution_id": execID,
		"chain":        results,
		"final_output": prevStdout,
	})
}

// match holds the result of a single script routing match.
type match struct {
	scriptPath  string
	scriptName  string
	description string
	confidence  float64
	usage       string
}

// toMatchMaps converts a slice of match to JSON-serialisable maps.
func toMatchMaps(matches []match) []map[string]any {
	out := make([]map[string]any, len(matches))
	for i, m := range matches {
		out[i] = map[string]any{
			"script_path": m.scriptPath,
			"script_name": m.scriptName,
			"description": m.description,
			"confidence":  m.confidence,
			"usage":       m.usage,
		}
	}
	return out
}

// findMatches routes a query using semantic matching if available, falling
// back to keyword matching otherwise.
func (s *Server) findMatches(query string) []match {
	if s.emb != nil && s.matcher != nil {
		return s.findMatchesSemantic(query)
	}
	return s.findMatchesKeyword(query)
}

// findMatchesKeyword performs substring matching against script descriptions.
func (s *Server) findMatchesKeyword(query string) []match {
	queryLower := strings.ToLower(query)
	var matches []match
	for _, path := range s.scriptPaths {
		entry, ok := s.index.Entries[path]
		if !ok || len(entry.Block.Commands) == 0 {
			continue
		}
		cmd := entry.Block.Commands[0]
		if strings.Contains(strings.ToLower(cmd.Description), queryLower) {
			matches = append(matches, match{
				scriptPath:  path,
				scriptName:  entry.Block.Name,
				description: cmd.Description,
				confidence:  0.9,
				usage:       cmd.Usage,
			})
		}
	}
	return matches
}

// findMatchesSemantic embeds the query and performs cosine similarity matching.
// Falls back to keyword matching on error or when no embeddings are available.
func (s *Server) findMatchesSemantic(query string) []match {
	ctx := context.Background()

	queryEmbedding, err := s.emb.Embed(ctx, query)
	if err != nil {
		log.Printf("embed query: %v — falling back to keyword matching", err)
		return s.findMatchesKeyword(query)
	}

	var candidates [][]float32
	var paths []string
	for path, entry := range s.index.Entries {
		if entry.Embedding != nil {
			candidates = append(candidates, entry.Embedding)
			paths = append(paths, path)
		}
	}
	if len(candidates) == 0 {
		return s.findMatchesKeyword(query)
	}

	results := s.matcher.FindAll(queryEmbedding, candidates)
	matches := make([]match, 0, len(results))
	for _, r := range results {
		path := paths[r.Index]
		entry := s.index.Entries[path]
		if len(entry.Block.Commands) == 0 {
			continue
		}
		cmd := entry.Block.Commands[0]
		matches = append(matches, match{
			scriptPath:  path,
			scriptName:  entry.Block.Name,
			description: cmd.Description,
			confidence:  r.Score,
			usage:       cmd.Usage,
		})
	}
	return matches
}
