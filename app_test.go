package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"soapgo/internal/model"
	"soapgo/internal/transport"
	"soapgo/internal/workspace"
)

// newTestApp builds an app on a fresh, empty workspace in a temp directory.
func newTestApp(t *testing.T) *app {
	t.Helper()
	dir := t.TempDir()
	reg, err := workspace.Open(dir, "")
	if err != nil {
		t.Fatalf("workspace.Open: %v", err)
	}
	return newApp(reg)
}

// TestProjectAndCallLifecycle drives the same actions the UI buttons call:
// create a project, add a service/operation/call, select the call, edit it and
// save it back through the store.
func TestProjectAndCallLifecycle(t *testing.T) {
	a := newTestApp(t)

	a.createProject("演示项目")
	a.reload()
	if len(a.projects) != 1 {
		t.Fatalf("projects = %d, want 1", len(a.projects))
	}
	p := a.projects[0]

	svc, err := a.st.AddService(p.ID, model.Service{Name: "Svc"})
	if err != nil {
		t.Fatalf("AddService: %v", err)
	}
	op, err := a.st.AddOperation(p.ID, svc.ID, model.Operation{
		Name: "GetUser", Endpoint: "http://old", SoapAction: "urn:getUser",
	})
	if err != nil {
		t.Fatalf("AddOperation: %v", err)
	}
	call, err := a.st.AddCall(p.ID, svc.ID, op.ID, model.Call{
		Title: "请求1", Endpoint: "http://old", Action: "urn:getUser", Request: "<old/>",
	})
	if err != nil {
		t.Fatalf("AddCall: %v", err)
	}

	a.reload()
	a.openCall(p.ID, svc.ID, op.ID, call.ID)
	if a.endpoint != "http://old" || a.action != "urn:getUser" {
		t.Fatalf("loaded endpoint=%q action=%q", a.endpoint, a.action)
	}
	if a.xmlEd.Buffer.String() != "<old/>" {
		t.Fatalf("loaded body = %q", a.xmlEd.Buffer.String())
	}
	if len(a.tabs) != 1 {
		t.Fatalf("tabs = %d, want 1", len(a.tabs))
	}

	a.endpoint = "http://new"
	a.action = "urn:new"
	a.headers = append(a.headers, headerRow{Name: "X-Test", Value: "1"})
	a.xmlEd.Buffer.Reset("<new/>")
	a.saveCall()

	got, err := a.st.Get(p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	var saved *model.Call
	for i := range got.Services[0].Operations[0].Calls {
		if got.Services[0].Operations[0].Calls[i].ID == call.ID {
			saved = &got.Services[0].Operations[0].Calls[i]
		}
	}
	if saved == nil {
		t.Fatal("saved call not found")
	}
	if saved.Endpoint != "http://new" || saved.Action != "urn:new" || saved.Request != "<new/>" {
		t.Fatalf("saved = %+v", *saved)
	}
	if len(saved.Headers) != 1 || saved.Headers[0].Name != "X-Test" {
		t.Fatalf("saved headers = %+v", saved.Headers)
	}
}

// TestDeleteCallClearsEditor checks the destructive path the UI exposes.
func TestDeleteCallClearsEditor(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	svc, _ := a.st.AddService(p.ID, model.Service{Name: "S"})
	op, _ := a.st.AddOperation(p.ID, svc.ID, model.Operation{Name: "O"})
	call, _ := a.st.AddCall(p.ID, svc.ID, op.ID, model.Call{Title: "c", Request: "<a/>"})

	a.reload()
	a.openCall(p.ID, svc.ID, op.ID, call.ID)
	a.deleteCall()
	if !a.confirmOpen {
		t.Fatal("delete did not ask for confirmation")
	}
	a.confirmOpen = false
	fn := a.confirmAction
	a.confirmAction = nil
	fn()

	if a.cid != "" || len(a.tabs) != 0 {
		t.Fatalf("after delete: cid=%q tabs=%d", a.cid, len(a.tabs))
	}
	if len(a.projects[0].Services[0].Operations[0].Calls) != 0 {
		t.Fatal("call still in store")
	}
}

func TestFormatXML(t *testing.T) {
	in := `<a><b>1</b><c x="1"/></a>`
	out, err := formatXML(in)
	if err != nil {
		t.Fatalf("formatXML: %v", err)
	}
	want := "<a>\n  <b>\n    1\n  </b>\n  <c x=\"1\"/>\n</a>"
	if out != want {
		t.Fatalf("formatXML = %q, want %q", out, want)
	}
}

func TestMinify(t *testing.T) {
	a := newTestApp(t)
	a.xmlEd.Buffer.Reset("<a>\n  <b>1</b>\n</a>")
	a.minifyBody()
	if got := a.xmlEd.Buffer.String(); got != "<a><b>1</b></a>" {
		t.Fatalf("minify = %q", got)
	}
}

// TestBuildSendRequest checks the payload the Send button hands to transport:
// project headers, sample headers, the default SOAPAction and Content-Type, and
// the editor body.
func TestBuildSendRequest(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	proj, _ := a.st.Get(p.ID)
	proj.Headers = []model.Header{{Name: "X-Proj", Value: "2"}}
	if err := a.st.UpdateProject(proj); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	svc, _ := a.st.AddService(p.ID, model.Service{Name: "S"})
	op, _ := a.st.AddOperation(p.ID, svc.ID, model.Operation{Name: "O"})
	call, _ := a.st.AddCall(p.ID, svc.ID, op.ID, model.Call{
		Title: "c", Endpoint: "http://example.test/soap", Action: "urn:a", Request: "<req/>",
		Headers: []model.Header{{Name: "X-Call", Value: "1"}},
	})
	a.reload()
	a.openCall(p.ID, svc.ID, op.ID, call.ID)
	a.timeout = "5000"

	req, proxy, err := a.buildSendRequest()
	if err != nil {
		t.Fatalf("buildSendRequest: %v", err)
	}
	if req.URL != "http://example.test/soap" || req.Method != "POST" || req.Body != "<req/>" {
		t.Fatalf("req = %+v", req)
	}
	if req.TimeoutMS != 5000 {
		t.Fatalf("timeout = %d", req.TimeoutMS)
	}
	want := map[string]string{
		"SOAPAction":   "urn:a",
		"Content-Type": "text/xml; charset=utf-8",
		"X-Proj":       "2",
		"X-Call":       "1",
	}
	for k, v := range want {
		if req.Headers[k] != v {
			t.Fatalf("header %s = %q, want %q (%v)", k, req.Headers[k], v, req.Headers)
		}
	}
	if proxy != nil {
		t.Fatalf("proxy = %+v, want nil", proxy)
	}
}

func TestBuildSendRequestRejectsBadURL(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	svc, _ := a.st.AddService(p.ID, model.Service{Name: "S"})
	op, _ := a.st.AddOperation(p.ID, svc.ID, model.Operation{Name: "O"})
	call, _ := a.st.AddCall(p.ID, svc.ID, op.ID, model.Call{Title: "c", Request: "<a/>"})
	a.reload()
	a.openCall(p.ID, svc.ID, op.ID, call.ID)
	a.endpoint = "ftp://nope"
	if _, _, err := a.buildSendRequest(); err == nil {
		t.Fatal("expected an error for a non-http URL")
	}
}

// TestSendPayloadReachesServer runs the exact payload buildSendRequest makes
// through transport.Send against a real HTTP server.
func TestSendPayloadReachesServer(t *testing.T) {
	var gotAction, gotBody, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAction = r.Header.Get("SOAPAction")
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		io.WriteString(w, `<soap:Envelope><soap:Body><ok>1</ok></soap:Body></soap:Envelope>`)
	}))
	defer srv.Close()

	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	svc, _ := a.st.AddService(p.ID, model.Service{Name: "S"})
	op, _ := a.st.AddOperation(p.ID, svc.ID, model.Operation{Name: "O"})
	call, _ := a.st.AddCall(p.ID, svc.ID, op.ID, model.Call{
		Title: "c", Endpoint: srv.URL, Action: "urn:send", Request: "<req>1</req>",
	})
	a.reload()
	a.openCall(p.ID, svc.ID, op.ID, call.ID)

	req, proxy, err := a.buildSendRequest()
	if err != nil {
		t.Fatalf("buildSendRequest: %v", err)
	}
	resp := transport.Send(req, proxy)
	if resp.Error != "" {
		t.Fatalf("send error: %s", resp.Error)
	}
	if resp.Status != 200 || !strings.Contains(resp.Body, "<ok>1</ok>") {
		t.Fatalf("response = %+v", resp)
	}
	if gotAction != "urn:send" || gotCT != "text/xml; charset=utf-8" || gotBody != "<req>1</req>" {
		t.Fatalf("server saw action=%q ct=%q body=%q", gotAction, gotCT, gotBody)
	}
}
func TestAddServiceOperationAndRenameCall(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	a.pid = p.ID
	a.addService("Svc")
	a.reload()
	if len(a.projects[0].Services) != 1 || a.projects[0].Services[0].Name != "Svc" {
		t.Fatalf("services = %+v", a.projects[0].Services)
	}
	a.sid = a.projects[0].Services[0].ID
	a.addOperation("Op")
	a.reload()
	svc := a.projects[0].Services[0]
	if len(svc.Operations) != 1 || svc.Operations[0].Name != "Op" {
		t.Fatalf("operations = %+v", svc.Operations)
	}
	a.oid = svc.Operations[0].ID
	call, _ := a.st.AddCall(p.ID, a.sid, a.oid, model.Call{Title: "原名", Request: "<a/>"})
	a.reload()
	a.openCall(p.ID, a.sid, a.oid, call.ID)
	a.renameCall("新名")
	a.reload()
	if got := a.projects[0].Services[0].Operations[0].Calls[0].Title; got != "新名" {
		t.Fatalf("title = %q", got)
	}
}

