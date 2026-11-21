// Package store persists projects to a local JSON file and provides CRUD over
// the project -> service -> operation -> call hierarchy.
//
// It is safe for concurrent use: callers receive deep copies, and every
// mutation is applied to a copy, persisted, and only then swapped into memory.
// Writes are atomic (temp file + rename), so a crash cannot truncate the file.
package store

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"soapgo/internal/fileutil"
	"soapgo/internal/model"
)

// ErrNotFound is returned when a referenced node does not exist.
var ErrNotFound = errors.New("未找到指定对象")

// Store is the persistent project repository.
type Store struct {
	// mu guards projects for readers and the final in-memory swap.
	mu sync.RWMutex
	// writeMu serializes the clone -> write -> swap cycle, so JSON encoding and
	// disk I/O never block readers.
	writeMu  sync.Mutex
	path     string
	projects []model.Project
}

// New loads (or initialises) the store at the given file path.
func New(path string) (*Store, error) {
	s := &Store{path: path}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil // start empty
		}
		return nil, err
	}
	// Ignore a UTF-8 BOM: Windows editors add one by default and
	// encoding/json rejects it.
	data = fileutil.StripBOM(data)
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &s.projects); err != nil {
			return nil, err
		}
	}
	// Older WSDL imports stored operations without any request sample. Opening
	// such a project showed an empty editor, which looks like a failed import.
	// Backfill one sample from the generated template and persist the fix.
	if backfillWSDLCalls(s.projects) {
		if err := s.saveProjects(s.projects); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// backfillWSDLCalls gives legacy WSDL-imported operations a default request
// sample when the operation carries a generated template but no call. It is
// idempotent: projects that already have calls are left untouched.
func backfillWSDLCalls(projects []model.Project) bool {
	changed := false
	for pi := range projects {
		project := &projects[pi]
		if project.Source != "wsdl" && project.Source != "wsdl-url" {
			continue
		}
		for si := range project.Services {
			for oi := range project.Services[si].Operations {
				op := &project.Services[si].Operations[oi]
				if len(op.Calls) > 0 {
					continue
				}
				call, ok := model.SeedCall(op)
				if !ok {
					continue
				}
				call.ID = NewID()
				op.Calls = []model.Call{call}
				changed = true
			}
		}
	}
	return changed
}

// NewID returns a short random hex identifier.
func NewID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// saveProjects persists the project list atomically. Callers either run before
// the store is shared (New) or hold writeMu (commit).
func (s *Store) saveProjects(projects []model.Project) error {
	return fileutil.WriteJSONAtomic(s.path, projects, 0o644)
}

// commit applies mutate to a deep copy, persists it, and only then swaps it
// into memory, so a failed write leaves memory unchanged. Writers are
// serialized by writeMu; readers keep serving the previous snapshot while the
// file is being encoded and written.
func (s *Store) commit(mutate func([]model.Project) ([]model.Project, error)) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	s.mu.RLock()
	next := cloneProjects(s.projects)
	s.mu.RUnlock()

	next, err := mutate(next)
	if err != nil {
		return err
	}
	if err := s.saveProjects(next); err != nil {
		return err
	}

	s.mu.Lock()
	s.projects = next
	s.mu.Unlock()
	return nil
}

// ---- deep copy helpers ----

// cloneProject returns a project that shares no mutable state with p.
func cloneProject(p model.Project) model.Project {
	out := p
	if p.Proxy != nil {
		proxy := *p.Proxy
		out.Proxy = &proxy
	}
	out.Headers = cloneHeaders(p.Headers)
	out.Groups = cloneGroups(p.Groups)
	out.Calls = cloneCalls(p.Calls)
	if p.Services != nil {
		out.Services = make([]model.Service, len(p.Services))
		for i, svc := range p.Services {
			out.Services[i] = cloneService(svc)
		}
	}
	if p.TestSuites != nil {
		out.TestSuites = make([]model.TestSuite, len(p.TestSuites))
		for i, ts := range p.TestSuites {
			out.TestSuites[i] = cloneTestSuite(ts)
		}
	}
	return out
}

