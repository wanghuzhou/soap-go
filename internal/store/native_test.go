package store

import (
	"path/filepath"
	"testing"

	"soapgo/internal/model"
)

func newNativeStore(t *testing.T) (*Store, model.Project) {
	t.Helper()
	s, err := New(filepath.Join(t.TempDir(), "projects.json"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p, err := s.AddProject(model.Project{Name: "native", Source: "native"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	return s, p
}

// TestGroupCRUD covers creating nested folders, renaming, and deleting a folder
// together with everything inside it.
func TestGroupCRUD(t *testing.T) {
	s, p := newNativeStore(t)

	root, err := s.AddGroup(p.ID, "根目录", "")
	if err != nil {
		t.Fatalf("AddGroup root: %v", err)
	}
	child, err := s.AddGroup(p.ID, "子目录", root.ID)
	if err != nil {
		t.Fatalf("AddGroup child: %v", err)
	}
	if err := s.RenameGroup(p.ID, root.ID, "改名"); err != nil {
		t.Fatalf("RenameGroup: %v", err)
	}
	got, _ := s.Get(p.ID)
	if len(got.Groups) != 2 || got.Groups[0].Name != "改名" {
		t.Fatalf("groups = %+v", got.Groups)
	}

	c1, _ := s.AddProjectCall(p.ID, root.ID, model.Call{Title: "c1", Kind: model.KindHTTP})
	c2, _ := s.AddProjectCall(p.ID, child.ID, model.Call{Title: "c2", Kind: model.KindHTTP})

	removed, err := s.DeleteGroup(p.ID, root.ID)
	if err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
	if len(removed) != 2 {
		t.Fatalf("removed = %v, want the two nested calls", removed)
	}
	seen := map[string]bool{removed[0]: true, removed[1]: true}
	if !seen[c1.ID] || !seen[c2.ID] {
		t.Fatalf("removed ids = %v, want %v and %v", removed, c1.ID, c2.ID)
	}
	got, _ = s.Get(p.ID)
	if len(got.Groups) != 0 || len(got.Calls) != 0 {
		t.Fatalf("after delete: groups=%+v calls=%+v", got.Groups, got.Calls)
	}
}

// TestProjectCallCRUD covers the plain HTTP request lifecycle under a project.
func TestProjectCallCRUD(t *testing.T) {
	s, p := newNativeStore(t)

	call, err := s.AddProjectCall(p.ID, "", model.Call{Title: "新", Method: "GET"})
	if err != nil {
		t.Fatalf("AddProjectCall: %v", err)
	}
	if call.Kind != model.KindHTTP {
		t.Fatalf("kind = %q, want http", call.Kind)
	}
	call.Request = `{"a":1}`
	call.ContentType = "application/json"
	if err := s.UpdateProjectCall(p.ID, call); err != nil {
		t.Fatalf("UpdateProjectCall: %v", err)
	}
	got, _ := s.Get(p.ID)
	if len(got.Calls) != 1 || got.Calls[0].Request != `{"a":1}` || got.Calls[0].Method != "GET" {
		t.Fatalf("calls = %+v", got.Calls)
	}

	resp := &model.SavedResponse{Status: 200, Body: "ok"}
	if err := s.UpdateProjectCallResponse(p.ID, call.ID, resp); err != nil {
		t.Fatalf("UpdateProjectCallResponse: %v", err)
	}
	// A request edit must not erase the saved response.
	call.Request = `{"a":2}`
	if err := s.UpdateProjectCall(p.ID, call); err != nil {
		t.Fatalf("UpdateProjectCall 2: %v", err)
	}
	got, _ = s.Get(p.ID)
	if got.Calls[0].Response == nil || got.Calls[0].Response.Body != "ok" {
		t.Fatalf("response lost = %+v", got.Calls[0].Response)
	}

	copied, err := s.DuplicateProjectCall(p.ID, call.ID)
	if err != nil {
		t.Fatalf("DuplicateProjectCall: %v", err)
	}
	if len(got.Calls) != 1 || copied.ID == call.ID {
		t.Fatalf("duplicate = %+v", copied)
	}

	if err := s.DeleteProjectCall(p.ID, call.ID); err != nil {
		t.Fatalf("DeleteProjectCall: %v", err)
	}
	got, _ = s.Get(p.ID)
	if len(got.Calls) != 1 || got.Calls[0].ID != copied.ID {
		t.Fatalf("after delete: %+v", got.Calls)
	}
}

// TestEnsureIDsCoversNativeTree checks groups and project calls get unique ids
// on import.
func TestEnsureIDsCoversNativeTree(t *testing.T) {
	p := model.Project{
		Name: "n", Source: "native",
		Groups: []model.Group{{Name: "g", ID: "dup"}, {Name: "h", ID: "dup"}},
		Calls:  []model.Call{{Title: "c", ID: "dup"}},
	}
	EnsureIDs(&p)
	if p.Groups[0].ID == p.Groups[1].ID {
		t.Fatal("duplicate group ids not resolved")
	}
	if p.Calls[0].ID == p.Groups[0].ID || p.Calls[0].ID == p.Groups[1].ID {
		t.Fatal("call id collides with a group id")
	}
	if p.Calls[0].ID == "" {
		t.Fatal("call id not assigned")
	}
}