// TestPersistResponseAutoSaves checks the automatic save that replaced the
// response's 保存 button: a sent response is stored with its sample, and a
// failed (or cancelled) one is not.
func TestPersistResponseAutoSaves(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	svc, _ := a.st.AddService(p.ID, model.Service{Name: "S"})
	op, _ := a.st.AddOperation(p.ID, svc.ID, model.Operation{Name: "O"})
	call, _ := a.st.AddCall(p.ID, svc.ID, op.ID, model.Call{Title: "c", Request: "<a/>"})
	a.reload()

	resp := model.SendResponse{Status: 200, StatusText: "OK", Body: "<ok/>", DurationMS: 12, Size: 5}
	a.persistResponse(p.ID, svc.ID, op.ID, call.ID, resp)

	got, _ := a.st.Get(p.ID)
	saved := got.Services[0].Operations[0].Calls[0].Response
	if saved == nil || saved.Status != 200 || saved.Body != "<ok/>" {
		t.Fatalf("saved response = %+v", saved)
	}

	// A failed send must leave the previously saved response intact.
	a.persistResponse(p.ID, svc.ID, op.ID, call.ID, model.SendResponse{Error: "boom"})
	got, _ = a.st.Get(p.ID)
	if s := got.Services[0].Operations[0].Calls[0].Response; s == nil || s.Body != "<ok/>" {
		t.Fatalf("failed send changed saved response = %+v", s)
	}
}