// cloneHeaders copies a header slice, preserving nil so an untouched project
// keeps its JSON shape (omitempty).
func cloneHeaders(in []model.Header) []model.Header {
	if in == nil {
		return nil
	}
	return append([]model.Header(nil), in...)
}

// cloneGroups copies the flat folder slice. Group holds only value fields, so a
// slice copy is a full copy.
func cloneGroups(in []model.Group) []model.Group {
	if in == nil {
		return nil
	}
	return append([]model.Group(nil), in...)
}

// cloneCalls copies a call slice deeply enough that no header, auth or response
// is shared with the source.
func cloneCalls(in []model.Call) []model.Call {
	if in == nil {
		return nil
	}
	out := make([]model.Call, len(in))
	for i, c := range in {
		out[i] = cloneCall(c)
	}
	return out
}

func cloneService(svc model.Service) model.Service {
	out := svc
	if svc.Operations != nil {
		out.Operations = make([]model.Operation, len(svc.Operations))
		for i, op := range svc.Operations {
			out.Operations[i] = cloneOperation(op)
		}
	}
	return out
}

func cloneOperation(op model.Operation) model.Operation {
	out := op
	if op.Calls != nil {
		out.Calls = make([]model.Call, len(op.Calls))
		for i, c := range op.Calls {
			out.Calls[i] = cloneCall(c)
		}
	}
	return out
}

func cloneCall(c model.Call) model.Call {
	out := c
	out.Headers = cloneHeaders(c.Headers)
	if c.BodyFields != nil {
		out.BodyFields = append([]model.BodyField(nil), c.BodyFields...)
	}
	out.Response = cloneSavedResponse(c.Response)
	return out
}

// cloneSavedResponse deep-copies the response and its header map, preserving nil
// so an untouched sample keeps its JSON shape.
func cloneSavedResponse(r *model.SavedResponse) *model.SavedResponse {
	if r == nil {
		return nil
	}
	out := *r
	if r.Headers != nil {
		out.Headers = make(map[string]string, len(r.Headers))
		for k, v := range r.Headers {
			out.Headers[k] = v
		}
	}
	return &out
}

func cloneTestSuite(ts model.TestSuite) model.TestSuite {
	out := ts
	if ts.TestCases != nil {
		out.TestCases = make([]model.TestCase, len(ts.TestCases))
		for i, tc := range ts.TestCases {
			c := tc
			if tc.Steps != nil {
				c.Steps = make([]model.TestStep, len(tc.Steps))
				copy(c.Steps, tc.Steps)
			}
			out.TestCases[i] = c
		}
	}
	return out
}

func cloneProjects(in []model.Project) []model.Project {
	if in == nil {
		return nil
	}
	out := make([]model.Project, len(in))
	for i, p := range in {
		out[i] = cloneProject(p)
	}
	return out
}

// indexOfProject returns the index of a project by id, or -1.
func indexOfProject(list []model.Project, id string) int {
	for i := range list {
		if list[i].ID == id {
			return i
		}
	}
	return -1
}

// locateService returns the indexes of a service within a project. Callers hold
// the store lock; ok=false means the project or service does not exist.
func locateService(list []model.Project, projectID, serviceID string) (pi, si int, ok bool) {
	pi = indexOfProject(list, projectID)
	if pi < 0 {
		return 0, 0, false
	}
	for si = range list[pi].Services {
		if list[pi].Services[si].ID == serviceID {
			return pi, si, true
		}
	}
	return 0, 0, false
}

// locateOperation returns the indexes of an operation within a service.
func locateOperation(list []model.Project, projectID, serviceID, opID string) (pi, si, oi int, ok bool) {
	pi, si, ok = locateService(list, projectID, serviceID)
	if !ok {
		return 0, 0, 0, false
	}
	for oi = range list[pi].Services[si].Operations {
		if list[pi].Services[si].Operations[oi].ID == opID {
			return pi, si, oi, true
		}
	}
	return 0, 0, 0, false
}

