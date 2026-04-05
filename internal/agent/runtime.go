package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Runtime keeps the prompts and schemas loaded from app/agent for runtime use.
type Runtime struct {
	BasePath string
	Prompts  map[string]string
	Schemas  map[string][]byte
}

// Load reads the runtime prompts and schemas from the configured agent directory.
func Load(basePath string) (*Runtime, error) {
	runtime := &Runtime{
		BasePath: basePath,
		Prompts:  map[string]string{},
		Schemas:  map[string][]byte{},
	}

	promptFiles := map[string]string{
		"intent":     filepath.Join(basePath, "prompts", "intent-parser.md"),
		"draft":      filepath.Join(basePath, "prompts", "knowledge-extractor.md"),
		"similarity": filepath.Join(basePath, "prompts", "similarity-judge.md"),
		"answer":     filepath.Join(basePath, "prompts", "answer-composer.md"),
	}
	for key, filePath := range promptFiles {
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("read prompt %s: %w", filePath, err)
		}
		runtime.Prompts[key] = string(content)
	}

	schemaFiles := map[string]string{
		"intent":     filepath.Join(basePath, "schemas", "intent.schema.json"),
		"draft":      filepath.Join(basePath, "schemas", "draft.schema.json"),
		"similarity": filepath.Join(basePath, "schemas", "similarity.schema.json"),
		"answer":     filepath.Join(basePath, "schemas", "answer.schema.json"),
	}
	for key, filePath := range schemaFiles {
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("read schema %s: %w", filePath, err)
		}
		runtime.Schemas[key] = content
	}

	return runtime, nil
}

// LoadFromCandidates tries multiple candidate directories and returns the first loadable runtime config.
func LoadFromCandidates(paths ...string) (*Runtime, error) {
	tried := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		tried = append(tried, path)
		runtime, err := Load(path)
		if err == nil {
			return runtime, nil
		}
	}
	if len(tried) == 0 {
		return nil, fmt.Errorf("no runtime agent path configured")
	}
	return nil, fmt.Errorf("load runtime agent failed from candidates: %s", strings.Join(tried, ", "))
}

func (r *Runtime) Prompt(name string) string {
	if r == nil {
		return ""
	}
	return r.Prompts[name]
}

func (r *Runtime) Schema(name string) []byte {
	if r == nil {
		return nil
	}
	return r.Schemas[name]
}
