package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
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
// store path, for both a request sample and a project.
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

// TestFormDirtySeesEveryField checks the merged dirty check still notices a
// change in each editable field, so a field added to formValues cannot be
// silently left out of the comparison.
func TestFormDirtySeesEveryField(t *testing.T) {
	setters := []struct {
		name string
		set  func(a *app)
	}{
		{"endpoint", func(a *app) { a.endpoint = "http://changed/1" }},
		{"action", func(a *app) { a.action = "urn:changed" }},
		{"timeout", func(a *app) { a.timeout = "1234" }},
		{"method", func(a *app) { a.method = "PUT" }},
		{"contentType", func(a *app) { a.contentType = "text/plain" }},
		{"bodyType", func(a *app) { a.bodyType = model.BodyJSON }},
		{"bodyFile", func(a *app) { a.bodyFile = "C:/tmp/x.bin" }},
		{"bodyFields", func(a *app) {
			a.bodyFields = append(a.bodyFields, bodyField{Name: "n", Value: "v"})
		}},
		{"headers", func(a *app) {
			a.headers = append(a.headers, headerRow{Name: "X-Test", Value: "1"})
		}},
		{"auth", func(a *app) { a.auth = authForm{Type: "basic", Enabled: true, Username: "u"} }},
		{"body", func(a *app) { a.xmlEd.Buffer.Reset("changed") }},
	}
	for _, tc := range setters {
		a, _ := tabTestApp(t, 1)
		if a.isDirty() {
			t.Fatalf("%s: a fresh tab starts dirty", tc.name)
		}
		tc.set(a)
		if !a.isDirty() {
			t.Fatalf("%s: the change is not seen as dirty", tc.name)
		}
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

// TestDragCallToFolder drives the real view: pressing a request row, dragging
// it over a folder and releasing moves the request into that folder.
func TestDragCallToFolder(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	ga, _ := a.st.AddGroup(p.ID, "A", "")
	gb, _ := a.st.AddGroup(p.ID, "B", "")
	call, _ := a.st.AddProjectCall(p.ID, ga.ID, model.Call{Title: "R", Method: "GET", BodyType: model.BodyNone})
	a.reload()
	a.expanded[p.ID] = true
	a.expanded[ga.ID] = true

	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	src, ok := tt.Find("R")
	if !ok {
		t.Fatal("request row not found")
	}
	dst, ok := tt.Find("B")
	if !ok {
		t.Fatal("target folder not found")
	}
	sx, sy := src.X+src.W/2, src.Y+src.H/2
	dx, dy := dst.X+dst.W/2, dst.Y+dst.H/2
	tt.Press(sx, sy)
	for i := 1; i <= 6; i++ {
		f := float32(i) / 6
		tt.Move(sx+(dx-sx)*f, sy+(dy-sy)*f)
		tt.Frame()
	}
	tt.Release(dx, dy)
	tt.Frame()

	got, _ := a.st.Get(p.ID)
	if got.Calls[0].ID != call.ID || got.Calls[0].FolderID != gb.ID {
		t.Fatalf("after drag FolderID = %q, want %q", got.Calls[0].FolderID, gb.ID)
	}
}

// TestMoveProjectCall checks a request can be moved between folders and back
// to the project root through the app method the drag uses.
func TestMoveProjectCall(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.reload()
	p := a.projects[0]
	ga, _ := a.st.AddGroup(p.ID, "A", "")
	gb, _ := a.st.AddGroup(p.ID, "B", "")
	call, _ := a.st.AddProjectCall(p.ID, ga.ID, model.Call{Title: "R", Method: "GET", BodyType: model.BodyNone})
	a.reload()

	a.moveProjectCall(p.ID, call.ID, gb.ID)
	got, _ := a.st.Get(p.ID)
	if got.Calls[0].FolderID != gb.ID {
		t.Fatalf("after move FolderID = %q, want %q", got.Calls[0].FolderID, gb.ID)
	}
	a.moveProjectCall(p.ID, call.ID, "")
	got, _ = a.st.Get(p.ID)
	if got.Calls[0].FolderID != "" {
		t.Fatalf("after move to root FolderID = %q", got.Calls[0].FolderID)
	}
	a.moveProjectCall(p.ID, call.ID, "missing")
	if a.errMsg == "" {
		t.Fatal("moving into a missing folder should report an error")
	}
}

// TestKindIconFolders checks projects and native folders draw distinct open
// and closed folder icons, while other nodes keep their glyphs.
func TestKindIconFolders(t *testing.T) {
	for _, kind := range []string{"project", "group"} {
		closed := kindIcon(kind, false)
		open := kindIcon(kind, true)
		if closed == nil || open == nil {
			t.Fatalf("%s has no folder icon", kind)
		}
		if closed == open {
			t.Fatalf("%s uses the same icon open and closed", kind)
		}
	}
	if kindIcon("call", false) != nil || kindIcon("operation", true) != nil {
		t.Fatal("non-folder nodes should keep their glyph")
	}
}

// TestEmptyWorkbenchGuidance checks a fresh window explains how to start
// instead of showing a blank form.
func TestEmptyWorkbenchGuidance(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if !tt.HasText("开始使用 soap-go") {
		t.Fatal("no empty workbench guidance")
	}
	if err := tt.Click("新建项目"); err != nil {
		t.Fatalf("click 新建项目: %v", err)
	}
	tt.Frame()
	if !a.newProjectOpen {
		t.Fatal("新建项目 button did not open the dialog")
	}
}

// TestShortcutHelpInSettings checks the shortcuts live in the settings dialog.
func TestShortcutHelpInSettings(t *testing.T) {
	a := newTestApp(t)
	a.settingsOpen = true
	a.settingsSection = 2
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if !tt.HasText("快捷键") || !tt.HasText("Ctrl/Cmd + Enter") {
		t.Fatal("settings dialog does not list shortcuts")
	}
}

// TestShortcutSettingsScrolls checks a shortcut list taller than the settings
// window can be scrolled, rather than being cut off with no way to reach the
// last rows. A large interface font makes the list overflow the fixed window.
func TestShortcutSettingsScrolls(t *testing.T) {
	a := newTestApp(t)
	a.settingsOpen, a.settingsSection = true, 2
	a.uiFontSize = 20
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()

	before, ok := tt.Find("Enter / Space")
	if !ok {
		t.Fatal("the last shortcut is not laid out")
	}
	tt.Scroll(600, 400, 0, 400)
	tt.Frame()
	after, ok := tt.Find("Enter / Space")
	if !ok {
		t.Fatal("the last shortcut is not laid out after scrolling")
	}
	if after.Y >= before.Y {
		t.Fatalf("the shortcut list did not scroll: y %v -> %v", before.Y, after.Y)
	}
}

// TestSaveAllTabs checks every changed tab is written, including a non-active
// tab whose edits live in its draft.
func TestSaveAllTabs(t *testing.T) {
	a, keys := tabTestApp(t, 2)
	if d := a.drafts[keys[0]]; d != nil {
		d.endpoint = "http://changed/0"
	} else {
		t.Fatal("missing draft for the first tab")
	}
	a.endpoint = "http://changed/1"
	if !a.saveAllTabs() {
		t.Fatalf("saveAllTabs failed: %s", a.errMsg)
	}
	if a.hasUnsavedTabs() {
		t.Fatal("tabs still dirty after save-all")
	}
	a.reload()
	changed := 0
	for _, c := range a.projects[0].Calls {
		if strings.HasPrefix(c.Endpoint, "http://changed/") {
			changed++
		}
	}
	if changed != 2 {
		t.Fatalf("saved endpoints changed=%d, calls=%+v", changed, a.projects[0].Calls)
	}
}

// TestQuitDialogOffersSaveAll checks the unsaved-changes quit dialog shows both
// "save all and quit" and the plain "quit".
func TestQuitDialogOffersSaveAll(t *testing.T) {
	a, keys := tabTestApp(t, 1)
	_ = keys
	a.endpoint = "http://dirty"
	a.askQuit()
	if !a.confirmOpen || a.confirmLabel != "全部保存并退出" || a.confirmAltLabel != "退出" {
		t.Fatalf("quit dialog labels: %q / %q", a.confirmLabel, a.confirmAltLabel)
	}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if !tt.HasText("全部保存并退出") || !tt.HasText("退出") {
		t.Fatal("quit dialog does not show both actions")
	}
}

// TestConfirmUsesSpecificLabel checks the confirmation button says what it
// does instead of a generic 确定.
func TestConfirmUsesSpecificLabel(t *testing.T) {
	a := newTestApp(t)
	a.confirmOpen = true
	a.confirmText = "确定删除该请求？"
	a.confirmLabel = "删除"
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if _, ok := tt.AccessByRole(ui.RoleButton, "删除"); !ok {
		t.Fatal("confirm button does not use the specific label")
	}
}

// TestNameValueInputsAccessible checks the name/value rows carry accessible
// names instead of relying on disappearing placeholders alone.
func TestNameValueInputsAccessible(t *testing.T) {
	a, _ := newNativeCall(t)
	a.reqKind = model.KindHTTP
	a.headers = []headerRow{{}}
	a.reqSection = 2
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if _, ok := tt.Access("请求头名称"); !ok {
		t.Fatal("header name input has no accessible name")
	}
	if _, ok := tt.Access("请求头值"); !ok {
		t.Fatal("header value input has no accessible name")
	}

	a.queryRows = []queryRow{{}}
	a.reqSection = 1
	tt.Frame()
	if _, ok := tt.Access("参数名"); !ok {
		t.Fatal("query key input has no accessible name")
	}
	if _, ok := tt.Access("参数值"); !ok {
		t.Fatal("query value input has no accessible name")
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

// TestTreeSearchFindsInsideCollapsedFolder checks a search still reaches a
// request under a folder the user has collapsed.
func TestTreeSearchFindsInsideCollapsedFolder(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	p := a.projects[0]
	g, err := a.st.AddGroup(p.ID, "折叠目录", "")
	if err != nil {
		t.Fatalf("AddGroup: %v", err)
	}
	if _, err := a.st.AddProjectCall(p.ID, g.ID, model.Call{
		Title: "隐藏接口", Kind: model.KindHTTP, Method: "GET", BodyType: model.BodyNone,
	}); err != nil {
		t.Fatalf("AddProjectCall: %v", err)
	}
	a.reload()
	a.expanded[p.ID] = true
	a.expanded[g.ID] = false
	a.search = "隐藏接口"

	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if !tt.HasText("隐藏接口") {
		t.Fatal("search did not reveal a request inside a collapsed folder")
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

// TestWorkspaceActionsWarnAboutEveryDirtyTab checks the workspace dialogs count
// unsaved edits in tabs other than the active one, which their old isDirty
// check missed.
func TestWorkspaceActionsWarnAboutEveryDirtyTab(t *testing.T) {
	cases := []struct {
		name string
		run  func(a *app)
	}{
		{"switch", func(a *app) { a.switchWorkspace("missing") }},
		{"create", func(a *app) { a.createWorkspace("W") }},
		{"import", func(a *app) { a.importWorkspaceNamed("x.json", "W") }},
		{"delete", func(a *app) { a.deleteWorkspace() }},
	}
	for _, tc := range cases {
		a, keys := tabTestApp(t, 2)
		// Edit the last (active) tab, then switch to the clean first tab, so the
		// dirty tab is only a stored draft.
		a.endpoint = "http://changed/last"
		pid, sid, oid, cid := splitKey(keys[0])
		a.openCall(pid, sid, oid, cid)
		if a.isDirty() {
			t.Fatalf("%s: setup left the active tab dirty", tc.name)
		}
		tc.run(a)
		if !a.confirmOpen {
			t.Fatalf("%s: no confirmation for another tab's unsaved edits", tc.name)
		}
		if !strings.Contains(a.confirmText, "1 个标签") {
			t.Fatalf("%s: confirmation %q does not name the dirty tab", tc.name, a.confirmText)
		}
	}
}

// TestImportProxyChoice checks the WSDL URL dialog resolves its proxy choice
// on the UI thread, and an empty URL is rejected before any fetch starts.
func TestImportProxyChoice(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	a.projects[0].Proxy = &model.ProxySettings{
		Mode: "custom", Type: "http", Host: "10.0.0.5", Port: 8080,
	}
	a.importProxySel = "当前项目代理"
	if got := a.importProxy(); got == nil || got.Host != "10.0.0.5" || got.Port != 8080 {
		t.Fatalf("current project proxy = %+v", got)
	}
	a.importProxySel = "系统代理"
	if got := a.importProxy(); got.Mode != "system" {
		t.Fatalf("system proxy = %+v", got)
	}
	a.importProxySel = "直连（默认）"
	if got := a.importProxy(); got.Mode != "direct" {
		t.Fatalf("direct proxy = %+v", got)
	}

	a.importWSDLURL("   ")
	if a.errMsg == "" || a.importing {
		t.Fatalf("empty URL: errMsg = %q importing = %v", a.errMsg, a.importing)
	}
}

// TestHeaderLinesPrintsEveryValue checks a repeated response header (stored as
// newline-joined values) is shown as one line per value.
func TestHeaderLinesPrintsEveryValue(t *testing.T) {
	got := headerLines(map[string]string{"Set-Cookie": "a=1\nb=2"})
	want := "Set-Cookie: a=1\nSet-Cookie: b=2\n"
	if got != want {
		t.Fatalf("headerLines = %q, want %q", got, want)
	}
}

// TestCurlCommandForFileBodies checks a multipart body becomes -F fields and a
// binary body points curl at the file instead of inlining its bytes.
func TestCurlCommandForFileBodies(t *testing.T) {
	a, _ := tabTestApp(t, 1)
	a.reqKind = model.KindHTTP
	a.method = "POST"
	a.bodyType = model.BodyBinary
	a.bodyFile = "C:/tmp/report.pdf"
	a.contentType = "application/pdf"

	cmd, err := a.curlCommand()
	if err != nil {
		t.Fatalf("binary curl: %v", err)
	}
	if !strings.Contains(cmd, "--data-binary '@C:/tmp/report.pdf'") {
		t.Fatalf("binary curl does not point at the file:\n%s", cmd)
	}
	if strings.Contains(cmd, "Content-Type: application/pdf") == false {
		t.Fatalf("binary curl lost its content type:\n%s", cmd)
	}

	a.bodyFile = ""
	if _, err := a.curlCommand(); err == nil {
		t.Fatal("binary curl without a file should fail")
	}

	a.bodyType = model.BodyMultipart
	a.bodyFields = []bodyField{
		{Name: "note", Value: "hi"},
		{Name: "doc", Type: "file", Value: "C:/tmp/a.png"},
	}
	cmd, err = a.curlCommand()
	if err != nil {
		t.Fatalf("multipart curl: %v", err)
	}
	if !strings.Contains(cmd, "-F 'note=hi'") || !strings.Contains(cmd, "-F 'doc=@C:/tmp/a.png'") {
		t.Fatalf("multipart curl lacks its -F fields:\n%s", cmd)
	}
	if strings.Contains(cmd, "boundary") {
		t.Fatalf("multipart curl embeds a boundary body:\n%s", cmd)
	}
}

// TestProxyPasswordIsMasked checks the project page never shows the proxy
// password in clear text.
func TestProxyPasswordIsMasked(t *testing.T) {
	a := newTestApp(t)
	a.createProject("P")
	p := a.projects[0]
	if err := a.st.UpdateProjectProxy(p.ID, &model.ProxySettings{
		Mode: "custom", Type: "http", Host: "127.0.0.1", Port: 8080, Password: "s3cret",
	}); err != nil {
		t.Fatalf("UpdateProjectProxy: %v", err)
	}
	a.reload()
	a.openProjectPage(p.ID)
	if a.proxyModeSel != "自定义代理" || a.proxyPass != "s3cret" {
		t.Fatalf("proxy form = %q/%q, want the custom settings", a.proxyModeSel, a.proxyPass)
	}

	tt := ui.NewTester(a.view, 1100, 800)
	tt.Frame()
	if tt.HasText("s3cret") {
		t.Fatal("the proxy password is shown in clear text")
	}
}

// TestWSDLImportShowsProgress checks the URL import dialog shows progress while
// the background fetch runs and cannot be started twice.
func TestWSDLImportShowsProgress(t *testing.T) {
	a := newTestApp(t)
	a.importOpen, a.importKind, a.importing = true, 3, true

	tt := ui.NewTester(a.view, 900, 700)
	tt.Frame()
	if !tt.HasText("正在下载并解析 WSDL，请稍候…") {
		t.Fatal("no progress hint while importing")
	}
	if !tt.HasText("获取中…") {
		t.Fatal("the import button still reads 获取并导入 while importing")
	}
}

// TestStatusMessageExpires checks transient status messages clear themselves
// while sticky progress messages stay until replaced.
func TestStatusMessageExpires(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 900, 600)
	tt.Frame()

	a.setStatus("临时提示-xyz")
	tt.Frame()
	if !tt.HasText("临时提示-xyz") {
		t.Fatal("a fresh status message is not shown")
	}
	a.statusAt = time.Now().Add(-statusTTL - time.Second)
	tt.Frame()
	if tt.HasText("临时提示-xyz") {
		t.Fatal("a transient status message did not expire")
	}

	a.setStickyStatus("发送中…")
	a.statusAt = time.Now().Add(-statusTTL - time.Second)
	tt.Frame()
	if !tt.HasText("发送中…") {
		t.Fatal("a sticky status message expired")
	}
}

// TestTabSwitchAndClose checks the Ctrl+Tab / Ctrl+W helpers move and close
// tabs, wrapping at both ends.
func TestTabSwitchAndClose(t *testing.T) {
	a, keys := tabTestApp(t, 3)
	if got := a.activeTabKey(); got != keys[2] {
		t.Fatalf("setup active = %q, want %q", got, keys[2])
	}
	a.switchTab(1)
	if got := a.activeTabKey(); got != keys[0] {
		t.Fatalf("switch past the end = %q, want %q", got, keys[0])
	}
	a.switchTab(-1)
	if got := a.activeTabKey(); got != keys[2] {
		t.Fatalf("switch before the start = %q, want %q", got, keys[2])
	}
	a.closeActiveTab()
	if len(a.tabs) != 2 || a.hasTabKey(keys[2]) {
		t.Fatalf("after close: tabs=%v", a.tabs)
	}
}

// TestShortcutHelpListsTabKeys checks the shortcuts dialog documents the tab
// switching and closing keys.
func TestShortcutHelpListsTabKeys(t *testing.T) {
	var all strings.Builder
	for _, s := range shortcutHelp {
		all.WriteString(s.keys + " " + s.desc + "\n")
	}
	for _, want := range []string{"Ctrl + Tab", "Ctrl + Shift + Tab", "Ctrl/Cmd + W"} {
		if !strings.Contains(all.String(), want) {
			t.Fatalf("shortcut help lacks %q", want)
		}
	}
}

// TestTimeoutNormalizesAndSteps checks a typed timeout is rewritten to the
// value actually used, and the stepper stays within the allowed range.
func TestTimeoutNormalizesAndSteps(t *testing.T) {
	a := newTestApp(t)
	a.timeout = "60"
	a.normalizeTimeout()
	if a.timeout != "1000" {
		t.Fatalf("timeout 60 normalized to %q, want 1000", a.timeout)
	}
	a.timeout = "999999"
	a.normalizeTimeout()
	if a.timeout != "600000" {
		t.Fatalf("timeout 999999 normalized to %q, want 600000", a.timeout)
	}
	a.timeout = "5000"
	a.stepTimeout(1000)
	if a.timeout != "6000" {
		t.Fatalf("step up = %q, want 6000", a.timeout)
	}
	a.stepTimeout(-10000)
	if a.timeout != "1000" {
		t.Fatalf("step down = %q, want 1000", a.timeout)
	}
}

// TestSaveResponseToFile checks the response body can be written out as-is.
func TestSaveResponseToFile(t *testing.T) {
	a := newTestApp(t)
	a.resp = model.SendResponse{Status: 200, Body: `{"ok":true}`}
	path := filepath.Join(t.TempDir(), "response.json")
	if err := a.saveResponseTo(path); err != nil {
		t.Fatalf("saveResponseTo: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != `{"ok":true}` {
		t.Fatalf("saved body = %q", data)
	}
}

// TestBinaryBodySentFromFilePath checks the UI thread only records the file
// path: buildSendRequest must not read the file (it may not even exist yet).
func TestBinaryBodySentFromFilePath(t *testing.T) {
	a, _ := tabTestApp(t, 1)
	a.reqKind = model.KindHTTP
	a.method = "POST"
	a.bodyType = model.BodyBinary
	a.bodyFile = "C:/does/not/exist/upload.bin"

	req, _, err := a.buildSendRequest()
	if err != nil {
		t.Fatalf("buildSendRequest read the file on the UI thread: %v", err)
	}
	if req.FilePath != a.bodyFile || req.Body != "" {
		t.Fatalf("request = %+v, want FilePath only", req)
	}
}

// TestBinaryBodyCacheInvalidates checks the preview cache is reused while the
// file is unchanged and refreshed when it changes.
func TestBinaryBodyCacheInvalidates(t *testing.T) {
	a := newTestApp(t)
	path := filepath.Join(t.TempDir(), "body.bin")
	if err := os.WriteFile(path, []byte("one"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got, err := a.readBinaryBody(path); err != nil || got != "one" {
		t.Fatalf("first read = %q, %v", got, err)
	}
	if err := os.WriteFile(path, []byte("two-two"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got, err := a.readBinaryBody(path); err != nil || got != "two-two" {
		t.Fatalf("second read = %q, %v", got, err)
	}
}

// TestResponsePanelOffersSave checks a response can be written to a file from
// the response panel, including a large truncated one.
func TestResponsePanelOffersSave(t *testing.T) {
	a, _ := newNativeCall(t)
	a.hasResp = true
	a.resp = model.SendResponse{Status: 200, StatusText: "OK", Body: "hi", Truncated: true}
	a.syncResponse()

	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if !tt.HasText("保存到文件…") {
		t.Fatal("the response panel has no save-to-file action")
	}
}

// TestSaveUIStateWritesAtomically checks the UI state lands on disk in one
// piece, without a leftover temp file beside it.
func TestSaveUIStateWritesAtomically(t *testing.T) {
	a := newTestApp(t)
	a.dark = false // differs from the saved snapshot, forcing a write
	a.saveUIState()

	data, err := os.ReadFile(a.uiPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var st uiState
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatalf("ui-state.json is not valid JSON: %v", err)
	}
	if st.Dark {
		t.Fatal("the written state does not hold the change")
	}
	if _, err := os.Stat(a.uiPath + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("a temp file survived the atomic write: %v", err)
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
// holds only import now that project exports moved to the project page, and the
// workspace menu still holds workspace export.
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
	if !strings.Contains(items, "导入…") {
		t.Fatalf("more menu = %v", tt.Menu())
	}
	for _, gone := range []string{"导出 SoapUI XML…", "导出 OpenAPI"} {
		if strings.Contains(items, gone) {
			t.Fatalf("more menu still offers %q: %v", gone, tt.Menu())
		}
	}
	tt.CloseMenu()
	tt.Frame()

	// A SoapUI-sourced project also keeps its exports off the more menu.
	a.createProject("S")
	a.reload()
	a.projects[1].Source = "soapui"
	a.pid = a.projects[1].ID
	tt.Frame()
	if err := tt.Click("更多"); err != nil {
		t.Fatalf("click 更多: %v", err)
	}
	tt.Frame()
	if items := strings.Join(tt.Menu(), ","); strings.Contains(items, "导出") {
		t.Fatalf("soap project more menu still offers an export: %v", tt.Menu())
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

// TestProjectPageExports checks the project-level exports live on the project
// page, offering only what the project's source supports.
func TestProjectPageExports(t *testing.T) {
	a := newTestApp(t)
	a.createProject("原生")
	a.reload()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if !tt.HasText("导出 OpenAPI (JSON)…") || !tt.HasText("导出 OpenAPI (YAML)…") {
		t.Fatal("native project page lacks the OpenAPI exports")
	}
	if tt.HasText("导出 SoapUI XML…") {
		t.Fatal("native project page must not offer the SoapUI export")
	}

	// A SoapUI project offers the SoapUI export and no OpenAPI one.
	a.projects[0].Source = "soapui"
	a.openProjectPage(a.projects[0].ID)
	tt.Frame()
	if !tt.HasText("导出 SoapUI XML…") {
		t.Fatal("soapui project page lacks the SoapUI export")
	}
	if tt.HasText("导出 OpenAPI (JSON)…") {
		t.Fatal("soapui project page must not offer the OpenAPI export")
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

// TestBulkEditFields checks the bulk editor round-trips the rows and parses both
// "name: value" and "name=value" lines.
func TestBulkEditFields(t *testing.T) {
	a, _ := newNativeCall(t)
	a.setBodyType(model.BodyMultipart)
	a.bodyFields = []bodyField{{Name: "a", Value: "1"}}
	a.openBulkEdit()
	if a.bulkText != "a: 1" {
		t.Fatalf("bulkText = %q", a.bulkText)
	}
	a.bulkText = "x=2\ny: 3\nbare\n\n"
	a.applyBulkEdit()
	if a.bulkOpen {
		t.Fatal("bulk modal still open")
	}
	if len(a.bodyFields) != 3 ||
		a.bodyFields[0].Name != "x" || a.bodyFields[0].Value != "2" ||
		a.bodyFields[1].Name != "y" || a.bodyFields[1].Value != "3" ||
		a.bodyFields[2].Name != "bare" || a.bodyFields[2].Value != "" {
		t.Fatalf("fields = %+v", a.bodyFields)
	}

	// A file field keeps its type when its name survives the bulk edit.
	a.bodyFields = []bodyField{{Name: "doc", Value: "/tmp/a.bin", Type: "file"}}
	a.openBulkEdit()
	a.applyBulkEdit()
	if len(a.bodyFields) != 1 || a.bodyFields[0].Type != "file" {
		t.Fatalf("file type lost: %+v", a.bodyFields)
	}
}

// TestMultipartFileField checks a file field becomes a file part with its
// filename, content type and bytes, and that a missing file is an error.
func TestMultipartFileField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "up.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	body, ct, err := buildMultipart([]bodyField{
		{Name: "doc", Value: path, Type: "file"},
		{Name: "note", Value: "hi"},
	})
	if err != nil {
		t.Fatalf("buildMultipart: %v", err)
	}
	if !strings.HasPrefix(ct, "multipart/form-data; boundary=") {
		t.Fatalf("ct = %q", ct)
	}
	if !strings.Contains(body, `name="doc"; filename="up.txt"`) ||
		!strings.Contains(body, "Content-Type: text/plain") ||
		!strings.Contains(body, "hello") ||
		!strings.Contains(body, `name="note"`) {
		t.Fatalf("body = %q", body)
	}
	if _, _, err := buildMultipart([]bodyField{{Name: "doc", Type: "file"}}); err == nil {
		t.Fatal("a file field without a path should fail")
	}
}

// TestMultipartFileFieldPersists checks a file field's type and path survive a
// save and reload.
func TestMultipartFileFieldPersists(t *testing.T) {
	a, _ := newNativeCall(t)
	a.setBodyType(model.BodyMultipart)
	path := filepath.Join(t.TempDir(), "a.bin")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.bodyFields = []bodyField{{Name: "f", Value: path, Type: "file"}}
	a.saveCall()
	a.reload()
	got := a.projects[0].Calls[0].BodyFields
	if len(got) != 1 || got[0].Type != "file" || got[0].Value != path {
		t.Fatalf("stored fields = %+v", got)
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
	// The file is read by the sending goroutine, not on the UI thread.
	if req.Body != "" || req.FilePath != f || req.ContentType != "application/octet-stream" {
		t.Fatalf("binary = body %q file %q ct %q", req.Body, req.FilePath, req.ContentType)
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

// TestImportOpenAPICreatesNativeProject checks an OpenAPI import lands as a
// native project with its operations as requests, and that opening one shows
// the operation description in the 设置 tab.
func TestImportOpenAPICreatesNativeProject(t *testing.T) {
	a := newTestApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.openapi.json")
	doc := `{
	  "openapi": "3.0.1",
	  "info": {"title": "OpenAPI 演示", "version": "1.0.0"},
	  "paths": {
	    "/user": {
	      "get": {
	        "summary": "用户列表",
	        "description": "查询用户列表。",
	        "tags": ["User"],
	        "parameters": [{"name": "q", "in": "query", "example": "z", "schema": {"type": "string"}}],
	        "responses": {"200": {"description": "ok"}}
	      }
	    }
	  }
	}`
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	a.importOpenAPI(path)
	if a.errMsg != "" {
		t.Fatalf("import error: %s", a.errMsg)
	}
	if len(a.projects) != 1 {
		t.Fatalf("projects = %d", len(a.projects))
	}
	p := a.projects[0]
	if p.Source != "native" || p.Name != "OpenAPI 演示" {
		t.Fatalf("project = %q/%q", p.Name, p.Source)
	}
	if len(p.Calls) != 1 || len(p.Groups) != 1 {
		t.Fatalf("calls=%d groups=%d", len(p.Calls), len(p.Groups))
	}
	a.openCall(p.ID, "", "", p.Calls[0].ID)
	if a.reqDescription != "查询用户列表。" {
		t.Fatalf("reqDescription = %q", a.reqDescription)
	}
	if a.endpoint != "/user?q=z" {
		t.Fatalf("endpoint = %q", a.endpoint)
	}
}

// TestTabLimitClosesOldestSaved checks the limit closes the earliest opened
// saved tab and never a dirty one, and never the tab just opened.
func TestTabLimitClosesOldestSaved(t *testing.T) {
	a := newTestApp(t)
	proj, err := a.st.AddProject(model.Project{Name: "Tabs", Source: "native"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	for i := 0; i < 8; i++ {
		if _, err := a.st.AddProjectCall(proj.ID, "", model.Call{
			Title: fmt.Sprintf("请求 %02d", i), Kind: model.KindHTTP,
			Endpoint: fmt.Sprintf("http://example.test/%d", i),
		}); err != nil {
			t.Fatalf("AddProjectCall: %v", err)
		}
	}
	a.reload()
	a.maxTabs = 5
	p := a.projects[0]
	key := func(i int) string { return callKey(p.ID, "", "", p.Calls[i].ID) }
	for i := 0; i < 5; i++ {
		a.openCall(p.ID, "", "", p.Calls[i].ID)
	}
	if len(a.tabs) != 5 {
		t.Fatalf("tabs = %d, want 5", len(a.tabs))
	}
	a.openCall(p.ID, "", "", p.Calls[5].ID)
	if len(a.tabs) != 5 || a.tabs[0] != key(1) || !a.hasTabKey(key(5)) {
		t.Fatalf("after 6th open: %v", a.tabs)
	}
	if !strings.Contains(a.statusMsg, "已自动关闭") {
		t.Fatalf("auto-close not reported: %q", a.statusMsg)
	}

	// A dirty oldest tab is skipped, so the next oldest saved one goes.
	a.drafts[a.tabs[0]] = &callDraft{
		formValues:  formValues{endpoint: "edited"},
		savedValues: savedValues{savedEndpoint: "clean"},
	}
	a.openCall(p.ID, "", "", p.Calls[6].ID)
	if len(a.tabs) != 5 || a.tabs[0] != key(1) || a.hasTabKey(key(2)) {
		t.Fatalf("dirty skip: %v", a.tabs)
	}
}

// TestTabLimitPersists checks the limit is stored and read back, out-of-range
// values falling back to the default.
func TestTabLimitPersists(t *testing.T) {
	a := newTestApp(t)
	if a.maxTabs != defaultTabLimit {
		t.Fatalf("default maxTabs = %d", a.maxTabs)
	}
	a.maxTabs = 33
	if st := a.currentUI(); st.MaxTabs != 33 {
		t.Fatalf("currentUI MaxTabs = %d", st.MaxTabs)
	}
	if err := os.WriteFile(a.uiPath, []byte(`{"maxTabs": 42}`), 0o644); err != nil {
		t.Fatal(err)
	}
	a.loadUIState()
	if a.maxTabs != 42 {
		t.Fatalf("loaded maxTabs = %d", a.maxTabs)
	}
	if err := os.WriteFile(a.uiPath, []byte(`{"maxTabs": 999}`), 0o644); err != nil {
		t.Fatal(err)
	}
	a.loadUIState()
	if a.maxTabs != 42 {
		t.Fatalf("out-of-range maxTabs changed to %d", a.maxTabs)
	}
}

// TestOtherSettingsTabLimit checks the 其他 section shows the tab limit and the
// manual input applies a valid number while ignoring an invalid one.
func TestOtherSettingsTabLimit(t *testing.T) {
	a := newTestApp(t)
	a.settingsOpen = true
	a.settingsSection = 3
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if !tt.HasText("其他") || !tt.HasText("标签页数量上限") {
		t.Fatal("其他 section lacks the tab limit row")
	}
	a.maxTabsText = "35"
	a.applyMaxTabsText()
	if a.maxTabs != 35 {
		t.Fatalf("maxTabs = %d, want 35", a.maxTabs)
	}
	a.maxTabsText = "999"
	a.applyMaxTabsText()
	if a.maxTabs != 35 {
		t.Fatalf("invalid input changed maxTabs to %d", a.maxTabs)
	}
}

// TestBodyTypeLabelsEnglish checks the body-type names the picker shows are the
// English ones, and that GraphQL is no longer a body type.
func TestBodyTypeLabelsEnglish(t *testing.T) {
	cases := map[string]string{
		model.BodyJSON:       "JSON",
		model.BodyXML:        "XML",
		model.BodyText:       "Other",
		model.BodyURLEncoded: "Url Encoded",
		model.BodyMultipart:  "Multi-Part",
		model.BodyBinary:     "Binary File",
		model.BodyNone:       "No Body",
	}
	for bt, want := range cases {
		if got := bodyTypeLabel(bt); got != want {
			t.Errorf("bodyTypeLabel(%q) = %q, want %q", bt, got, want)
		}
	}
	if got := model.NormalizeBodyType("graphql"); got != "" {
		t.Errorf("graphql should no longer be a body type, got %q", got)
	}
}

// tabTestApp builds an app with n native requests and opens every one as a tab.
func tabTestApp(t *testing.T, n int) (*app, []string) {
	t.Helper()
	a := newTestApp(t)
	proj, err := a.st.AddProject(model.Project{Name: "Tabs", Source: "native"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	for i := 0; i < n; i++ {
		_, err := a.st.AddProjectCall(proj.ID, "", model.Call{
			Title: fmt.Sprintf("请求 %02d", i), Kind: model.KindHTTP,
			Endpoint: fmt.Sprintf("http://example.test/%d", i),
		})
		if err != nil {
			t.Fatalf("AddProjectCall: %v", err)
		}
	}
	a.reload()
	p := a.projects[0]
	keys := make([]string, 0, n)
	for _, c := range p.Calls {
		a.openCall(p.ID, "", "", c.ID)
		keys = append(keys, callKey(p.ID, "", "", c.ID))
	}
	return a, keys
}

// TestTabStripKeepsSplitSwitch checks a full strip stays inside its own scroller
// so the split buttons remain reachable, and that a wheel over the tabs scrolls
// them sideways.
func TestTabStripKeepsSplitSwitch(t *testing.T) {
	a, _ := tabTestApp(t, 15)
	tt := ui.NewTester(a.view, 900, 700)
	tt.Frame()

	if err := tt.Click("左右分屏"); err != nil {
		t.Fatalf("split button unreachable with many tabs: %v", err)
	}
	if a.splitSide != 1 {
		t.Fatalf("splitSide = %d, want 1", a.splitSide)
	}

	a.tabScroll.X = 0
	tt.Frame()
	before := a.tabScroll.X
	tt.Move(400, 60)
	tt.Wheel(400, 60, 0, 120)
	tt.Frame()
	if a.tabScroll.X <= before {
		t.Fatalf("wheel did not scroll tabs sideways: %v -> %v", before, a.tabScroll.X)
	}
}

// TestTabContextMenu checks a right click on a tab offers the close items and
// that choosing one closes the other tabs.
func TestTabContextMenu(t *testing.T) {
	a, keys := tabTestApp(t, 5)
	tt := ui.NewTester(a.view, 900, 700)
	tt.Frame()
	tt.RightClickAt(300, 62)
	items := tt.Menu()
	want := []string{"关闭当前标签页", "关闭其他标签页", "关闭左侧标签页", "关闭右侧标签页"}
	for _, w := range want {
		found := false
		for _, it := range items {
			if it == w {
				found = true
			}
		}
		if !found {
			t.Fatalf("context menu %v lacks %q", items, w)
		}
	}
	if err := tt.ChooseMenuItem("关闭其他标签页"); err != nil {
		t.Fatalf("choose: %v", err)
	}
	tt.Frame()
	if len(a.tabs) != 1 {
		t.Fatalf("after close others: %v", a.tabs)
	}
	// The pointer lands on whichever tab the layout put at that x, so only
	// require the survivor to be one of the tabs that were open.
	kept := a.tabs[0]
	found := false
	for _, k := range keys {
		if k == kept {
			found = true
		}
	}
	if !found {
		t.Fatalf("kept tab %q is not one of the originals", kept)
	}
}

// TestMenusDrawnInWindow checks the app draws its popup menus in the window
// with the theme instead of letting the system draw them, so the tab, tree and
// text-field menus match the rest of the UI.
func TestMenusDrawnInWindow(t *testing.T) {
	a, _ := tabTestApp(t, 3)
	var got ui.MenuPresentation
	tt := ui.NewTester(func(c *ui.Context) {
		a.view(c)
		got = c.MenuPresentation()
	}, 900, 700)
	tt.Frame()
	if got != ui.PopoverMenus {
		t.Fatalf("menu presentation = %d, want ui.PopoverMenus", got)
	}
	tt.RightClickAt(300, 62)
	tt.Frame()
	// A menu the system draws is not part of the window's frame, so its labels
	// do not show in the rendered window; one drawn in the window does.
	if !tt.HasText("关闭当前标签页") {
		t.Fatalf("the tab menu is not drawn in the window")
	}
	if items := tt.Menu(); len(items) == 0 {
		t.Fatalf("the drawn tab menu offers no items")
	}
}

// TestProjectPageShowsCounts checks clicking a project opens its page, with the
// folder and request counts and the project proxy form.
func TestProjectPageShowsCounts(t *testing.T) {
	a := newTestApp(t)
	a.createProject("演示项目")
	p := a.projects[0]
	a.st.AddGroup(p.ID, "User", "")
	a.st.AddProjectCall(p.ID, "", model.Call{
		Title: "登录", Kind: model.KindHTTP, Method: "POST", Endpoint: "/login",
	})
	a.reload()
	a.selectTree(treeItem{kind: "project", pid: p.ID})
	if !a.projectPage {
		t.Fatal("clicking a project did not open its page")
	}
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()
	for _, want := range []string{"目录数", "接口数", "项目 Host", "代理模式"} {
		if !tt.HasText(want) {
			t.Fatalf("project page lacks %q", want)
		}
	}
	if !tt.HasText("1 个目录 · 1 个接口") {
		t.Fatal("project page does not show the folder and request counts")
	}
}

// TestProjectPageSavesHostAndProxy checks the page's save buttons write the
// host and proxy settings back to the selected project.
func TestProjectPageSavesHostAndProxy(t *testing.T) {
	a := newTestApp(t)
	a.createProject("演示项目")
	p := a.projects[0]

	a.hostInput = "http://localhost/projectA/"
	a.applyProjectHost()
	if got := a.projects[0].Host; got != "http://localhost/projectA" {
		t.Fatalf("host = %q, want the normalized value", got)
	}

	a.proxyModeSel, a.proxyTypeSel = "自定义代理", "HTTP"
	a.proxyHost, a.proxyPort, a.proxyUser, a.proxyPass = "127.0.0.1", "8080", "u", "p"
	a.updateProxy()
	proxy := a.projects[0].Proxy
	if proxy == nil || proxy.Mode != "custom" || proxy.Host != "127.0.0.1" || proxy.Port != 8080 {
		t.Fatalf("proxy = %+v, want the custom settings", proxy)
	}

	a.st.UpdateProjectProxy(p.ID, &model.ProxySettings{
		Mode: "custom", Type: "socks5", Host: "10.0.0.1", Port: 1080,
	})
	a.reload()
	a.openProjectPage(p.ID)
	if a.proxyModeSel != "自定义代理" || a.proxyTypeSel != "SOCKS5" ||
		a.proxyHost != "10.0.0.1" || a.proxyPort != "1080" {
		t.Fatalf("proxy form = %q/%q/%q/%q, want the stored settings",
			a.proxyModeSel, a.proxyTypeSel, a.proxyHost, a.proxyPort)
	}
}

// TestProjectHostPrefixInUrlBar checks the host shows in front of the request
// address only while the endpoint is a relative path.
func TestProjectHostPrefixInUrlBar(t *testing.T) {
	a, _ := tabTestApp(t, 2)
	p := a.projects[0]
	a.st.UpdateProjectHost(p.ID, "http://localhost/projectA")
	a.reload()
	p = a.projects[0]
	a.openCall(p.ID, "", "", p.Calls[0].ID)
	a.endpoint = "/a/b/list"

	tt := ui.NewTester(a.view, 900, 700)
	tt.Frame()
	if !tt.HasText("http://localhost/projectA") {
		t.Fatal("the project host is not shown before a relative endpoint")
	}

	a.endpoint = "http://other.test/x/y"
	tt.Frame()
	if tt.HasText("http://localhost/projectA") {
		t.Fatal("the project host still shows for a full endpoint address")
	}
}

// TestEndpointInputKeepsFocusWhenHostHides checks that typing a full address,
// which hides the host prefix in front of the field, keeps the input focused
// so the rest of the address can be typed.
func TestEndpointInputKeepsFocusWhenHostHides(t *testing.T) {
	a, _ := tabTestApp(t, 1)
	p := a.projects[0]
	if err := a.st.UpdateProjectHost(p.ID, "http://localhost/projectA"); err != nil {
		t.Fatalf("UpdateProjectHost: %v", err)
	}
	a.reload()
	p = a.projects[0]
	a.openCall(p.ID, "", "", p.Calls[0].ID)
	a.endpoint = ""

	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()

	info, ok := tt.AccessByRole(ui.RoleTextField, "请求地址")
	if !ok {
		t.Fatal("the request address field is missing")
	}
	tt.ClickAt(info.Bounds.X+info.Bounds.W/2, info.Bounds.Y+info.Bounds.H/2)
	tt.Frame()
	if info, ok = tt.AccessByRole(ui.RoleTextField, "请求地址"); !ok || !info.Focused {
		t.Fatal("the request address field did not take focus")
	}

	// Typing the scheme makes the endpoint absolute, which hides the host
	// prefix; the field must keep the focus through that relayout.
	tt.Type("http://l")
	tt.Frame()
	info, ok = tt.AccessByRole(ui.RoleTextField, "请求地址")
	if !ok {
		t.Fatal("the request address field disappeared")
	}
	if !info.Focused {
		t.Fatal("the request address field lost focus when the host prefix hid")
	}
	if a.endpoint != "http://l" {
		t.Fatalf("endpoint = %q, want the typed address", a.endpoint)
	}
}

// TestSendJoinsProjectHost checks the request built for sending appends a
// relative endpoint to the project host, and leaves a full address alone.
func TestSendJoinsProjectHost(t *testing.T) {
	a := newTestApp(t)
	a.createProject("演示项目")
	p := a.projects[0]
	if _, err := a.st.AddProjectCall(p.ID, "", model.Call{
		Title: "列表", Kind: model.KindHTTP, Method: "GET", Endpoint: "/a/b/list",
	}); err != nil {
		t.Fatalf("AddProjectCall: %v", err)
	}
	if err := a.st.UpdateProjectHost(p.ID, "http://localhost/projectA"); err != nil {
		t.Fatalf("UpdateProjectHost: %v", err)
	}
	a.reload()
	p = a.projects[0]
	a.openCall(p.ID, "", "", p.Calls[0].ID)

	req, _, err := a.buildSendRequest()
	if err != nil {
		t.Fatalf("buildSendRequest: %v", err)
	}
	if want := "http://localhost/projectA/a/b/list"; req.URL != want {
		t.Fatalf("URL = %q, want %q", req.URL, want)
	}

	a.endpoint = "http://other.test/x/y"
	req, _, err = a.buildSendRequest()
	if err != nil {
		t.Fatalf("buildSendRequest (absolute): %v", err)
	}
	if req.URL != "http://other.test/x/y" {
		t.Fatalf("URL = %q, want the endpoint as typed", req.URL)
	}
}

// TestOpenCallLeavesProjectPage checks opening a request returns the main area
// to the request editor.
func TestOpenCallLeavesProjectPage(t *testing.T) {
	a, keys := tabTestApp(t, 2)
	p := a.projects[0]
	a.openProjectPage(p.ID)
	if !a.projectPage {
		t.Fatal("openProjectPage did not switch to the page")
	}
	pid, sid, oid, cid := splitKey(keys[0])
	a.openCall(pid, sid, oid, cid)
	if a.projectPage {
		t.Fatal("opening a request left the project page showing")
	}
}

// TestNameDialogPrompts checks the shared name dialog still shows each
// caller's own title or label after the three copies were merged.
func TestNameDialogPrompts(t *testing.T) {
	cases := []struct {
		name string
		open func(a *app)
		want string
	}{
		{"project", func(a *app) { a.newProjectOpen = true }, "项目名称"},
		{"workspace rename", func(a *app) { a.wsOpen, a.wsKind = true, 1 }, "重命名工作空间"},
		{"folder", func(a *app) { a.treeModalOpen, a.treeModalKind = true, 3 }, "目录名称"},
	}
	for _, tc := range cases {
		a := newTestApp(t)
		tc.open(a)
		tt := ui.NewTester(a.view, 900, 700)
		tt.Frame()
		if !tt.HasText(tc.want) {
			t.Fatalf("%s: dialog lacks %q", tc.name, tc.want)
		}
	}
}

// TestSwitchingToOffscreenTabRevealsIt checks selecting an already-open tab
// that scrolled out of the strip from the tree scrolls it back into view.
func TestSwitchingToOffscreenTabRevealsIt(t *testing.T) {
	a, keys := tabTestApp(t, 12)
	tt := ui.NewTester(a.view, 900, 700)
	tt.Frame()

	// Scroll the strip to its far end, as many open tabs leave it.
	a.tabScroll.X = 1e9
	tt.Frame()
	if a.tabScroll.X < 100 {
		t.Fatalf("setup did not scroll the strip: X=%v", a.tabScroll.X)
	}

	pid, sid, oid, cid := splitKey(keys[0])
	a.selectTree(treeItem{kind: "call", pid: pid, sid: sid, oid: oid, cid: cid})
	tt.Frame()
	tt.Frame()
	if a.tabScroll.X > 60 {
		t.Fatalf("the first tab was not scrolled into view: X=%v", a.tabScroll.X)
	}
}

// TestCloseTabsBulk checks the close-other/left/right operations keep the right
// tabs and move the selection to a survivor.
func TestCloseTabsBulk(t *testing.T) {
	a, keys := tabTestApp(t, 5)
	a.closeOtherTabs(keys[2])
	if len(a.tabs) != 1 || a.tabs[0] != keys[2] {
		t.Fatalf("closeOtherTabs left %v", a.tabs)
	}

	a2, keys2 := tabTestApp(t, 5)
	a2.closeTabsOnSide(keys2[2], -1)
	if len(a2.tabs) != 3 || a2.tabs[0] != keys2[2] || a2.tabs[2] != keys2[4] {
		t.Fatalf("close left of 2 left %v", a2.tabs)
	}

	a3, keys3 := tabTestApp(t, 5)
	a3.closeTabsOnSide(keys3[2], 1)
	if len(a3.tabs) != 3 || a3.tabs[0] != keys3[0] || a3.tabs[2] != keys3[2] {
		t.Fatalf("close right of 2 left %v", a3.tabs)
	}
}
