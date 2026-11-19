package main

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"soapgo/internal/model"
	"soapgo/internal/soapuiproj"
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

// TestCloseTabKeepsActiveTab checks closing a tab before the active one keeps
// the same call open, closing the active one falls to a neighbour, and
// closing the last one clears the editor.
func TestCloseTabKeepsActiveTab(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	svc, _ := a.st.AddService(p.ID, model.Service{Name: "S"})
	op, _ := a.st.AddOperation(p.ID, svc.ID, model.Operation{Name: "O"})
	c1, _ := a.st.AddCall(p.ID, svc.ID, op.ID, model.Call{Title: "c1", Request: "<a/>"})
	c2, _ := a.st.AddCall(p.ID, svc.ID, op.ID, model.Call{Title: "c2", Request: "<b/>"})
	c3, _ := a.st.AddCall(p.ID, svc.ID, op.ID, model.Call{Title: "c3", Request: "<c/>"})
	a.reload()

	a.openCall(p.ID, svc.ID, op.ID, c1.ID)
	a.openCall(p.ID, svc.ID, op.ID, c2.ID)
	a.openCall(p.ID, svc.ID, op.ID, c3.ID)
	if len(a.tabs) != 3 || a.active != 2 {
		t.Fatalf("tabs=%d active=%d", len(a.tabs), a.active)
	}

	// Closing an inactive tab before the active one keeps c3 open.
	a.closeTab(0)
	if a.cid != c3.ID || a.active != 1 || len(a.tabs) != 2 {
		t.Fatalf("after close before: cid=%q active=%d tabs=%d", a.cid, a.active, len(a.tabs))
	}

	// Closing the active tab falls to the remaining neighbour.
	a.closeTab(1)
	if a.cid != c2.ID || a.active != 0 || len(a.tabs) != 1 {
		t.Fatalf("after close active: cid=%q active=%d tabs=%d", a.cid, a.active, len(a.tabs))
	}

	// Closing the last tab clears the selection and the editor.
	a.closeTab(0)
	if a.cid != "" || len(a.tabs) != 0 || a.xmlEd.Buffer.String() != "" {
		t.Fatalf("after last close: cid=%q tabs=%d body=%q", a.cid, len(a.tabs), a.xmlEd.Buffer.String())
	}
}

// TestToolbarButtonClicks checks a compact toolbar button still runs its
// action through the headless build of the real view.
func TestToolbarButtonClicks(t *testing.T) {
	a := newTestApp(t)
	a.reqKind = model.KindSOAP
	a.xmlEd.Buffer.Reset("<a>\n  <b>1</b>\n</a>")
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("压缩"); err != nil {
		t.Fatalf("click 压缩: %v", err)
	}
	tt.Frame()
	if got := a.xmlEd.Buffer.String(); got != "<a><b>1</b></a>" {
		t.Fatalf("after 压缩 = %q", got)
	}
}

// TestInlineRenameCommits checks the inline rename commits through the same
// store path as the old dialogs, for both a request sample and a project.
func TestInlineRenameCommits(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	svc, _ := a.st.AddService(p.ID, model.Service{Name: "S"})
	op, _ := a.st.AddOperation(p.ID, svc.ID, model.Operation{Name: "O"})
	call, _ := a.st.AddCall(p.ID, svc.ID, op.ID, model.Call{Title: "旧名字", Request: "<a/>"})
	a.reload()

	a.startInlineRename(call.ID, "旧名字")
	a.renameInput = "新名字"
	a.commitInlineRename(treeItem{kind: "call", pid: p.ID, sid: svc.ID, oid: op.ID, cid: call.ID})
	if got := a.projects[0].Services[0].Operations[0].Calls[0].Title; got != "新名字" {
		t.Fatalf("call title = %q", got)
	}
	if a.renamingID != "" {
		t.Fatal("inline rename did not finish")
	}

	a.startInlineRename(p.ID, "P")
	a.renameInput = "新项目"
	a.commitInlineRename(treeItem{kind: "project", pid: p.ID})
	if got := a.projects[0].Name; got != "新项目" {
		t.Fatalf("project name = %q", got)
	}

	// Rendering a row as an inline editor must not panic.
	a.startInlineRename(call.ID, "旧名字")
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Frame()

	// Escape/cancel drops the draft without touching the tree.
	a.startInlineRename(p.ID, "不要")
	a.renameInput = "不要"
	a.cancelInlineRename()
	if a.renamingID != "" || a.renameInput != "" {
		t.Fatal("cancel left rename state")
	}
	if got := a.projects[0].Name; got != "新项目" {
		t.Fatalf("cancel changed the name to %q", got)
	}
}

// TestInlineRenameViaUI drives the inline editor through the real view: focus
// the input, type a new title and press Enter.
func TestInlineRenameViaUI(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	svc, _ := a.st.AddService(p.ID, model.Service{Name: "S"})
	op, _ := a.st.AddOperation(p.ID, svc.ID, model.Operation{Name: "O"})
	call, _ := a.st.AddCall(p.ID, svc.ID, op.ID, model.Call{Title: "旧名字", Request: "<a/>"})
	a.reload()
	a.openCall(p.ID, svc.ID, op.ID, call.ID)
	a.startInlineRename(call.ID, "旧名字")

	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Frame()
	tt.Type("新名字")
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()

	if got := a.projects[0].Services[0].Operations[0].Calls[0].Title; got != "新名字" {
		t.Fatalf("call title = %q", got)
	}
}

func TestFormatXML(t *testing.T) {
	in := `<a><b>1</b><c x="1"/></a>`
	out, err := formatXML(in)
	if err != nil {
		t.Fatalf("formatXML: %v", err)
	}
	want := "<a>\n  <b>1</b>\n  <c x=\"1\"/>\n</a>"
	if out != want {
		t.Fatalf("formatXML = %q, want %q", out, want)
	}
}

// TestFormatXMLEmptyAndInline checks an empty <a></a> stays expanded, a source
// <b/> stays self-closing, and text-only elements stay on one line.
func TestFormatXMLEmptyAndInline(t *testing.T) {
	in := `<r><a></a><b/><c>t</c><d>  </d></r>`
	out, err := formatXML(in)
	if err != nil {
		t.Fatalf("formatXML: %v", err)
	}
	want := "<r>\n  <a></a>\n  <b/>\n  <c>t</c>\n  <d></d>\n</r>"
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

// TestFormatXMLPreservesEdgeCases checks the tokenizer keeps namespace
// prefixes, a ">" inside an attribute value and a CDATA section intact, which a
// regex tokenizer used to break.
func TestFormatXMLPreservesEdgeCases(t *testing.T) {
	in := `<soap:Envelope xmlns:soap="http://x" a="1>2"><soap:Body><![CDATA[ x > < y ]]></soap:Body></soap:Envelope>`
	out, err := formatXML(in)
	if err != nil {
		t.Fatalf("formatXML: %v", err)
	}
	for _, want := range []string{
		"<soap:Envelope",
		`a="1>2"`,
		"<![CDATA[ x > < y ]]>",
		"</soap:Envelope>",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("formatXML lost %q:\n%s", want, out)
		}
	}
}