// List returns a deep copy of all projects. An empty store returns [] rather
// than null, since "no projects yet" is a normal state.
func (s *Store) List() []model.Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.projects == nil {
		return []model.Project{}
	}
	return cloneProjects(s.projects)
}

// Get returns a deep copy of one project by id.
func (s *Store) Get(id string) (model.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if i := indexOfProject(s.projects, id); i >= 0 {
		return cloneProject(s.projects[i]), nil
	}
	return model.Project{}, ErrNotFound
}

// ---- project-level CRUD ----

// AddProject appends a project, assigning an id if missing, and saves.
func (s *Store) AddProject(p model.Project) (model.Project, error) {
	if err := p.Proxy.Validate(); err != nil {
		return p, err
	}
	if p.ID == "" {
		p.ID = NewID()
	}
	if p.Name == "" {
		p.Name = "未命名项目"
	}
	EnsureIDs(&p)
	err := s.commit(func(list []model.Project) ([]model.Project, error) {
		return append(list, cloneProject(p)), nil
	})
	return p, err
}

// UpdateProject replaces a whole project (matched by id) and saves.
func (s *Store) UpdateProject(p model.Project) error {
	if err := p.Proxy.Validate(); err != nil {
		return err
	}
	// A full replace must leave the same invariant as AddProject: every node
	// reachable by id. Without this a client that omits an id silently persists
	// a node that no CRUD route can address afterwards.
	EnsureIDs(&p)
	return s.commit(func(list []model.Project) ([]model.Project, error) {
		i := indexOfProject(list, p.ID)
		if i < 0 {
			return nil, ErrNotFound
		}
		list[i] = cloneProject(p)
		return list, nil
	})
}

// UpdateProjectProxy updates only the proxy settings. A narrow write avoids
// overwriting concurrent edits from another tab with a stale snapshot.
func (s *Store) UpdateProjectProxy(id string, proxy *model.ProxySettings) error {
	if err := proxy.Validate(); err != nil {
		return err
	}
	return s.commit(func(list []model.Project) ([]model.Project, error) {
		i := indexOfProject(list, id)
		if i < 0 {
			return nil, ErrNotFound
		}
		if proxy == nil {
			list[i].Proxy = nil
		} else {
			p := *proxy
			list[i].Proxy = &p
		}
		return list, nil
	})
}

// UpdateProjectHost updates only a project's base address. A narrow write keeps
// a stale snapshot from overwriting concurrent edits elsewhere.
func (s *Store) UpdateProjectHost(id, host string) error {
	if err := model.ValidateHost(host); err != nil {
		return err
	}
	return s.commit(func(list []model.Project) ([]model.Project, error) {
		i := indexOfProject(list, id)
		if i < 0 {
			return nil, ErrNotFound
		}
		list[i].Host = model.NormalizeHost(host)
		return list, nil
	})
}

// RenameProject updates only a project's name.
func (s *Store) RenameProject(id, name string) error {
	return s.commit(func(list []model.Project) ([]model.Project, error) {
		i := indexOfProject(list, id)
		if i < 0 {
			return nil, ErrNotFound
		}
		list[i].Name = name
		return list, nil
	})
}

// DeleteProject removes a project by id.
func (s *Store) DeleteProject(id string) error {
	return s.commit(func(list []model.Project) ([]model.Project, error) {
		i := indexOfProject(list, id)
		if i < 0 {
			return nil, ErrNotFound
		}
		return append(list[:i], list[i+1:]...), nil
	})
}

// ---- nested CRUD helpers ----

// AddService appends a service to a project.
func (s *Store) AddService(projectID string, svc model.Service) (model.Service, error) {
	// The service is new to the project, so only its own subtree has to be
	// self-consistent: nothing being inserted can collide with what it joins.
	claim := NewIDClaimer()
	svc.ID = claim(svc.ID)
	for i := range svc.Operations {
		assignOperationIDs(&svc.Operations[i], claim)
	}
	err := s.commit(func(list []model.Project) ([]model.Project, error) {
		pi := indexOfProject(list, projectID)
		if pi < 0 {
			return nil, ErrNotFound
		}
		list[pi].Services = append(list[pi].Services, cloneService(svc))
		return list, nil
	})
	return svc, err
}

