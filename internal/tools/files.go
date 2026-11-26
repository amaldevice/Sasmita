package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model"
)

// fileTool reads only from explicitly enabled project roots and attached files.
// Symlinks and .. segments are resolved before containment is checked.
type fileTool struct {
	roots     []string
	attached  []string
	readLimit int
}

func (fileTool) Spec() Spec {
	return Spec{
		Name:        "read_project_file",
		Description: "Read a text file from an attached file or the project folder.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
		Risk:        RiskReadOnly, Timeout: defaultTimeout, MaxOutputBytes: 4096,
	}
}

func (f fileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	roots, attached := f.roots, f.attached
	if scope, ok := ScopeFrom(ctx); ok {
		roots, attached = scope.Roots, scope.Attached
	}
	path, err := f.resolve(roots, attached, in.Path)
	if err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, int64(f.readLimit)))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (f fileTool) resolve(roots, attached []string, requested string) (string, error) {
	if strings.TrimSpace(requested) == "" {
		return "", fmt.Errorf("path is required")
	}
	allowed := append(append([]string(nil), roots...), attached...)
	if len(allowed) == 0 {
		return "", fmt.Errorf("no attached or project files are available")
	}
	var candidates []string
	if strings.HasPrefix(requested, "file://") {
		u, err := url.Parse(requested)
		if err != nil {
			return "", err
		}
		requested = u.Path
	}
	if filepath.IsAbs(requested) {
		candidates = []string{requested}
	} else {
		for _, root := range roots {
			candidates = append(candidates, filepath.Join(root, filepath.FromSlash(requested)))
		}
		for _, attached := range attached {
			if requested == attached || filepath.Base(attached) == filepath.Base(requested) {
				candidates = append(candidates, attached)
			}
		}
	}
	for _, candidate := range candidates {
		target, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			continue
		}
		for _, root := range allowed {
			base, err := filepath.EvalSymlinks(root)
			if err != nil {
				continue
			}
			rel, err := filepath.Rel(base, target)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			if info, err := os.Stat(target); err == nil && !info.IsDir() {
				return target, nil
			}
		}
	}
	return "", fmt.Errorf("path %q is outside the allowed files", requested)
}

// searchTool is bounded in depth, hits and bytes. It searches conversations
// via the repository and files via a bounded walk of the same allowed roots.
type searchTool struct {
	roots     []string
	search    ConversationSearcher
	readLimit int
}

func (searchTool) Spec() Spec {
	return Spec{
		Name:        "search",
		Description: "Search conversation history and project files, with a hard result cap.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer"}},"required":["query"]}`),
		Risk:        RiskReadOnly, Timeout: defaultTimeout, MaxOutputBytes: 4096,
	}
}

type fileHit struct {
	Path    string `json:"path"`
	Snippet string `json:"snippet"`
}

type searchOutput struct {
	Conversations []domain.SearchResult `json:"conversations"`
	Files         []fileHit             `json:"files"`
}

func (s searchTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	query := strings.TrimSpace(in.Query)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	limit := in.Limit
	if limit <= 0 || limit > 5 {
		limit = 5
	}
	out := searchOutput{Conversations: []domain.SearchResult{}, Files: []fileHit{}}
	if s.search != nil {
		found, err := s.search.SearchConversations(ctx, query, limit)
		if err == nil {
			out.Conversations = found
		}
	}
	needle := strings.ToLower(query)
	budget := s.readLimit
	if budget <= 0 {
		budget = 2048
	}
	roots := s.roots
	if scope, ok := ScopeFrom(ctx); ok {
		roots = scope.Roots
	}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || len(out.Files) >= limit || budget <= 0 || ctx.Err() != nil {
				return filepath.SkipAll
			}
			if entry.IsDir() {
				depth := strings.Count(strings.TrimPrefix(path, root), string(filepath.Separator))
				if depth >= 4 {
					return filepath.SkipDir
				}
				return nil
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			file, err := os.Open(path)
			if err != nil {
				return nil
			}
			data, err := io.ReadAll(io.LimitReader(file, int64(min(2048, budget))))
			_ = file.Close()
			if err != nil {
				return nil
			}
			budget -= len(data)
			lower := strings.ToLower(string(data))
			if at := strings.Index(lower, needle); at >= 0 {
				rel, _ := filepath.Rel(root, path)
				out.Files = append(out.Files, fileHit{Path: rel, Snippet: snippet(string(data), at, 160)})
			}
			return nil
		})
	}
	encoded, err := model.MarshalDeterministic(out)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func snippet(text string, at, span int) string {
	start := max(0, at-span/2)
	end := min(len(text), start+span)
	return strings.TrimSpace(text[start:end])
}

// artifactTool writes a sandboxed artifact and returns only its stable URI.
type artifactTool struct{ artifacts *ArtifactStore }

func (artifactTool) Spec() Spec {
	return Spec{
		Name:        "write_artifact",
		Description: "Store a text artifact and return its stable artifact:// reference.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"content":{"type":"string"}},"required":["content"]}`),
		Risk:        RiskWrite, Timeout: defaultTimeout, MaxOutputBytes: 512,
	}
}

func (a artifactTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if len(in.Content) > 1<<20 {
		return "", fmt.Errorf("artifact content exceeds 1 MiB")
	}
	art, err := a.artifacts.Write(in.Name, in.Content)
	if err != nil {
		return "", err
	}
	return art.Ref, nil
}