// TestMinifyPreservesCDataAndText checks minify drops only structural
// whitespace and keeps the exact text and CDATA content.
func TestMinifyPreservesCDataAndText(t *testing.T) {
	a := newTestApp(t)
	a.xmlEd.Buffer.Reset("<a>\n  <b> hi </b>\n  <c><![CDATA[ x > < y ]]></c>\n</a>")
	a.minifyBody()
	want := `<a><b> hi </b><c><![CDATA[ x > < y ]]></c></a>`
	if got := a.xmlEd.Buffer.String(); got != want {
		t.Fatalf("minify = %q, want %q", got, want)
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

// TestResponseTextTabs checks what each response tab shows: formatted XML,
// the raw body, the request preview and the response headers.
func TestResponseTextTabs(t *testing.T) {
	a := newTestApp(t)
	a.hasResp = true
	a.resp = model.SendResponse{
		Status: 200, StatusText: "OK",
		Headers: map[string]string{"Content-Type": "text/xml"},
		Body:    "<a><b>1</b></a>",
	}
	a.endpoint = "http://example.test/soap"
	a.action = "urn:a"
	a.reqKind = model.KindSOAP
	a.headers = []headerRow{{Name: "X-Test", Value: "1"}}
	a.xmlEd.Buffer.Reset("<req/>")

	a.respTab = 0
	if got := a.responseText(); !strings.Contains(got, "\n") {
		t.Fatalf("formatted = %q", got)
	}
	a.respTab = 1
	if got := a.responseText(); got != "<a><b>1</b></a>" {
		t.Fatalf("raw = %q", got)
	}
	a.respTab = 2
	if got := a.responseText(); !strings.Contains(got, "POST http://example.test/soap") ||
		!strings.Contains(got, "<req/>") ||
		!strings.Contains(got, "SOAPAction: urn:a") {
		t.Fatalf("request preview = %q", got)
	}
	a.respTab = 3
	if got := a.responseText(); !strings.Contains(got, "Content-Type: text/xml") {
		t.Fatalf("headers = %q", got)
	}
}

// TestJSONHighlightAndResponseFormat checks that choosing the JSON body type
// gives the request editor JSON highlighting, and that a JSON response is
// pretty-printed on the 格式化 tab and highlighted as JSON.
func TestJSONHighlightAndResponseFormat(t *testing.T) {
	a := newTestApp(t)
	a.reqKind = model.KindHTTP
	a.bodyType = model.BodyNone
	a.contentType = ""
	a.setBodyType(model.BodyJSON)
	if lang := a.xmlEd.Buffer.Language(); lang == nil || lang.Name != "JSON" {
		t.Fatalf("request language = %v, want JSON", lang)
	}

	a.hasResp = true
	a.resp = model.SendResponse{
		Status: 200, StatusText: "OK",
		Headers: map[string]string{"Content-Type": "application/json; charset=utf-8"},
		Body:    `{"a":1,"b":[2,3]}`,
	}
	a.respTab = 0
	if got := a.responseText(); !strings.Contains(got, "\n") || !strings.Contains(got, `"a": 1`) {
		t.Fatalf("formatted JSON response = %q", got)
	}
	a.syncResponse()
	if lang := a.respEd.Buffer.Language(); lang == nil || lang.Name != "JSON" {
		t.Fatalf("response language = %v, want JSON", lang)
	}
	if got := a.respEd.Buffer.String(); !strings.Contains(got, "\n") {
		t.Fatalf("synced JSON response not formatted = %q", got)
	}
}

// TestInferBodyTypeSniffsBody checks that a legacy call with no stored body
// type and no telling Content-Type is classified by how its body is written.
func TestInferBodyTypeSniffsBody(t *testing.T) {
	cases := []struct{ ct, body, want string }{
		{"", `{"a":1}`, model.BodyJSON},
		{"", `[1,2]`, model.BodyJSON},
		{"", "<a/>", model.BodyXML},
		{"", "hello", model.BodyText},
		{"", "", model.BodyNone},
		{"application/json", "<weird/>", model.BodyJSON},
	}
	for _, c := range cases {
		if got := model.InferBodyType(c.ct, c.body); got != c.want {
			t.Errorf("InferBodyType(%q, %q) = %q, want %q", c.ct, c.body, got, c.want)
		}
	}
}

// TestResponseTabClickSwitchesEditor clicks the response tabs in a headless
// build of the real view and checks the editor follows the choice: the bug
// was that the tab value changed but the editor was never re-synced.
func TestResponseTabClickSwitchesEditor(t *testing.T) {
	a := newTestApp(t)
	a.hasResp = true
	a.resp = model.SendResponse{
		Status: 200, StatusText: "OK",
		Headers: map[string]string{"X-Test": "1"},
		Body:    "<a><b>1</b></a>",
	}
	a.endpoint = "http://example.test/soap"
	a.xmlEd.Buffer.Reset("<req/>")
	a.syncResponse()
	if got := a.respEd.Buffer.String(); got == "<a><b>1</b></a>" {
		t.Fatalf("precondition: formatted body should differ from raw, got %q", got)
	}

	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("原始"); err != nil {
		t.Fatalf("click 原始: %v", err)
	}
	tt.Frame()
	if a.respTab != 1 {
		t.Fatalf("respTab = %d, want 1", a.respTab)
	}
	if got := a.respEd.Buffer.String(); got != "<a><b>1</b></a>" {
		t.Fatalf("raw editor = %q", got)
	}

	if err := tt.Click("响应头"); err != nil {
		t.Fatalf("click 响应头: %v", err)
	}
	tt.Frame()
	if a.respTab != 3 {
		t.Fatalf("respTab = %d, want 3", a.respTab)
	}
	if got := a.respEd.Buffer.String(); !strings.Contains(got, "X-Test: 1") {
		t.Fatalf("headers editor = %q", got)
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
	a.action = "urn:changed"
	if !a.isDirty() {
		t.Fatal("edited SOAPAction is not dirty")
	}
	a.endpoint = "http://changed"
	if !a.isDirty() {
		t.Fatal("edited endpoint is not dirty")
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

// TestTreeSearchEmptyState checks that filtering to nothing explains it and
// offers a way to clear the search.
func TestTreeSearchEmptyState(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	a.search = "不存在的接口"

	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if !tt.HasText("没有匹配的接口") {
		t.Fatal("no empty search state")
	}
	if err := tt.Click("清空搜索"); err != nil {
		t.Fatalf("click 清空搜索: %v", err)
	}
	tt.Frame()
	if a.search != "" {
		t.Fatalf("search = %q, want cleared", a.search)
	}
}

// TestFriendlySendError checks transport errors become actionable Chinese
// messages while unknown errors pass through unchanged.
func TestFriendlySendError(t *testing.T) {
	cases := []struct{ in, want string }{
		{"dial tcp: lookup api.test: no such host", "无法解析服务器地址"},
		{"dial tcp 127.0.0.1:80: connect: connection refused", "服务器拒绝连接"},
		{"context deadline exceeded (Client.Timeout exceeded)", "请求超时"},
		{"x509: certificate signed by unknown authority", "TLS 证书校验失败"},
		{"some odd failure", "some odd failure"},
	}
	for _, c := range cases {
		if got := friendlySendError(c.in); !strings.Contains(got, c.want) {
			t.Errorf("friendlySendError(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestHasUnsavedTabs checks the quit guard sees edits and forgets them once
// saved.
func TestHasUnsavedTabs(t *testing.T) {
	a, _ := newNativeCall(t)
	if a.hasUnsavedTabs() {
		t.Fatal("a freshly opened request is dirty")
	}
	a.xmlEd.Buffer.Reset(`{"a":1}`)
	if !a.hasUnsavedTabs() {
		t.Fatal("edited request not seen as dirty")
	}
	a.saveCall()
	if a.hasUnsavedTabs() {
		t.Fatal("saved request still seen as dirty")
	}
}

// TestKeyboardAccessibleWidgets checks the custom clickable rows are in the
// accessibility tree as focusable controls with names, so keyboard and screen
// reader users can reach the tabs, tree rows and close button.
func TestKeyboardAccessibleWidgets(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	c1, _ := a.st.AddProjectCall(p.ID, "", model.Call{Title: "请求一", BodyType: model.BodyNone})
	a.reload()
	a.openCall(p.ID, "", "", c1.ID)

	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if info, ok := tt.AccessByRole(ui.RoleTab, "请求一"); !ok || !info.Focusable || !info.Selected {
		t.Fatalf("tab a11y = %+v ok=%v, want focusable selected tab named 请求一", info, ok)
	}
	if info, ok := tt.Access("请求一"); !ok || !info.Focusable {
		t.Fatalf("tree row a11y = %+v ok=%v, want focusable", info, ok)
	}
	if info, ok := tt.Access("关闭标签"); !ok || !info.Focusable {
		t.Fatalf("close button a11y = %+v ok=%v, want focusable", info, ok)
	}
}

// TestTabsPersistAcrossRestart checks the open tabs and the active one come
// back after reopening the workspace, that a tab whose request was deleted is
// dropped, and that closing every tab leaves the next run empty.
func TestTabsPersistAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	reg, err := workspace.Open(dir, "")
	if err != nil {
		t.Fatalf("workspace.Open: %v", err)
	}
	a := newApp(reg)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	c1, _ := a.st.AddProjectCall(p.ID, "", model.Call{Title: "C1", BodyType: model.BodyNone})
	c2, _ := a.st.AddProjectCall(p.ID, "", model.Call{Title: "C2", BodyType: model.BodyNone})
	a.reload()
	a.openCall(p.ID, "", "", c1.ID)
	a.openCall(p.ID, "", "", c2.ID) // C2 is now the active tab.
	a.saveUIState()

	reg2, _ := workspace.Open(dir, "")
	b := newApp(reg2)
	if len(b.tabs) != 2 || b.active != 1 {
		t.Fatalf("restored tabs=%d active=%d, want 2 and 1", len(b.tabs), b.active)
	}
	if b.cid != c2.ID {
		t.Fatalf("restored active call = %q, want %q", b.cid, c2.ID)
	}

	// Deleting a request drops its tab on the next start even though the saved
	// state still names it, then clamps the active index.
	if err := b.st.DeleteProjectCall(p.ID, c2.ID); err != nil {
		t.Fatalf("delete C2: %v", err)
	}
	reg3, _ := workspace.Open(dir, "")
	c := newApp(reg3)
	if len(c.tabs) != 1 || c.cid != c1.ID {
		t.Fatalf("after delete: tabs=%d cid=%q, want 1 and %q", len(c.tabs), c.cid, c1.ID)
	}

	// Closing every tab is remembered too: the next run shows an empty editor
	// rather than reopening the first request.
	c.tabs = nil
	c.active = 0
	c.clearSelection()
	c.saveUIState()
	reg4, _ := workspace.Open(dir, "")
	d := newApp(reg4)
	if len(d.tabs) != 0 {
		t.Fatalf("closed tabs came back: %v", d.tabs)
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
	// Use fonts this platform's dropdown actually offers, so the choices
	// survive normalization on both macOS and Windows.
	uiFont, editorFont := secondChoice(uiFontOptions), secondChoice(editorFontOptions)
	a.uiFontChoice = uiFont
	a.uiFontSize = 16
	a.editorFontChoice = editorFont
	a.editorFontSize = 18
	a.accent = "#10b981"
	a.saveUIState()

	reg2, err := workspace.Open(dir, "")
	if err != nil {
		t.Fatalf("workspace.Open 2: %v", err)
	}
	b := newApp(reg2)
	if b.dark != false || b.splitSide != 1 || b.sideW != 333 || b.splitV != 55 || b.splitH != 44 {
		t.Fatalf("restored = %+v", b.currentUI())
	}
	if b.uiFontChoice != uiFont || b.uiFontSize != 16 ||
		b.editorFontChoice != editorFont || b.editorFontSize != 18 {
		t.Fatalf("restored fonts = %q %v %q %v", b.uiFontChoice, b.uiFontSize, b.editorFontChoice, b.editorFontSize)
	}
	if b.accent != "#10b981" {
		t.Fatalf("restored accent = %q", b.accent)
	}
}

// TestAccentSettingApplies checks the picked theme color reaches the accent
// fields used by POST tags, primary buttons and splitters, in both modes, and
// that resetting brings the default purple back.
func TestAccentSettingApplies(t *testing.T) {
	a := newTestApp(t)
	a.setAccent("#10b981")
	for _, dark := range []bool{false, true} {
		a.dark = dark
		if got := a.theme().Accent; got != ui.Hex("#10b981") {
			t.Fatalf("dark=%v accent = %v, want #10b981", dark, got)
		}
	}
	// An unknown color is ignored rather than written into the theme.
	a.setAccent("#zzzzzz")
	if a.accent != "#10b981" {
		t.Fatalf("invalid accent changed the choice to %q", a.accent)
	}
	a.setAccent("")
	if a.accent != "" {
		t.Fatalf("reset left accent = %q", a.accent)
	}
	if got := a.theme().Accent; got != ui.Hex(defaultAccent) {
		t.Fatalf("default accent = %v, want %v", got, ui.Hex(defaultAccent))
	}
}

// TestFontSettingsApply checks the interface and editor font settings take
// effect separately: the theme's size and the scaled view sizes follow the
// interface setting, and the editor states follow the editor setting.
func TestFontSettingsApply(t *testing.T) {
	a := newTestApp(t)
	a.uiFontSize = 18
	editorFont := secondChoice(editorFontOptions)
	a.editorFontChoice = editorFont
	a.editorFontSize = 20

	if got := a.theme().FontSize; got != 18 {
		t.Fatalf("theme FontSize = %v, want 18", got)
	}
	if fontFamily(systemFontLabel) != "" {
		t.Fatalf("system font choice = %q, want empty family", fontFamily(systemFontLabel))
	}
	want := 10 * float32(18/uiBaseFontSize())
	if got := a.us(10); got != want {
		t.Fatalf("us(10) = %v, want %v", got, want)
	}
	// At the default size the view's literals are unchanged.
	a.uiFontSize = uiBaseFontSize()
	if got := a.us(10); got != 10 {
		t.Fatalf("us(10) at default = %v, want 10", got)
	}

	a.uiFontSize = 18
	a.syncEditorFont()
	if a.xmlEd.Font.Size != 20 || a.xmlEd.Font.Family != editorFont {
		t.Fatalf("xmlEd font = %+v", a.xmlEd.Font)
	}
	if a.respEd.Font.Size != 20 || a.respEd.Font.Family != editorFont {
		t.Fatalf("respEd font = %+v", a.respEd.Font)
	}

	// The settings dialog must build without panicking, in both sections.
	a.settingsOpen = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Frame()
	a.settingsSection = 1
	tt.Frame()
	tt.Frame()
}

// TestTopBarMenus checks the file menu is gone, the "more" menu beside "+"
// holds import and SoapUI export, and the workspace menu holds workspace export.
func TestTopBarMenus(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()

	if tt.HasText("文件") {
		t.Fatal("top bar still shows 文件")
	}

	if err := tt.Click("更多"); err != nil {
		t.Fatalf("click 更多: %v", err)
	}
	tt.Frame()
	items := strings.Join(tt.Menu(), ",")
	// A native (plain HTTP) project must not offer the SoapUI export.
	if !strings.Contains(items, "导入…") {
		t.Fatalf("more menu = %v", tt.Menu())
	}
	if strings.Contains(items, "导出 SoapUI XML…") {
		t.Fatalf("native project must not offer SoapUI export: %v", tt.Menu())
	}
	tt.CloseMenu()
	tt.Frame()

	// A SoapUI-sourced project does offer the export.
	a.createProject("S")
	a.reload()
	a.projects[1].Source = "soapui"
	a.pid = a.projects[1].ID
	tt.Frame()
	if err := tt.Click("更多"); err != nil {
		t.Fatalf("click 更多: %v", err)
	}
	tt.Frame()
	if !strings.Contains(strings.Join(tt.Menu(), ","), "导出 SoapUI XML…") {
		t.Fatalf("soap project more menu = %v", tt.Menu())
	}
	tt.CloseMenu()
	tt.Frame()

	if err := tt.Click("工作空间 · 默认"); err != nil {
		t.Fatalf("click workspace menu: %v", err)
	}
	tt.Frame()
	if !strings.Contains(strings.Join(tt.Menu(), ","), "导出工作空间…") {
		t.Fatalf("workspace menu = %v", tt.Menu())
	}
}

// TestTabDirtyDot checks the active tab shows a dot only while the request has
// unsaved changes.
func TestTabDirtyDot(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	svc, _ := a.st.AddService(p.ID, model.Service{Name: "S"})
	op, _ := a.st.AddOperation(p.ID, svc.ID, model.Operation{Name: "O"})
	call, _ := a.st.AddCall(p.ID, svc.ID, op.ID, model.Call{Title: "c", Request: "<a/>"})
	a.reload()
	a.openCall(p.ID, svc.ID, op.ID, call.ID)

	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if tt.HasText("●") {
		t.Fatal("fresh tab shows a dirty dot")
	}

	a.xmlEd.Buffer.Reset("<b/>")
	tt.Frame()
	if !tt.HasText("●") {
		t.Fatal("dirty tab is missing the dot")
	}

	a.saveCall()
	tt.Frame()
	if tt.HasText("●") {
		t.Fatal("saved tab still shows the dot")
	}
}

// TestContextMenuRightClickKeepsSelection checks that opening a node's context
// menu does not move the selection, so a following save cannot overwrite the
// right-clicked call with the editor's other call.
func TestContextMenuRightClickKeepsSelection(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	s, _ := a.st.AddService(p.ID, model.Service{Name: "S"})
	o, _ := a.st.AddOperation(p.ID, s.ID, model.Operation{Name: "O"})
	c1, _ := a.st.AddCall(p.ID, s.ID, o.ID, model.Call{Title: "C1", Request: "<first/>"})
	c2, _ := a.st.AddCall(p.ID, s.ID, o.ID, model.Call{Title: "C2", Request: "<second/>"})
	a.reload()
	a.openCall(p.ID, s.ID, o.ID, c1.ID)

	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Frame()
	if err := tt.RightClick("C2"); err != nil {
		t.Fatalf("right-click C2: %v", err)
	}
	tt.Frame()
	tt.CloseMenu()

	if a.cid != c1.ID {
		t.Fatalf("right-click moved selection to %q, want %q", a.cid, c1.ID)
	}
	if got := a.xmlEd.Buffer.String(); got != "<first/>" {
		t.Fatalf("right-click changed the editor: %q", got)
	}

	// Saving after a right-click must keep C1 and leave C2 untouched.
	a.saveCall()
	got, _ := a.st.Get(p.ID)
	for _, c := range got.Services[0].Operations[0].Calls {
		switch c.ID {
		case c1.ID:
			if c.Request != "<first/>" {
				t.Fatalf("C1 request changed to %q", c.Request)
			}
		case c2.ID:
			if c.Request != "<second/>" {
				t.Fatalf("C2 overwritten with %q", c.Request)
			}
		}
	}
}

// TestSendRequiresSelectedCall checks Send refuses when no request sample is
// open, instead of sending the editor's previous payload.
func TestSendRequiresSelectedCall(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	s, _ := a.st.AddService(p.ID, model.Service{Name: "S"})
	o, _ := a.st.AddOperation(p.ID, s.ID, model.Operation{Name: "O", Endpoint: "http://example.test/soap"})
	c, _ := a.st.AddCall(p.ID, s.ID, o.ID, model.Call{Title: "C", Endpoint: "http://example.test/soap", Request: "<a/>"})
	a.reload()
	a.openCall(p.ID, s.ID, o.ID, c.ID)
	// No open tab: the tree selection alone must not authorise a send.
	a.clearSelection()

	a.pid, a.sid, a.oid, a.cid = p.ID, "", "", ""
	if _, _, err := a.buildSendRequest(); err == nil {
		t.Fatal("Send allowed with only a project selected")
	}
	a.pid, a.sid, a.oid, a.cid = p.ID, s.ID, "", ""
	if _, _, err := a.buildSendRequest(); err == nil {
		t.Fatal("Send allowed with only a service selected")
	}
	a.openCall(p.ID, s.ID, o.ID, c.ID)
	if _, _, err := a.buildSendRequest(); err != nil {
		t.Fatalf("valid selection rejected: %v", err)
	}
}

// TestTabSwitchKeepsDraft checks that switching tabs keeps each tab's unsaved
// edits and that a background tab still reports dirty.
func TestTabSwitchKeepsDraft(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	s, _ := a.st.AddService(p.ID, model.Service{Name: "S"})
	o, _ := a.st.AddOperation(p.ID, s.ID, model.Operation{Name: "O"})
	c1, _ := a.st.AddCall(p.ID, s.ID, o.ID, model.Call{Title: "C1", Request: "<a/>"})
	c2, _ := a.st.AddCall(p.ID, s.ID, o.ID, model.Call{Title: "C2", Request: "<b/>"})
	a.reload()

	a.openCall(p.ID, s.ID, o.ID, c1.ID)
	a.xmlEd.Buffer.Reset("<edited/>")
	a.openCall(p.ID, s.ID, o.ID, c2.ID)
	if got := a.xmlEd.Buffer.String(); got != "<b/>" {
		t.Fatalf("C2 body = %q", got)
	}
	if !a.dirtyKey(callKey(p.ID, s.ID, o.ID, c1.ID)) {
		t.Fatal("inactive tab lost its dirty state")
	}

	a.openCall(p.ID, s.ID, o.ID, c1.ID)
	if got := a.xmlEd.Buffer.String(); got != "<edited/>" {
		t.Fatalf("unsaved edit lost on tab switch: %q", got)
	}
	if !a.isDirty() {
		t.Fatal("restored draft is not dirty")
	}
}

// TestTabCloseButtonCloses checks the tab strip's close button closes that tab
// rather than the click falling through to the tab row and opening it.
func TestTabCloseButtonCloses(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	c1, _ := a.st.AddProjectCall(p.ID, "", model.Call{Title: "C1", BodyType: model.BodyNone})
	c2, _ := a.st.AddProjectCall(p.ID, "", model.Call{Title: "C2", BodyType: model.BodyNone})
	a.reload()
	a.openCall(p.ID, "", "", c1.ID)
	a.openCall(p.ID, "", "", c2.ID)

	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("关闭标签"); err != nil {
		t.Fatalf("click close: %v", err)
	}
	tt.Frame()
	if len(a.tabs) != 1 || a.cid != c2.ID {
		t.Fatalf("after close: tabs=%d cid=%q, want tabs=1 cid=%q", len(a.tabs), a.cid, c2.ID)
	}
}

// TestDirtyDotOnInactiveTab checks the dot shows on a tab edited in the
// background, not only on the active tab.
func TestDirtyDotOnInactiveTab(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	s, _ := a.st.AddService(p.ID, model.Service{Name: "S"})
	o, _ := a.st.AddOperation(p.ID, s.ID, model.Operation{Name: "O"})
	c1, _ := a.st.AddCall(p.ID, s.ID, o.ID, model.Call{Title: "C1", Request: "<a/>"})
	c2, _ := a.st.AddCall(p.ID, s.ID, o.ID, model.Call{Title: "C2", Request: "<b/>"})
	a.reload()
	a.openCall(p.ID, s.ID, o.ID, c1.ID)
	a.xmlEd.Buffer.Reset("<edited/>")
	a.openCall(p.ID, s.ID, o.ID, c2.ID)

	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if !tt.HasText("●") {
		t.Fatal("inactive dirty tab shows no dot")
	}
}

// TestCloseDirtyTabAsks checks closing a dirty tab asks first and only then
// drops it.
func TestCloseDirtyTabAsks(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	s, _ := a.st.AddService(p.ID, model.Service{Name: "S"})
	o, _ := a.st.AddOperation(p.ID, s.ID, model.Operation{Name: "O"})
	c1, _ := a.st.AddCall(p.ID, s.ID, o.ID, model.Call{Title: "C1", Request: "<a/>"})
	a.reload()
	a.openCall(p.ID, s.ID, o.ID, c1.ID)
	a.xmlEd.Buffer.Reset("<edited/>")

	a.closeTab(0)
	if !a.confirmOpen {
		t.Fatal("closing a dirty tab did not ask for confirmation")
	}
	if len(a.tabs) != 1 {
		t.Fatalf("tab closed before confirming: tabs=%d", len(a.tabs))
	}
	fn := a.confirmAction
	a.confirmAction = nil
	a.confirmOpen = false
	fn()
	if len(a.tabs) != 0 {
		t.Fatalf("tab not closed after confirming: tabs=%d", len(a.tabs))
	}
}

// TestDeleteServiceKeepsOtherTabs checks deleting a service closes only its own
// tabs.
func TestDeleteServiceKeepsOtherTabs(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	s1, _ := a.st.AddService(p.ID, model.Service{Name: "S1"})
	s2, _ := a.st.AddService(p.ID, model.Service{Name: "S2"})
	o1, _ := a.st.AddOperation(p.ID, s1.ID, model.Operation{Name: "O1"})
	o2, _ := a.st.AddOperation(p.ID, s2.ID, model.Operation{Name: "O2"})
	c1, _ := a.st.AddCall(p.ID, s1.ID, o1.ID, model.Call{Title: "C1", Request: "<a/>"})
	c2, _ := a.st.AddCall(p.ID, s2.ID, o2.ID, model.Call{Title: "C2", Request: "<b/>"})
	a.reload()
	a.openCall(p.ID, s1.ID, o1.ID, c1.ID)
	a.openCall(p.ID, s2.ID, o2.ID, c2.ID)

	a.pid, a.sid, a.oid, a.cid = p.ID, s1.ID, "", ""
	a.deleteService()
	fn := a.confirmAction
	a.confirmAction = nil
	a.confirmOpen = false
	fn()

	if len(a.tabs) != 1 || a.cid != c2.ID {
		t.Fatalf("after deleting S1: tabs=%d cid=%q, want tabs=1 cid=%q", len(a.tabs), a.cid, c2.ID)
	}
	if got := a.xmlEd.Buffer.String(); got != "<b/>" {
		t.Fatalf("active tab content = %q", got)
	}
}

// TestDeleteUnopenedCallKeepsTabs checks deleting a call that is not open does
// not disturb the open tab.
func TestDeleteUnopenedCallKeepsTabs(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	s, _ := a.st.AddService(p.ID, model.Service{Name: "S"})
	o, _ := a.st.AddOperation(p.ID, s.ID, model.Operation{Name: "O"})
	c1, _ := a.st.AddCall(p.ID, s.ID, o.ID, model.Call{Title: "C1", Request: "<a/>"})
	c2, _ := a.st.AddCall(p.ID, s.ID, o.ID, model.Call{Title: "C2", Request: "<b/>"})
	a.reload()
	a.openCall(p.ID, s.ID, o.ID, c2.ID)

	a.pid, a.sid, a.oid, a.cid = p.ID, s.ID, o.ID, c1.ID
	a.deleteCall()
	fn := a.confirmAction
	a.confirmAction = nil
	a.confirmOpen = false
	fn()

	if len(a.tabs) != 1 || a.cid != c2.ID {
		t.Fatalf("after deleting unopened C1: tabs=%d cid=%q, want tabs=1 cid=%q", len(a.tabs), a.cid, c2.ID)
	}
	if got := a.xmlEd.Buffer.String(); got != "<b/>" {
		t.Fatalf("open tab content = %q", got)
	}
}

// TestDeleteProjectKeepsOtherProjectTabs checks deleting one project keeps the
// other project's tabs.
func TestDeleteProjectKeepsOtherProjectTabs(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P1")
	a.createProject("P2")
	a.reload()
	p1, p2 := a.projects[0], a.projects[1]
	s1, _ := a.st.AddService(p1.ID, model.Service{Name: "S1"})
	o1, _ := a.st.AddOperation(p1.ID, s1.ID, model.Operation{Name: "O1"})
	c1, _ := a.st.AddCall(p1.ID, s1.ID, o1.ID, model.Call{Title: "C1", Request: "<a/>"})
	s2, _ := a.st.AddService(p2.ID, model.Service{Name: "S2"})
	o2, _ := a.st.AddOperation(p2.ID, s2.ID, model.Operation{Name: "O2"})
	c2, _ := a.st.AddCall(p2.ID, s2.ID, o2.ID, model.Call{Title: "C2", Request: "<b/>"})
	a.reload()
	a.openCall(p1.ID, s1.ID, o1.ID, c1.ID)
	a.openCall(p2.ID, s2.ID, o2.ID, c2.ID)

	a.pid, a.sid, a.oid, a.cid = p1.ID, "", "", ""
	a.deleteProject()
	fn := a.confirmAction
	a.confirmAction = nil
	a.confirmOpen = false
	fn()

	if len(a.tabs) != 1 || a.cid != c2.ID {
		t.Fatalf("after deleting P1: tabs=%d cid=%q, want tabs=1 cid=%q", len(a.tabs), a.cid, c2.ID)
	}
}

// TestSettingsSizeStepper checks the minus and plus buttons of the font-size
// control step the value by one and stop at the bounds.
func TestSettingsSizeStepper(t *testing.T) {
	a := newTestApp(t)
	a.settingsOpen = true
	a.settingsSection = 0
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()

	start := a.uiFontSize
	if err := tt.Click("−"); err != nil {
		t.Fatalf("click minus: %v", err)
	}
	tt.Frame()
	if a.uiFontSize != start-1 {
		t.Fatalf("after minus = %v, want %v", a.uiFontSize, start-1)
	}
	if err := tt.Click("+"); err != nil {
		t.Fatalf("click plus: %v", err)
	}
	tt.Frame()
	if a.uiFontSize != start {
		t.Fatalf("after plus = %v, want %v", a.uiFontSize, start)
	}
}

// newCallForAuth builds one project/service/operation/call and opens the call,
// for the authentication tests.
func newCallForAuth(t *testing.T) (*app, model.Call) {
	t.Helper()
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	svc, err := a.st.AddService(p.ID, model.Service{Name: "S"})
	if err != nil {
		t.Fatalf("AddService: %v", err)
	}
	op, err := a.st.AddOperation(p.ID, svc.ID, model.Operation{Name: "O", Endpoint: "http://x/Op"})
	if err != nil {
		t.Fatalf("AddOperation: %v", err)
	}
	call, err := a.st.AddCall(p.ID, svc.ID, op.ID, model.Call{Title: "C", Endpoint: "http://x/Op", Request: "<a/>"})
	if err != nil {
		t.Fatalf("AddCall: %v", err)
	}
	a.reload()
	a.openCall(p.ID, svc.ID, op.ID, call.ID)
	return a, call
}

// TestAuthHeaderInSendRequest checks the auth page produces the Basic/Bearer
// header, that it wins over a hand-written Authorization, and that a disabled
// or absent configuration leaves the manual header alone.
func TestAuthHeaderInSendRequest(t *testing.T) {
	a, _ := newCallForAuth(t)
	a.headers = []headerRow{{Name: "Authorization", Value: "manual"}}

	a.setAuthType("basic")
	a.auth.Username, a.auth.Password = "u", "p"
	req, _, err := a.buildSendRequest()
	if err != nil {
		t.Fatalf("buildSendRequest: %v", err)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("u:p"))
	if got := req.Headers["Authorization"]; got != want {
		t.Fatalf("basic Authorization = %q, want %q", got, want)
	}

	a.setAuthType("bearer")
	a.auth.Token = "tok"
	req, _, err = a.buildSendRequest()
	if err != nil {
		t.Fatalf("buildSendRequest: %v", err)
	}
	if got := req.Headers["Authorization"]; got != "Bearer tok" {
		t.Fatalf("bearer Authorization = %q, want %q", got, "Bearer tok")
	}
	a.auth.Prefix = "Token"
	req, _, _ = a.buildSendRequest()
	if got := req.Headers["Authorization"]; got != "Token tok" {
		t.Fatalf("prefixed Authorization = %q", got)
	}

	a.auth.Enabled = false
	req, _, _ = a.buildSendRequest()
	if got := req.Headers["Authorization"]; got != "manual" {
		t.Fatalf("disabled auth should leave manual header, got %q", got)
	}

	a.setAuthType("none")
	req, _, _ = a.buildSendRequest()
	if got := req.Headers["Authorization"]; got != "manual" {
		t.Fatalf("no auth should leave manual header, got %q", got)
	}
}

// TestAuthPersistsAndDirty checks the auth form saves with the sample, loads
// back, and marks the tab dirty while it differs from the saved copy.
func TestAuthPersistsAndDirty(t *testing.T) {
	a, call := newCallForAuth(t)

	a.auth = authForm{Type: "basic", Enabled: true, Username: "u", Password: "p"}
	if !a.currentDirty() {
		t.Fatal("editing auth did not mark the tab dirty")
	}
	a.saveCall()
	if a.currentDirty() {
		t.Fatal("saved tab still dirty")
	}

	a.reload()
	c := a.projects[0].Services[0].Operations[0].Calls[0]
	if c.ID != call.ID || c.Auth == nil || c.Auth.Type != "basic" ||
		c.Auth.Username != "u" || c.Auth.Password != "p" || !c.Auth.Enabled {
		t.Fatalf("stored auth = %+v", c.Auth)
	}
}

// TestAuthPanelRenders checks the 认证 page builds in each type without
// panicking and shows the scheme's fields.
func TestAuthPanelRenders(t *testing.T) {
	a, _ := newCallForAuth(t)
	a.reqSection = 2
	a.setAuthType("basic")
	a.auth.Username, a.auth.Password = "u", "p"
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if !tt.HasText("Username") || !tt.HasText("Password") {
		t.Fatal("basic auth page is missing its fields")
	}
	a.setAuthType("bearer")
	tt.Frame()
	if !tt.HasText("Token") || !tt.HasText("Prefix") {
		t.Fatal("bearer auth page is missing its fields")
	}
	a.setAuthType("none")
	tt.Frame()
	if tt.HasText("Username") {
		t.Fatal("no-auth page still shows credential fields")
	}
}

// newNativeCall builds a native project with one HTTP request and opens it.
func newNativeCall(t *testing.T) (*app, model.Call) {
	t.Helper()
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	call, err := a.st.AddProjectCall(p.ID, "", model.Call{
		Title: "R", Endpoint: "http://example.test/api", Method: "GET",
	})
	if err != nil {
		t.Fatalf("AddProjectCall: %v", err)
	}
	a.reload()
	a.openCall(p.ID, "", "", call.ID)
	return a, call
}

// TestNativeHTTPBuild checks a native request builds with its own method, no
// SOAPAction, no content type for a bodyless GET, and a JSON body for POST.
func TestNativeHTTPBuild(t *testing.T) {
	a, _ := newNativeCall(t)
	if a.reqKind != model.KindHTTP {
		t.Fatalf("reqKind = %q, want http", a.reqKind)
	}
	if a.bodyType != model.BodyNone {
		t.Fatalf("a new GET request should default to No Body, got %q", a.bodyType)
	}

	a.method = "GET"
	a.xmlEd.Buffer.Reset(`{"q":1}`)
	req, _, err := a.buildSendRequest()
	if err != nil {
		t.Fatalf("buildSendRequest: %v", err)
	}
	if req.Method != "GET" || req.Body != "" {
		t.Fatalf("GET req = %+v, want no body", req)
	}
	if _, ok := req.Headers["SOAPAction"]; ok {
		t.Fatalf("HTTP request must not set SOAPAction: %v", req.Headers)
	}
	if _, ok := req.Headers["Content-Type"]; ok || req.ContentType != "" {
		t.Fatalf("bodyless GET must not carry a Content-Type: %+v", req.Headers)
	}

	a.method = "POST"
	a.setBodyType(model.BodyJSON)
	req, _, _ = a.buildSendRequest()
	if req.Method != "POST" || req.Body != `{"q":1}` {
		t.Fatalf("POST req = %+v", req)
	}
	if req.ContentType != "application/json" {
		t.Fatalf("POST content type = %q", req.ContentType)
	}

	a.saveCall()
	a.reload()
	if got := a.projects[0].Calls[0]; got.Kind != model.KindHTTP || got.Method != "POST" || got.BodyType != model.BodyJSON {
		t.Fatalf("stored call = %+v", got)
	}
}

// TestJSONFormat checks the JSON branch of format/minify.
func TestJSONFormat(t *testing.T) {
	a, _ := newNativeCall(t)
	a.setBodyType(model.BodyJSON)
	a.xmlEd.Buffer.Reset(`{"a":1,"b":[2,3]}`)

	a.formatBody()
	if got := a.xmlEd.Buffer.String(); !strings.Contains(got, "\n") || !strings.Contains(got, `"a": 1`) {
		t.Fatalf("formatted json = %q", got)
	}
	a.minifyBody()
	if got := a.xmlEd.Buffer.String(); got != `{"a":1,"b":[2,3]}` {
		t.Fatalf("minified json = %q", got)
	}
}

// TestQueryTabSync checks the 查询 rows are parsed from the URL and edits are
// written back to it.
func TestQueryTabSync(t *testing.T) {
	a, _ := newNativeCall(t)
	a.endpoint = "http://example.test/api?a=1&b=2"
	a.querySrc = "" // force a re-parse on the next frame

	a.reqSection = 1 // 报文, 查询, ...
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if len(a.queryRows) != 2 || a.queryRows[0].Key != "a" || a.queryRows[1].Value != "2" {
		t.Fatalf("parsed rows = %+v", a.queryRows)
	}

	// Editing a row writes the URL.
	a.queryRows[1].Value = "9"
	tt.Frame()
	if !strings.Contains(a.endpoint, "b=9") || !strings.Contains(a.endpoint, "a=1") {
		t.Fatalf("endpoint = %q", a.endpoint)
	}
}

// TestNativeExportRefused checks both the app entry point and the library
// reject a native project.
func TestNativeExportRefused(t *testing.T) {
	a, _ := newNativeCall(t)
	path := filepath.Join(t.TempDir(), "out.xml")
	a.exportProject(path)
	if a.errMsg == "" {
		t.Fatal("native export should have been refused")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("native export must not write a file")
	}
	if _, err := soapuiproj.Marshal(a.projects[0]); err == nil {
		t.Fatal("soapuiproj.Marshal must refuse a native project")
	}
}

// TestBodyTypeRequestBodies checks each plain-HTTP body shape produces the
// right body and content type.
func TestBodyTypeRequestBodies(t *testing.T) {
	a, _ := newNativeCall(t)
	a.method = "POST"

	a.setBodyType(model.BodyURLEncoded)
	a.bodyFields = []bodyField{{Name: "a", Value: "1"}, {Name: "b", Value: "x y"}}
	req, _, err := a.buildSendRequest()
	if err != nil {
		t.Fatalf("urlencoded build: %v", err)
	}
	if req.Body != "a=1&b=x+y" || req.ContentType != "application/x-www-form-urlencoded" {
		t.Fatalf("urlencoded = %q ct=%q", req.Body, req.ContentType)
	}

	a.saveCall()
	a.reload()
	if got := a.projects[0].Calls[0]; got.BodyType != model.BodyURLEncoded || len(got.BodyFields) != 2 {
		t.Fatalf("stored body = %+v", got)
	}

	a.setBodyType(model.BodyMultipart)
	req, _, _ = a.buildSendRequest()
	if !strings.HasPrefix(req.ContentType, "multipart/form-data; boundary=") ||
		!strings.Contains(req.Body, `name="a"`) {
		t.Fatalf("multipart = ct %q body %q", req.ContentType, req.Body)
	}

	a.setBodyType(model.BodyNone)
	req, _, _ = a.buildSendRequest()
	if req.Body != "" || req.ContentType != "" {
		t.Fatalf("none = body %q ct %q", req.Body, req.ContentType)
	}

	f := filepath.Join(t.TempDir(), "blob.bin")
	if err := os.WriteFile(f, []byte{0, 1, 2, 3}, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	a.setBodyType(model.BodyBinary)
	a.bodyFile = f
	req, _, err = a.buildSendRequest()
	if err != nil {
		t.Fatalf("binary build: %v", err)
	}
	if req.Body != string([]byte{0, 1, 2, 3}) || req.ContentType != "application/octet-stream" {
		t.Fatalf("binary = %q ct %q", req.Body, req.ContentType)
	}
}

// TestGroupTreeRenders checks a folder and a request inside it appear in the
// tree.
func TestGroupTreeRenders(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	g, err := a.st.AddGroup(p.ID, "我的目录", "")
	if err != nil {
		t.Fatalf("AddGroup: %v", err)
	}
	if _, err := a.st.AddProjectCall(p.ID, g.ID, model.Call{Title: "目录内请求", Method: "POST"}); err != nil {
		t.Fatalf("AddProjectCall: %v", err)
	}
	a.reload()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if !tt.HasText("我的目录") || !tt.HasText("目录内请求") {
		t.Fatal("group tree did not render its folder and request")
	}
}

// TestTitleBarPadClearsTrafficLights pins the top bar's inner padding: on
// macOS the window controls (the traffic lights) occupy bar.Left pixels from
// the left edge and bar.Bottom down from the top, so the content must start
// to their right or the app icon overlaps them. Platforms with no reserved
// space on the left keep the ordinary left pad.
func TestTitleBarPadClearsTrafficLights(t *testing.T) {
	// macOS: traffic lights from x=0 to about x=78, bottom at y≈28.
	mac := ui.TitleBar{Height: 28, Left: 78, Bottom: 28}
	top, right, bottom, left := titleBarPad(mac, 14)
	if left <= mac.Left {
		t.Fatalf("left pad = %v, want > traffic-light span %v", left, mac.Left)
	}
	if top != 0 || bottom != 0 {
		t.Fatalf("top/bottom pad = %v/%v, want 0/0", top, bottom)
	}
	if right <= 0 {
		t.Fatalf("right pad = %v, want > 0", right)
	}

	// Windows / Linux: no room reserved, so the plain left pad stays.
	win := ui.TitleBar{}
	_, _, _, wleft := titleBarPad(win, 14)
	if wleft != 14 {
		t.Fatalf("left pad = %v, want 14", wleft)
	}
}

// TestTopBarClearsTrafficLightsInFrame renders the real top bar under a
// macOS-style title bar (traffic lights from the left edge to x≈78) and checks
// the app icon ("S" mark) starts to their right instead of overlapping them.
func TestTopBarClearsTrafficLightsInFrame(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 1200, 800)
	mac := ui.TitleBar{Height: 28, Left: 78, Bottom: 28}
	tt.SetTitleBar(mac)
	tt.Frame()

	box, ok := tt.Find("S")
	if !ok {
		t.Fatal("app icon mark not found in the top bar")
	}
	// The traffic lights reach mac.Left from the left edge; the icon must sit
	// past them (with the small gap titleBarPad adds), not under them.
	if box.X < mac.Left {
		t.Fatalf("icon x = %.1f, want >= traffic-light span %.1f (icon overlaps the traffic lights)", box.X, mac.Left)
	}
}

// TestTrafficLightPositionCentered checks macOS gets a traffic-light position
// that vertically centers the lights in the top bar (aligned with the right
// icons) rather than hugging the top-left corner, and other platforms get none.
func TestTrafficLightPositionCentered(t *testing.T) {
	pos := trafficLightPosition()
	if runtime.GOOS != "darwin" {
		if pos != nil {
			t.Fatalf("non-darwin got traffic light position %+v, want nil", pos)
		}
		return
	}
	if pos == nil {
		t.Fatal("darwin got nil traffic light position")
	}
	// The lights' vertical center (top margin + half the button) should sit at
	// the middle of the bar, where the right-side icons are centered.
	center := float64(pos.Y) + float64(macTrafficLightButton)/2
	if want := float64(topBarHeight) / 2; center != want {
		t.Fatalf("traffic light center y = %.1f, want %.1f (centered in the %.0fpx bar)", center, want, float64(topBarHeight))
	}
	if pos.X <= 0 {
		t.Fatalf("traffic light left margin = %d, want > 0", pos.X)
	}
}

// secondChoice returns the first option after the system default, a real font
// family this platform offers, for tests that need a non-default choice.
func secondChoice(options []string) string {
	if len(options) < 2 {
		return systemFontLabel
	}
	return options[1]
}

// TestFontOptionsPerPlatform checks the settings dialog offers a non-trivial
// font list whose first entry is the system default, that a stored family the
// current platform does not offer (such as one saved on another OS) falls back
// to the system font, and that offered families survive normalization.
func TestFontOptionsPerPlatform(t *testing.T) {
	if len(uiFontOptions) < 3 || len(editorFontOptions) < 3 {
		t.Fatalf("font options too short: ui=%v editor=%v", uiFontOptions, editorFontOptions)
	}
	if uiFontOptions[0] != systemFontLabel || editorFontOptions[0] != systemFontLabel {
		t.Fatalf("first option must be the system default: ui=%q editor=%q", uiFontOptions[0], editorFontOptions[0])
	}
	// A family from the other platform's list is not offered here.
	foreign := "PingFang SC"
	if runtime.GOOS == "darwin" {
		foreign = "Microsoft YaHei UI"
	}
	if got := normalizeFontChoice(foreign, uiFontOptions); got != systemFontLabel {
		t.Fatalf("foreign font %q normalized to %q, want system default", foreign, got)
	}
	// An offered family is kept.
	offered := secondChoice(editorFontOptions)
	if got := normalizeFontChoice(offered, editorFontOptions); got != offered {
		t.Fatalf("offered font %q normalized to %q, want itself", offered, got)
	}
}