// DeleteService removes a service from a project.
func (s *Store) DeleteService(projectID, serviceID string) error {
	return s.commit(func(list []model.Project) ([]model.Project, error) {
		pi, si, ok := locateService(list, projectID, serviceID)
		if !ok {
			return nil, ErrNotFound
		}
		list[pi].Services = append(list[pi].Services[:si], list[pi].Services[si+1:]...)
		return list, nil
	})
}

// AddOperation appends an operation to a service.
func (s *Store) AddOperation(projectID, serviceID string, op model.Operation) (model.Operation, error) {
	// As in AddService: only the inserted subtree needs to be self-consistent.
	assignOperationIDs(&op, NewIDClaimer())
	err := s.commit(func(list []model.Project) ([]model.Project, error) {
		pi, si, ok := locateService(list, projectID, serviceID)
		if !ok {
			return nil, ErrNotFound
		}
		list[pi].Services[si].Operations = append(list[pi].Services[si].Operations, cloneOperation(op))
		return list, nil
	})
	return op, err
}

// DeleteOperation removes an operation from a service.
func (s *Store) DeleteOperation(projectID, serviceID, opID string) error {
	return s.commit(func(list []model.Project) ([]model.Project, error) {
		pi, si, oi, ok := locateOperation(list, projectID, serviceID, opID)
		if !ok {
			return nil, ErrNotFound
		}
		svc := &list[pi].Services[si]
		svc.Operations = append(svc.Operations[:oi], svc.Operations[oi+1:]...)
		return list, nil
	})
}

// AddCall appends a request sample (call) to an operation.
func (s *Store) AddCall(projectID, serviceID, opID string, call model.Call) (model.Call, error) {
	if call.ID == "" {
		call.ID = NewID()
	}
	if call.Title == "" {
		call.Title = "新请求 " + time.Now().Format("15:04:05")
	}
	err := s.commit(func(list []model.Project) ([]model.Project, error) {
		pi, si, oi, ok := locateOperation(list, projectID, serviceID, opID)
		if !ok {
			return nil, ErrNotFound
		}
		op := &list[pi].Services[si].Operations[oi]
		// Seed the new request from the operation's structural template
		// (方案 A: pure skeleton from WSDL, no business data).
		if call.Request == "" {
			call.Request = op.DefaultRequestBody()
		}
		if call.Endpoint == "" {
			call.Endpoint = op.Endpoint
		}
		if call.Action == "" {
			call.Action = op.SoapAction
		}
		// cloneCall: the caller keeps ownership of the value it passed in. The
		// package promises that no mutable state is shared with callers, and
		// Headers is a slice.
		op.Calls = append(op.Calls, cloneCall(call))
		return list, nil
	})
	return call, err
}

// UpdateCall replaces a call (by id) within an operation.
func (s *Store) UpdateCall(projectID, serviceID, opID string, call model.Call) error {
	return s.commit(func(list []model.Project) ([]model.Project, error) {
		pi, si, oi, ok := locateOperation(list, projectID, serviceID, opID)
		if !ok {
			return nil, ErrNotFound
		}
		op := &list[pi].Services[si].Operations[oi]
		for ci := range op.Calls {
			if op.Calls[ci].ID == call.ID {
				// The saved response is written only by UpdateCallResponse: a
				// request edit must not erase it by omitting the field.
				call.Response = op.Calls[ci].Response
				op.Calls[ci] = cloneCall(call)
				return list, nil
			}
		}
		return nil, ErrNotFound
	})
}

// UpdateCallResponse stores (or, with nil, clears) the saved response of one
// request sample. A narrow write keeps a response save from overwriting
// concurrent request edits made in another tab.
func (s *Store) UpdateCallResponse(projectID, serviceID, opID, callID string, resp *model.SavedResponse) error {
	return s.commit(func(list []model.Project) ([]model.Project, error) {
		pi, si, oi, ok := locateOperation(list, projectID, serviceID, opID)
		if !ok {
			return nil, ErrNotFound
		}
		op := &list[pi].Services[si].Operations[oi]
		for ci := range op.Calls {
			if op.Calls[ci].ID == callID {
				op.Calls[ci].Response = cloneSavedResponse(resp)
				return list, nil
			}
		}
		return nil, ErrNotFound
	})
}

