package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"soapgo/internal/model"
)

// TestResponseBodyStoredInSeparateFile checks a saved body lives in its own
// file, stays out of projects.json, and is loaded back on reload.
func TestResponseBodyStoredInSeparateFile(t *testing.T) {
	s, path := newTestStore(t)
	p, err := s.AddProject(model.Project{Name: "p"})
	if err != nil {
		t.Fatal(err)
	}
	call, err := s.AddProjectCall(p.ID, "", model.Call{Title: "c"})
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Repeat("x", 4096)
	if err := s.UpdateProjectCallResponse(p.ID, call.ID, &model.SavedResponse{Status: 200, Body: body}); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), body) {
		t.Fatal("projects.json still contains the response body")
	}
	file, err := os.ReadFile(s.responseFilePath(p.ID, call.ID))
	if err != nil {
		t.Fatalf("response body file missing: %v", err)
	}
	if string(file) != body {
		t.Fatalf("body file length = %d, want %d", len(file), len(body))
	}

	s2, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s2.Get(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Calls[0].Response == nil || got.Calls[0].Response.Body != body {
		t.Fatalf("reloaded response = %+v", got.Calls[0].Response)
	}
}

// TestLegacyInlineResponseMigratesToFile checks a workspace written before the
// split still loads its body, and the body is moved out of the JSON on saving.
func TestLegacyInlineResponseMigratesToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects.json")
	legacy := `[{"id":"p1","name":"p","calls":[{"id":"c1","title":"c","response":{"status":200,"body":"<legacy/>"}}]}]`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("p1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Calls[0].Response == nil || got.Calls[0].Response.Body != "<legacy/>" {
		t.Fatalf("legacy body not loaded: %+v", got.Calls[0].Response)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "<legacy/>") {
		t.Fatal("legacy body was not migrated out of projects.json")
	}
	if _, err := os.Stat(s.responseFilePath("p1", "c1")); err != nil {
		t.Fatalf("legacy body file missing: %v", err)
	}
}
