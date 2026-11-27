package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"soapgo/internal/model"
)

// TestCloseRefusesWrites checks a closed store rejects a late write instead of
// recreating its (deleted) workspace file.
func TestCloseRefusesWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects.json")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.AddProject(model.Project{Name: "P"})
	if err != nil {
		t.Fatal(err)
	}
	call, err := s.AddProjectCall(p.ID, "", model.Call{Title: "c"})
	if err != nil {
		t.Fatal(err)
	}

	s.Close()

	// Simulate the workspace file being deleted, then a late response write.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	err = s.UpdateProjectCallResponse(p.ID, call.ID, &model.SavedResponse{Status: 200, Body: "late"})
	if !errors.Is(err, ErrStoreClosed) {
		t.Fatalf("write to closed store = %v, want ErrStoreClosed", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("closed store recreated the file: statErr=%v", statErr)
	}
}