// DeleteCall removes a call from an operation.
func (s *Store) DeleteCall(projectID, serviceID, opID, callID string) error {
	return s.commit(func(list []model.Project) ([]model.Project, error) {
		pi, si, oi, ok := locateOperation(list, projectID, serviceID, opID)
		if !ok {
			return nil, ErrNotFound
		}
		op := &list[pi].Services[si].Operations[oi]
		for ci := range op.Calls {
			if op.Calls[ci].ID == callID {
				op.Calls = append(op.Calls[:ci], op.Calls[ci+1:]...)
				return list, nil
			}
		}
		return nil, ErrNotFound
	})
}

// DuplicateCall copies a call within its operation, giving the copy a title
// that does not collide with a sibling ("查询" → "查询 (副本)" → "查询 (副本 2)").
// The UI uses it to create a variant of an existing sample instead of starting
// from the operation template.
func (s *Store) DuplicateCall(projectID, serviceID, opID, callID string) (model.Call, error) {
	var out model.Call
	err := s.commit(func(list []model.Project) ([]model.Project, error) {
		pi, si, oi, ok := locateOperation(list, projectID, serviceID, opID)
		if !ok {
			return nil, ErrNotFound
		}
		op := &list[pi].Services[si].Operations[oi]
		var copied bool
		out, op.Calls, copied = duplicateCall(op.Calls, callID)
		if !copied {
			return nil, ErrNotFound
		}
		return list, nil
	})
	return out, err
}

// duplicateCall appends a copy of the call with callID to calls, under a fresh
// id and a "(副本)" title, sharing nothing mutable with the original. copied is
// false when no call has that id.
func duplicateCall(calls []model.Call, callID string) (out model.Call, grown []model.Call, copied bool) {
	for _, c := range calls {
		if c.ID != callID {
			continue
		}
		c.ID = NewID()
		c.Title = copyTitle(c.Title, calls)
		c.Headers = append([]model.Header(nil), c.Headers...)
		c.BodyFields = append([]model.BodyField(nil), c.BodyFields...)
		c.Auth = c.Auth.Clone()
		c.Response = cloneSavedResponse(c.Response)
		return c, append(calls, c), true
	}
	return model.Call{}, calls, false
}

// copyTitle returns "<title> (副本)" or, when that is taken, the first free
// "<title> (副本 N)" with N starting at 2.
func copyTitle(title string, siblings []model.Call) string {
	if title == "" {
		title = "请求"
	}
	base := title + " (副本)"
	taken := make(map[string]bool, len(siblings))
	for _, c := range siblings {
		taken[c.Title] = true
	}
	if !taken[base] {
		return base
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s (副本 %d)", title, n)
		if !taken[candidate] {
			return candidate
		}
	}
}

// ---- native tree CRUD: groups and project-level requests ----

// AddGroup creates a folder under the project or another group.
func (s *Store) AddGroup(projectID, name, parentID string) (model.Group, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "新建目录"
	}
	g := model.Group{ID: NewID(), Name: name, ParentID: parentID}
	err := s.commit(func(list []model.Project) ([]model.Project, error) {
		i := indexOfProject(list, projectID)
		if i < 0 {
			return nil, ErrNotFound
		}
		if parentID != "" && !hasGroup(list[i].Groups, parentID) {
			return nil, ErrNotFound
		}
		list[i].Groups = append(list[i].Groups, g)
		return list, nil
	})
	return g, err
}

// RenameGroup changes a folder's name.
func (s *Store) RenameGroup(projectID, groupID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("目录名称不能为空")
	}
	return s.commit(func(list []model.Project) ([]model.Project, error) {
		i := indexOfProject(list, projectID)
		if i < 0 {
			return nil, ErrNotFound
		}
		for gi := range list[i].Groups {
			if list[i].Groups[gi].ID == groupID {
				list[i].Groups[gi].Name = name
				return list, nil
			}
		}
		return nil, ErrNotFound
	})
}

