package api

import (
	"testing"

	"knowledgebook/internal/model"
)

func TestExtractDraftID_ModelDraft(t *testing.T) {
	data := map[string]any{
		"draft": &model.Draft{ID: 42, Title: "test"},
	}
	got := extractDraftID(data)
	if got != 42 {
		t.Errorf("extractDraftID(*model.Draft) = %d, want 42", got)
	}
}

func TestExtractDraftID_MapFloat64(t *testing.T) {
	// JSON unmarshal always produces float64 for numbers
	data := map[string]any{
		"draft": map[string]any{"id": float64(99)},
	}
	got := extractDraftID(data)
	if got != 99 {
		t.Errorf("extractDraftID(map float64) = %d, want 99", got)
	}
}

func TestExtractDraftID_MapInt64(t *testing.T) {
	data := map[string]any{
		"draft": map[string]any{"id": int64(88)},
	}
	got := extractDraftID(data)
	if got != 88 {
		t.Errorf("extractDraftID(map int64) = %d, want 88", got)
	}
}

func TestExtractDraftID_NilData(t *testing.T) {
	got := extractDraftID(nil)
	if got != 0 {
		t.Errorf("extractDraftID(nil) = %d, want 0", got)
	}
}

func TestExtractDraftID_NoDraftKey(t *testing.T) {
	data := map[string]any{
		"other": "value",
	}
	got := extractDraftID(data)
	if got != 0 {
		t.Errorf("extractDraftID(no draft key) = %d, want 0", got)
	}
}

func TestExtractDraftID_DraftJSONString(t *testing.T) {
	// This is what was happening before the fix — Data["draft"] was a string, not a map
	data := map[string]any{
		"draftJSON": `{"id":42,"title":"test"}`,
	}
	got := extractDraftID(data)
	if got != 0 {
		t.Errorf("extractDraftID(draftJSON only) = %d, want 0 (draftJSON string is not 'draft' key)", got)
	}
}

func TestExtractDraftID_AgentResult_WithDraftMap(t *testing.T) {
	// After the fix: agent puts both draftJSON and draft map
	data := map[string]any{
		"draftJSON": `{"id":42,"title":"test"}`,
		"draft":     map[string]any{"id": float64(42), "title": "test"},
	}
	got := extractDraftID(data)
	if got != 42 {
		t.Errorf("extractDraftID(agent result with draft map) = %d, want 42", got)
	}
}