func TestIsDirtyAndSaveClears(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	svc, _ := a.st.AddService(p.ID, model.Service{Name: "S"})
	op, _ := a.st.AddOperation(p.ID, svc.ID, model.Operation{Name: "O"})
	call, _ := a.st.AddCall(p.ID, svc.ID, op.ID, model.Call{Title: "c", Endpoint: "http://x", Request: "<a/>"})
	a.reload()
	a.openCall(p.ID, svc.ID, op.ID, call.ID)
	if a.isDirty() {
		t.Fatal("freshly opened call is dirty")
	}
	a.xmlEd.Buffer.Reset("<b/>")
	if !a.isDirty() {
		t.Fatal("edited body is not dirty")
	}
	a.saveCall()
	if a.isDirty() {
		t.Fatal("saved call is still dirty")
	}
}
func TestSendContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		io.WriteString(w, "late")
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	resp := transport.SendContext(ctx, model.SendRequest{
		URL: srv.URL, Method: "POST", Headers: map[string]string{}, Body: "<a/>", TimeoutMS: 5000,
	}, nil)
	if resp.Error != "已取消" {
		t.Fatalf("error = %q, want 已取消", resp.Error)
	}
	if d := time.Since(start); d > 1500*time.Millisecond {
		t.Fatalf("cancel took %v", d)
	}
}

func TestUIStatePersists(t *testing.T) {
	dir := t.TempDir()
	reg, err := workspace.Open(dir, "")
	if err != nil {
		t.Fatalf("workspace.Open: %v", err)
	}
	a := newApp(reg)
	a.dark = false
	a.splitSide = 1
	a.sideW = 333
	a.splitV = 55
	a.splitH = 44
	a.saveUIState()

	reg2, err := workspace.Open(dir, "")
	if err != nil {
		t.Fatalf("workspace.Open 2: %v", err)
	}
	b := newApp(reg2)
	if b.dark != false || b.splitSide != 1 || b.sideW != 333 || b.splitV != 55 || b.splitH != 44 {
		t.Fatalf("restored = %+v", b.currentUI())
	}
}