// DeleteGroup removes a folder together with every nested folder and every
// request inside them. It returns the removed request ids so the UI can close
// their tabs.
func (s *Store) DeleteGroup(projectID, groupID string) (removedCallIDs []string, err error) {
	err = s.commit(func(list []model.Project) ([]model.Project, error) {
		i := indexOfProject(list, projectID)
		if i < 0 {
			return nil, ErrNotFound
		}
		if !hasGroup(list[i].Groups, groupID) {
			return nil, ErrNotFound
		}
		doomed := descendantGroupIDs(list[i].Groups, groupID)
		// Drop the groups.
		kept := list[i].Groups[:0]
		for _, g := range list[i].Groups {
			if !doomed[g.ID] {
				kept = append(kept, g)
			}
		}
		list[i].Groups = kept
		// Drop the calls living in those groups.
		keptCalls := list[i].Calls[:0]
		for _, c := range list[i].Calls {
			if doomed[c.FolderID] {
				removedCallIDs = append(removedCallIDs, c.ID)
				continue
			}
			keptCalls = append(keptCalls, c)
		}
		list[i].Calls = keptCalls
		return list, nil
	})
	return removedCallIDs, err
}

// hasGroup reports whether the project owns a group with this id.
func hasGroup(groups []model.Group, id string) bool {
	for _, g := range groups {
		if g.ID == id {
			return true
		}
	}
	return false
}

// descendantGroupIDs returns gid plus every group nested under it.
func descendantGroupIDs(groups []model.Group, gid string) map[string]bool {
	out := map[string]bool{gid: true}
	for changed := true; changed; {
		changed = false
		for _, g := range groups {
			if out[g.ParentID] && !out[g.ID] {
				out[g.ID] = true
				changed = true
			}
		}
	}
	return out
}

// AddProjectCall inserts a request under the project root (folderID "") or one
// of its groups. Kind defaults to HTTP.
func (s *Store) AddProjectCall(projectID, folderID string, call model.Call) (model.Call, error) {
	if call.ID == "" {
		call.ID = NewID()
	}
	if strings.TrimSpace(call.Title) == "" {
		call.Title = "新请求 " + time.Now().Format("15:04:05")
	}
	call.FolderID = folderID
	if call.Kind == "" {
		call.Kind = model.KindHTTP
	}
	err := s.commit(func(list []model.Project) ([]model.Project, error) {
		i := indexOfProject(list, projectID)
		if i < 0 {
			return nil, ErrNotFound
		}
		if folderID != "" && !hasGroup(list[i].Groups, folderID) {
			return nil, ErrNotFound
		}
		list[i].Calls = append(list[i].Calls, cloneCall(call))
		return list, nil
	})
	return call, err
}

// UpdateProjectCall replaces a project-level request by id.
func (s *Store) UpdateProjectCall(projectID string, call model.Call) error {
	return s.commit(func(list []model.Project) ([]model.Project, error) {
		i := indexOfProject(list, projectID)
		if i < 0 {
			return nil, ErrNotFound
		}
		for ci := range list[i].Calls {
			if list[i].Calls[ci].ID == call.ID {
				call.Response = list[i].Calls[ci].Response
				call.FolderID = list[i].Calls[ci].FolderID
				list[i].Calls[ci] = cloneCall(call)
				return list, nil
			}
		}
		return nil, ErrNotFound
	})
}

// MoveProjectCall moves a project-level request into folderID, "" being the
// project root, keeping its other fields.
func (s *Store) MoveProjectCall(projectID, callID, folderID string) error {
	return s.commit(func(list []model.Project) ([]model.Project, error) {
		i := indexOfProject(list, projectID)
		if i < 0 {
			return nil, ErrNotFound
		}
		if folderID != "" && !hasGroup(list[i].Groups, folderID) {
			return nil, ErrNotFound
		}
		for ci := range list[i].Calls {
			if list[i].Calls[ci].ID == callID {
				list[i].Calls[ci].FolderID = folderID
				return list, nil
			}
		}
		return nil, ErrNotFound
	})
}

