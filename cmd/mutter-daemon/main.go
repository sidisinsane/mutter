// Command mutter-daemon is the backend binary for mutter.
// It provides the indexer, router, session management, and API server.
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
	cfg        *config.Config
	index      *indexer.Index
	session    *session.Session
	normalizer *normalizer.Normalizer
	embedder   embedder.Embedder
	matcher    *matcher.Matcher
	// scriptPaths stores indexed script paths for matching
	scriptPaths []string
}

func main() {
	// Load configuration
	workspaceRoot, err := os.Getwd()
	if err != nil {
		log.Fatalf("get working directory: %v", err)
	}

	cfg, err := config.Load(workspaceRoot)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// Create embedder
	emb, err := embedder.New(embedder.Config{
		ModelPath:           cfg.Model.Path,
		ExpectedDimensions: cfg.Model.Dimensions,
	})
	if err != nil {
		log.Printf("warning: failed to create embedder: %v (using keyword matching)", err)
	}

	// Create matcher
	mat := matcher.New(cfg.ConfidenceThreshold)

	// Create server
	srv := &Server{
		cfg:        cfg,
		session:    session.New(cfg.Session.BufferSize),
		normalizer: normalizer.New(),
		embedder:   emb,
		matcher:    mat,
	}

	// Build initial index (with embeddings if embedder is available)
	if err := srv.reindex(); err != nil {
		log.Printf("initial index: %v", err)
	}

	// Set up routes
	http.HandleFunc("/api/index", srv.handleIndex)
	http.HandleFunc("/api/route", srv.handleRoute)
	http.HandleFunc("/api/execute", srv.handleExecute)
	http.HandleFunc("/api/query", srv.handleQuery)

	// Start server
	addr := ":8080"
	log.Printf("mutter-daemon starting on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// reindex rebuilds the script index.
func (s *Server) reindex() error {
	log.Printf("building index with workspace root: %s", s.cfg.WorkspaceRoot)
	log.Printf("discovery paths: %v", s.cfg.Discovery.Paths)
	log.Printf("discovery recursive: %v", s.cfg.Discovery.Recursive)

	// Use embedder if available
	var idx *indexer.Index
	var err error

	if s.embedder != nil {
		log.Printf("building index with embeddings")
		idx, err = indexer.BuildWithEmbedder(s.cfg, s.embedder)
	} else {
		log.Printf("building index without embeddings (keyword matching only)")
		idx, err = indexer.Build(s.cfg)
	}

	if err != nil {
		return fmt.Errorf("build index: %w", err)
	}
	s.index = idx

	// Store script paths for matching
	s.scriptPaths = make([]string, 0, len(idx.Entries))
	for path := range idx.Entries {
		s.scriptPaths = append(s.scriptPaths, path)
		log.Printf("indexed script: %s", path)
	}

	log.Printf("indexed %d scripts total", len(idx.Entries))
	return nil
}

// handleIndex handles POST /api/index requests.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.reindex(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := map[string]any{
		"scripts_found": len(s.index.Entries),
		"errors":        []string{},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleRoute handles POST /api/route requests.
func (s *Server) handleRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// Normalize query for chains
	normalized, hasChain := s.normalizer.Normalize(req.Query)

	// Simple keyword matching for now
	matches := s.findMatches(req.Query)

	resp := map[string]any{
		"query_normalized": normalized,
		"has_chain":        hasChain,
		"matches":          matches,
	}

	// If it's a chain, also return the individual commands
	if hasChain {
		commands := strings.Split(normalized, "|")
		for i := range commands {
			commands[i] = strings.TrimSpace(commands[i])
		}
		resp["chain_commands"] = commands
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// findMatches performs matching on script descriptions using either semantic
// embeddings (if available) or keyword matching.
func (s *Server) findMatches(query string) []map[string]any {
	// Use semantic matching if embedder and matcher are available
	if s.embedder != nil && s.matcher != nil {
		return s.findMatchesSemantic(query)
	}

	// Fall back to keyword matching
	return s.findMatchesKeyword(query)
}

// findMatchesKeyword performs simple keyword-based matching on script descriptions.
func (s *Server) findMatchesKeyword(query string) []map[string]any {
	var matches []map[string]any
	queryLower := strings.ToLower(query)

	for _, path := range s.scriptPaths {
		entry, ok := s.index.Entries[path]
		if !ok || len(entry.Block.Commands) == 0 {
			continue
		}

		cmd := entry.Block.Commands[0]
		descLower := strings.ToLower(cmd.Description)

		// Check if query keywords appear in description
		if strings.Contains(descLower, queryLower) {
			matches = append(matches, map[string]any{
				"script_path": path,
				"script_name": entry.Block.Name,
				"description": cmd.Description,
				"confidence":  0.9,
				"usage":       cmd.Usage,
			})
		}
	}

	return matches
}

// findMatchesSemantic performs semantic matching using embeddings and cosine similarity.
func (s *Server) findMatchesSemantic(query string) []map[string]any {
	ctx := context.Background()

	// Generate embedding for the query
	queryEmbedding, err := s.embedder.Embed(ctx, query)
	if err != nil {
		log.Printf("failed to embed query: %v", err)
		return s.findMatchesKeyword(query) // Fall back to keyword matching
	}

	// Collect embeddings from indexed scripts
	var candidates [][]float32
	var scriptPaths []string

	for path, entry := range s.index.Entries {
		if entry.Embedding != nil {
			candidates = append(candidates, entry.Embedding)
			scriptPaths = append(scriptPaths, path)
		}
	}

	if len(candidates) == 0 {
		return s.findMatchesKeyword(query) // No embeddings available
	}

	// Find matches using the matcher
	matcherResults := s.matcher.FindAll(queryEmbedding, candidates)

	// Convert to response format
	var matches []map[string]any
	for _, result := range matcherResults {
		path := scriptPaths[result.Index]
		entry := s.index.Entries[path]
		if len(entry.Block.Commands) == 0 {
			continue
		}

		cmd := entry.Block.Commands[0]
		matches = append(matches, map[string]any{
			"script_path": path,
			"script_name": entry.Block.Name,
			"description": cmd.Description,
			"confidence":  result.Score,
			"usage":       cmd.Usage,
		})
	}

	return matches
}

// handleExecute handles POST /api/execute requests.
func (s *Server) handleExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ScriptPath string            `json:"script_path"`
		Arguments  map[string]string `json:"arguments"`
		Chain      []string          `json:"chain"` // For chained execution
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// Generate execution ID
	execID := fmt.Sprintf("exec-%d", os.Getpid())

	// Handle chained execution
	if len(req.Chain) > 0 {
		s.executeChain(w, execID, req.Chain)
		return
	}

	// Execute single script
	cmd := exec.Command("bash", req.ScriptPath)
	if len(req.Arguments) > 0 {
		// TODO: Render script with arguments
	}

	output, err := cmd.CombinedOutput()

	resp := map[string]any{
		"execution_id": execID,
		"exit_code":    cmd.ProcessState.ExitCode(),
		"stdout":       string(output),
		"stderr":       "",
		"error":        "",
	}

	if err != nil {
		resp["error"] = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// executeChain executes a chain of scripts in sequence.
func (s *Server) executeChain(w http.ResponseWriter, execID string, chain []string) {
	var results []map[string]any
	var finalOutput string

	for i, scriptPath := range chain {
		cmd := exec.Command("bash", scriptPath)

		// Pipe previous output if available
		if i > 0 && finalOutput != "" {
			cmd.Stdin = strings.NewReader(finalOutput)
		}

		output, err := cmd.CombinedOutput()
		exitCode := 0
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}

		result := map[string]any{
			"script_path": scriptPath,
			"exit_code":   exitCode,
			"stdout":      string(output),
		}

		if err != nil {
			result["error"] = err.Error()
		}

		results = append(results, result)
		finalOutput = string(output)

		// Stop chain on error
		if err != nil {
			break
		}
	}

	resp := map[string]any{
		"execution_id": execID,
		"chain":        results,
		"final_output": finalOutput,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleQuery handles POST /api/query requests.
// This is the main endpoint for natural language queries that routes and executes.
func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// Generate execution ID
	execID := fmt.Sprintf("exec-%d", os.Getpid())

	// Normalize query for chains
	normalized, hasChain := s.normalizer.Normalize(req.Query)

	var chain []string
	if hasChain {
		// Split by pipe to get individual commands
		parts := strings.Split(normalized, "|")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			// Find matching script for this part
			matches := s.findMatches(part)
			if len(matches) > 0 {
				chain = append(chain, matches[0]["script_path"].(string))
			}
		}
	} else {
		// Single command
		matches := s.findMatches(req.Query)
		if len(matches) > 0 {
			chain = append(chain, matches[0]["script_path"].(string))
		}
	}

	if len(chain) == 0 {
		http.Error(w, "no matching scripts found", http.StatusNotFound)
		return
	}

	// Execute the chain
	s.executeChain(w, execID, chain)
}