// UpdateProjectCallResponse stores (or clears) one project-level request's
// saved response.
func (s *Store) UpdateProjectCallResponse(projectID, callID string, resp *model.SavedResponse) error {
	return s.commit(func(list []model.Project) ([]model.Project, error) {
		i := indexOfProject(list, projectID)
		if i < 0 {
			return nil, ErrNotFound
		}
		for ci := range list[i].Calls {
			if list[i].Calls[ci].ID == callID {
				list[i].Calls[ci].Response = cloneSavedResponse(resp)
				return list, nil
			}
		}
		return nil, ErrNotFound
	})
}

// DeleteProjectCall removes a project-level request by id.
func (s *Store) DeleteProjectCall(projectID, callID string) error {
	return s.commit(func(list []model.Project) ([]model.Project, error) {
		i := indexOfProject(list, projectID)
		if i < 0 {
			return nil, ErrNotFound
		}
		for ci := range list[i].Calls {
			if list[i].Calls[ci].ID == callID {
				list[i].Calls = append(list[i].Calls[:ci], list[i].Calls[ci+1:]...)
				return list, nil
			}
		}
		return nil, ErrNotFound
	})
}

// DuplicateProjectCall copies a project-level request in the same folder.
func (s *Store) DuplicateProjectCall(projectID, callID string) (model.Call, error) {
	var out model.Call
	err := s.commit(func(list []model.Project) ([]model.Project, error) {
		i := indexOfProject(list, projectID)
		if i < 0 {
			return nil, ErrNotFound
		}
		var copied bool
		out, list[i].Calls, copied = duplicateCall(list[i].Calls, callID)
		if !copied {
			return nil, ErrNotFound
		}
		return list, nil
	})
	return out, err
}

// IDClaimer hands out ids unique within one scope. Blank or repeated ids are
// replaced with fresh ones; everything else is kept, so a healthy tree never
// churns (a reassigned id would break an open tab's selection).
//
// Callers that own several trees in one namespace — an imported workspace file
// is a project array — create a single claimer and thread it through, because
// per-tree uniqueness is not enough: two projects may not share an id either.
type IDClaimer func(id string) string

// NewIDClaimer starts a fresh uniqueness scope.
func NewIDClaimer() IDClaimer {
	seen := map[string]bool{}
	return func(id string) string {
		if id != "" && !seen[id] {
			seen[id] = true
			return id
		}
		// Ids are 8 random bytes, so a fresh one colliding is effectively
		// impossible; the loop is here so that "effectively impossible" cannot
		// quietly become a duplicate.
		for {
			next := NewID()
			if !seen[next] {
				seen[next] = true
				return next
			}
		}
	}
}

// EnsureIDs gives every node in a project tree a unique id.
func EnsureIDs(p *model.Project) {
	EnsureIDsIn(p, NewIDClaimer())
}

// EnsureIDsIn is EnsureIDs using a caller-supplied claimer, so ids stay unique
// across several projects of the same workspace. Uniqueness matters because
// every lookup takes the first match on an id; a collision is reissued rather
// than rejected so an import still succeeds.
func EnsureIDsIn(p *model.Project, claim IDClaimer) {
	p.ID = claim(p.ID)
	for i := range p.Groups {
		p.Groups[i].ID = claim(p.Groups[i].ID)
	}
	for i := range p.Calls {
		p.Calls[i].ID = claim(p.Calls[i].ID)
	}
	for i := range p.Services {
		p.Services[i].ID = claim(p.Services[i].ID)
		for j := range p.Services[i].Operations {
			assignOperationIDs(&p.Services[i].Operations[j], claim)
		}
	}
}

// assignOperationIDs gives an operation and its calls unique ids, sharing the
// caller's claimer so ids stay unique across the whole subtree.
func assignOperationIDs(op *model.Operation, claim IDClaimer) {
	op.ID = claim(op.ID)
	for i := range op.Calls {
		op.Calls[i].ID = claim(op.Calls[i].ID)
	}
}
