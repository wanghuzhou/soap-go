package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"soapgo/internal/model"
	"soapgo/internal/soapuiproj"
	"soapgo/internal/store"
	"soapgo/internal/transport"
	"soapgo/internal/workspace"
	"soapgo/internal/wsdl"
	"soapgo/internal/wsdlfetch"
)

// headerRow is one editable request header override.
type headerRow struct {
	Name  string
	Value string
}

// uiState is the part of the UI kept between runs: the theme and the split
// layout, like the browser app's localStorage.
type uiState struct {
	Dark      bool    `json:"dark"`
	SplitSide int     `json:"splitSide"`
	SideW     float32 `json:"sideW"`
	SplitV    float32 `json:"splitV"`
	SplitH    float32 `json:"splitH"`
}

// app is the whole application state; the view is a function of it.
type app struct {
	win *mygo.Window

	reg  *workspace.Registry
	st   *store.Store
	info workspace.Info

	dark       bool
	darkTheme  *ui.Theme
	lightTheme *ui.Theme

	sideW     float32
	splitV    float32
	splitH    float32
	splitSide int // 0 上下, 1 左右

	projects []model.Project

	// Selection.
	pid, sid, oid, cid string

	// Current request form.
	endpoint    string
	action      string
	contentType string
	timeout     string
	headers     []headerRow
	projHeaders []model.Header
	xmlEd       *editor.State
	respEd      *editor.State
	reqSection  int

	// Response.
	hasResp   bool
	resp      model.SendResponse
	respSaved bool
	respTab   int
	sending    bool
	sendCancel context.CancelFunc

	// Tabs of open calls, keyed "pid/sid/oid/cid".
	tabs   []string
	active int

	// Tree.
	expanded map[string]bool
	search   string

	// Dialogs.
	importOpen     bool
	importKind     int // 0 chooser, 1 soapUI, 2 WSDL file, 3 WSDL URL
	importURL      string
	importProxy    int // 0 direct, 1 system, 2 current project
	importProxySel string
	exportOpen     bool
	keepProxy      bool
	proxyOpen      bool
	newProjectOpen bool
	projectModalKind int // 0 create, 1 rename
	treeModalOpen  bool
	treeModalKind  int // 1 add service, 2 add operation, 3 rename call
	confirmOpen    bool
	confirmText    string
	confirmAction  func()
	wsOpen         bool
	wsKind       int // 0 create, 1 rename, 2 import
	wsImportPath string
	nameInput      string

	// Editor preferences and dirty tracking.
	maskSecrets   bool
	autoFormat    bool
	savedSnapshot string
	savedHeaders  []headerRow
	savedTimeout  string

	// Proxy form.
	proxyModeSel, proxyTypeSel, proxyHost, proxyUser, proxyPass string
	proxyPort                                                   string

	statusMsg string
	errMsg    string

	uiPath    string
	savedUI   uiState
	lastUISave time.Time
}

func newApp(reg *workspace.Registry) *app {
	st, info := reg.Active()
	a := &app{
		reg:         reg,
		st:          st,
		info:        info,
		dark:        true,
		darkTheme:   newDarkTheme(),
		lightTheme:  newLightTheme(),
		sideW:       272,
		splitV:      60,
		splitH:      62,
		splitSide:      0,
				timeout:     "60000",
		contentType: "text/xml; charset=utf-8",
		xmlEd:       newXMLState("", false),
		respEd:      newXMLState("", true),
		expanded:    map[string]bool{},
		proxyModeSel:   "系统代理",
		proxyTypeSel:   "HTTP",
		importProxySel: "直连（默认）",
		maskSecrets:    true,
	}
	a.uiPath = filepath.Join(reg.DataDir(), "ui-state.json")
	a.loadUIState()
	a.reload()
	a.openFirstCall()
	return a
}

func (a *app) currentUI() uiState {
	return uiState{Dark: a.dark, SplitSide: a.splitSide, SideW: a.sideW, SplitV: a.splitV, SplitH: a.splitH}
}

func (a *app) loadUIState() {
	data, err := os.ReadFile(a.uiPath)
	if err == nil {
		var st uiState
		if json.Unmarshal(stripBOM(data), &st) == nil {
			a.dark = st.Dark
			a.splitSide = st.SplitSide
			if st.SideW > 100 {
				a.sideW = st.SideW
			}
			if st.SplitV > 5 && st.SplitV < 95 {
				a.splitV = st.SplitV
			}
			if st.SplitH > 5 && st.SplitH < 95 {
				a.splitH = st.SplitH
			}
		}
	}
	a.savedUI = a.currentUI()
}

func (a *app) saveUIState() {
	st := a.currentUI()
	if st == a.savedUI {
		return
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return
	}
	if os.WriteFile(a.uiPath, data, 0o644) == nil {
		a.savedUI = st
	}
}

// maybeSaveUIState persists split sizes changed by dragging, at most about
// once a second so a drag does not write the file every frame.
func (a *app) maybeSaveUIState() {
	if a.currentUI() == a.savedUI {
		return
	}
	if time.Since(a.lastUISave) < 800*time.Millisecond {
		return
	}
	a.lastUISave = time.Now()
	a.saveUIState()
}

// newXMLState builds a code editor state with XML highlighting.
func newXMLState(text string, readOnly bool) *editor.State {
	buf := editor.NewBuffer(text)
	buf.SetLanguage(editor.LanguageForText("request.xml", text))
	return &editor.State{Buffer: buf, ReadOnly: readOnly, SoftWrap: true}
}

func (a *app) theme() *ui.Theme {
	if a.dark {
		return a.darkTheme
	}
	return a.lightTheme
}

// reload re-reads the active workspace's projects.
func (a *app) reload() {
	a.projects = a.st.List()
}

// expandedAt reports whether a tree node shows its children.
func (a *app) expandedAt(id string, fallback bool) bool {
	v, ok := a.expanded[id]
	if !ok {
		return fallback
	}
	return v
}

func (a *app) toggleExpanded(id string, fallback bool) {
	a.expanded[id] = !a.expandedAt(id, fallback)
}

// findProject returns the project with the given id.
func (a *app) findProject(id string) *model.Project {
	for i := range a.projects {
		if a.projects[i].ID == id {
			return &a.projects[i]
		}
	}
	return nil
}

// locate returns pointers to the selected project, service, operation and call.
func (a *app) locate() (p *model.Project, s *model.Service, o *model.Operation, c *model.Call) {
	p = a.findProject(a.pid)
	if p == nil {
		return
	}
	for i := range p.Services {
		if p.Services[i].ID == a.sid {
			s = &p.Services[i]
			break
		}
	}
	if s == nil {
		return
	}
	for i := range s.Operations {
		if s.Operations[i].ID == a.oid {
			o = &s.Operations[i]
			break
		}
	}
	if o == nil {
		return
	}
	for i := range o.Calls {
		if o.Calls[i].ID == a.cid {
			c = &o.Calls[i]
			break
		}
	}
	return
}

// openFirstCall opens the first saved call so the workbench starts populated.
func (a *app) openFirstCall() {
	for _, p := range a.projects {
		for _, s := range p.Services {
			for _, o := range s.Operations {
				if len(o.Calls) > 0 {
					a.openCall(p.ID, s.ID, o.ID, o.Calls[0].ID)
					return
				}
			}
		}
	}
}

// openCall selects a call, opens a tab for it and loads it into the editor.
func (a *app) openCall(pid, sid, oid, cid string) {
	a.pid, a.sid, a.oid, a.cid = pid, sid, oid, cid
	_, _, _, c := a.locate()
	if c == nil {
		return
	}
	if p := a.findProject(pid); p != nil {
		a.projHeaders = append([]model.Header(nil), p.Headers...)
	}
	a.endpoint = c.Endpoint
	a.action = c.Action
	a.timeout = strconv.Itoa(model.ClampTimeout(c.TimeoutMS))
	if a.timeout == "0" {
		a.timeout = strconv.Itoa(model.DefaultTimeoutMS)
	}
	a.headers = make([]headerRow, 0, len(c.Headers))
	for _, h := range c.Headers {
		a.headers = append(a.headers, headerRow{Name: h.Name, Value: h.Value})
	}
	body := c.Request
	if a.autoFormat {
		if out, err := formatXML(body); err == nil {
			body = out
		}
	}
	a.xmlEd.Buffer.Reset(body)
	a.xmlEd.Buffer.SetLanguage(editor.LanguageForText("request.xml", body))
	a.savedSnapshot = body
	a.savedHeaders = append([]headerRow(nil), a.headers...)
	a.savedTimeout = a.timeout
	if c.Response != nil {
		a.hasResp = true
		a.resp = model.SendResponse{
			Status:     c.Response.Status,
			StatusText: c.Response.StatusText,
			Headers:    c.Response.Headers,
			Body:       c.Response.Body,
			DurationMS: c.Response.DurationMS,
			Size:       c.Response.Size,
			Truncated:  c.Response.Truncated,
		}
		a.respSaved = true
	} else {
		a.hasResp = false
		a.resp = model.SendResponse{}
		a.respSaved = false
	}
	a.syncResponse()

	key := callKey(pid, sid, oid, cid)
	if !a.hasTabKey(key) {
		a.tabs = append(a.tabs, key)
	}
	for i, k := range a.tabs {
		if k == key {
			a.active = i
		}
	}
}

func callKey(pid, sid, oid, cid string) string {
	return pid + "/" + sid + "/" + oid + "/" + cid
}

func splitKey(key string) (pid, sid, oid, cid string) {
	parts := strings.Split(key, "/")
	if len(parts) != 4 {
		return "", "", "", ""
	}
	return parts[0], parts[1], parts[2], parts[3]
}

func (a *app) hasTabKey(key string) bool {
	for _, k := range a.tabs {
		if k == key {
			return true
		}
	}
	return false
}

func (a *app) closeTab(i int) {
	if i < 0 || i >= len(a.tabs) {
		return
	}
	a.tabs = append(a.tabs[:i], a.tabs[i+1:]...)
	if len(a.tabs) == 0 {
		a.active = -1
		return
	}
	if a.active >= len(a.tabs) {
		a.active = len(a.tabs) - 1
	}
	pid, sid, oid, cid := splitKey(a.tabs[a.active])
	a.openCall(pid, sid, oid, cid)
}

// callTitle returns the name shown for a call key.
func (a *app) callTitle(key string) string {
	pid, sid, oid, cid := splitKey(key)
	p := a.findProject(pid)
	if p == nil {
		return "请求"
	}
	for _, s := range p.Services {
		if s.ID != sid {
			continue
		}
		for _, o := range s.Operations {
			if o.ID != oid {
				continue
			}
			for _, c := range o.Calls {
				if c.ID == cid {
					return c.Title
				}
			}
			return o.Name
		}
	}
	return "请求"
}

// selectedProject returns the project of the selection, if any.
func (a *app) selectedProject() *model.Project {
	return a.findProject(a.pid)
}

// saveCall writes the form back to the selected call.
func (a *app) saveCall() {
	p, s, o, c := a.locate()
	if p == nil || s == nil || o == nil || c == nil {
		a.errMsg = "请先选择一个请求样例"
		return
	}
	call := *c
	call.Endpoint = strings.TrimSpace(a.endpoint)
	call.Action = strings.TrimSpace(a.action)
	call.Request = a.xmlEd.Buffer.String()
	call.TimeoutMS = parseTimeout(a.timeout)
	call.Headers = headersFromRows(a.headers)
	if err := a.st.UpdateCall(p.ID, s.ID, o.ID, call); err != nil {
		a.errMsg = "保存失败：" + err.Error()
		return
	}
	a.savedSnapshot = call.Request
	a.savedHeaders = append([]headerRow(nil), a.headers...)
	a.savedTimeout = a.timeout
	a.errMsg = ""
	a.statusMsg = "已保存：" + call.Title
	a.reload()
}

// deleteCall removes the selected sample, after confirmation.
func (a *app) deleteCall() {
	p, s, o, c := a.locate()
	if p == nil || s == nil || o == nil || c == nil {
		a.errMsg = "请先选择一个请求样例"
		return
	}
	a.askConfirm("确定删除该请求样例？", func() {
		if err := a.st.DeleteCall(p.ID, s.ID, o.ID, c.ID); err != nil {
			a.errMsg = "删除失败：" + err.Error()
			return
		}
		a.statusMsg = "已删除请求样例"
		a.aFromCallRemoved(p.ID, s.ID, o.ID, c.ID)
		a.reload()
	})
}

// aFromCallRemoved clears the editor when its call is gone.
func (a *app) aFromCallRemoved(pid, sid, oid, cid string) {
	key := callKey(pid, sid, oid, cid)
	for i, k := range a.tabs {
		if k == key {
			a.tabs = append(a.tabs[:i], a.tabs[i+1:]...)
			break
		}
	}
	a.pid, a.sid, a.oid, a.cid = "", "", "", ""
	a.endpoint, a.action = "", ""
	a.headers = nil
	a.xmlEd.Buffer.Reset("")
	a.hasResp = false
}

// newCall adds a fresh sample under the selected operation.
func (a *app) newCall() {
	p, s, o, _ := a.locate()
	if p == nil || s == nil || o == nil {
		a.errMsg = "请先选择一个操作"
		return
	}
	call, err := a.st.AddCall(p.ID, s.ID, o.ID, model.Call{
		Title:    "新请求",
		Endpoint: o.Endpoint,
		Action:   o.SoapAction,
		Request:  o.DefaultRequestBody(),
	})
	if err != nil {
		a.errMsg = "新建请求失败：" + err.Error()
		return
	}
	a.reload()
	a.openCall(p.ID, s.ID, o.ID, call.ID)
	a.statusMsg = "已新建请求样例"
}

// duplicateCall copies the selected sample.
func (a *app) duplicateCall() {
	p, s, o, c := a.locate()
	if p == nil || s == nil || o == nil || c == nil {
		a.errMsg = "请先选择一个请求样例"
		return
	}
	copied, err := a.st.DuplicateCall(p.ID, s.ID, o.ID, c.ID)
	if err != nil {
		a.errMsg = "复制失败：" + err.Error()
		return
	}
	a.reload()
	a.openCall(p.ID, s.ID, o.ID, copied.ID)
	a.statusMsg = "已复制请求样例"
}

// deleteOperation removes the selected operation and everything under it.
func (a *app) deleteOperation() {
	p, s, o, _ := a.locate()
	if p == nil || s == nil || o == nil {
		a.errMsg = "请先选择一个操作"
		return
	}
	a.askConfirm("确定删除该操作及其下所有请求？", func() {
		if err := a.st.DeleteOperation(p.ID, s.ID, o.ID); err != nil {
			a.errMsg = "删除操作失败：" + err.Error()
			return
		}
		a.clearAfterDelete()
		a.statusMsg = "已删除操作"
		a.reload()
	})
}

// addService creates a service in the selected project.
func (a *app) addService(name string) {
	p := a.selectedProject()
	if p == nil {
		a.errMsg = "请先选择一个项目"
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "新服务"
	}
	if _, err := a.st.AddService(p.ID, model.Service{Name: name}); err != nil {
		a.errMsg = "新建服务失败：" + err.Error()
		return
	}
	a.statusMsg = "已新建服务：" + name
	a.reload()
}

// addOperation creates an operation in the selected service.
func (a *app) addOperation(name string) {
	p, s, _, _ := a.locate()
	if p == nil || s == nil {
		a.errMsg = "请先选择一个服务"
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "新操作"
	}
	if _, err := a.st.AddOperation(p.ID, s.ID, model.Operation{Name: name}); err != nil {
		a.errMsg = "新建操作失败：" + err.Error()
		return
	}
	a.statusMsg = "已新建操作：" + name
	a.reload()
}

// renameCall changes the selected sample's title.
func (a *app) renameCall(title string) {
	p, s, o, c := a.locate()
	if p == nil || s == nil || o == nil || c == nil {
		a.errMsg = "请先选择一个请求样例"
		return
	}
	title = strings.TrimSpace(title)
	if title == "" {
		a.errMsg = "请求名称不能为空"
		return
	}
	call := *c
	call.Title = title
	if err := a.st.UpdateCall(p.ID, s.ID, o.ID, call); err != nil {
		a.errMsg = "重命名失败：" + err.Error()
		return
	}
	a.statusMsg = "已重命名请求"
	a.reload()
}

// exportProject writes the selected project as SoapUI-compatible XML.
func (a *app) exportProject(path string) {
	p := a.selectedProject()
	if p == nil {
		a.errMsg = "请先选择一个项目"
		return
	}
	data, err := soapuiproj.Marshal(*p)
	if err != nil {
		a.errMsg = "生成 SoapUI XML 失败：" + err.Error()
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		a.errMsg = "写出文件失败：" + err.Error()
		return
	}
	a.statusMsg = "已导出 SoapUI XML：" + filepath.Base(path)
}

// isDirty reports whether the editor holds changes not yet saved.
func (a *app) isDirty() bool {
	if a.cid == "" {
		return false
	}
	if a.xmlEd.Buffer.String() != a.savedSnapshot {
		return true
	}
	if a.timeout != a.savedTimeout {
		return true
	}
	if len(a.headers) != len(a.savedHeaders) {
		return true
	}
	for i := range a.headers {
		if a.headers[i] != a.savedHeaders[i] {
			return true
		}
	}
	return false
}

// askConfirm opens the confirmation dialog and runs fn if the user agrees.
func (a *app) askConfirm(text string, fn func()) {
	a.confirmText = text
	a.confirmAction = fn
	a.confirmOpen = true
}

// deleteService removes the selected service.
func (a *app) deleteService() {
	p, s, _, _ := a.locate()
	if p == nil || s == nil {
		a.errMsg = "请先选择一个服务"
		return
	}
	a.askConfirm("确定删除该服务及其下所有操作？", func() {
		if err := a.st.DeleteService(p.ID, s.ID); err != nil {
			a.errMsg = "删除服务失败：" + err.Error()
			return
		}
		a.clearAfterDelete()
		a.statusMsg = "已删除服务"
		a.reload()
	})
}

// clearAfterDelete resets the editor and tabs after a structural delete.
func (a *app) clearAfterDelete() {
	a.pid, a.sid, a.oid, a.cid = "", "", "", ""
	a.endpoint, a.action = "", ""
	a.headers = nil
	a.projHeaders = nil
	a.xmlEd.Buffer.Reset("")
	a.hasResp = false
	a.tabs = nil
	a.active = -1
}
// saveResponse persists the last response with the selected call.
func (a *app) saveResponse() {
	p, s, o, c := a.locate()
	if p == nil || s == nil || o == nil || c == nil || !a.hasResp {
		a.errMsg = "没有可保存的响应"
		return
	}
	resp := &model.SavedResponse{
		Status:     a.resp.Status,
		StatusText: a.resp.StatusText,
		Headers:    a.resp.Headers,
		Body:       a.resp.Body,
		DurationMS: a.resp.DurationMS,
		Size:       a.resp.Size,
		Truncated:  a.resp.Truncated,
	}
	if err := a.st.UpdateCallResponse(p.ID, s.ID, o.ID, c.ID, resp); err != nil {
		a.errMsg = "保存响应失败：" + err.Error()
		return
	}
	a.respSaved = true
	a.statusMsg = "响应已保存"
	a.reload()
}

// buildSendRequest assembles the payload the transport sends: project headers,
// sample headers, the default SOAPAction and Content-Type, and the editor body.
func (a *app) buildSendRequest() (model.SendRequest, *model.ProxySettings, error) {
	p, _, _, _ := a.locate()
	if p == nil {
		return model.SendRequest{}, nil, fmt.Errorf("请先选择一个请求样例")
	}
	url := strings.TrimSpace(a.endpoint)
	if err := model.ValidateEndpoint(url); err != nil {
		return model.SendRequest{}, nil, err
	}
	hdrs := map[string]string{}
	for _, h := range p.Headers {
		if h.Name != "" {
			hdrs[h.Name] = h.Value
		}
	}
	for _, h := range a.headers {
		if h.Name != "" {
			hdrs[h.Name] = h.Value
		}
	}
	if a.action != "" {
		hdrs["SOAPAction"] = a.action
	}
	hdrs["Content-Type"] = a.contentType

	req := model.SendRequest{
		ProjectID:   p.ID,
		URL:         url,
		Method:      "POST",
		Headers:     hdrs,
		Body:        a.xmlEd.Buffer.String(),
		ContentType: a.contentType,
		TimeoutMS:   parseTimeout(a.timeout),
	}
	var proxy *model.ProxySettings
	if p.Proxy != nil {
		proxy = p.Proxy
	}
	return req, proxy, nil
}

// sendRequest runs the SOAP call off the UI thread.
func (a *app) sendRequest() {
	if a.sending {
		return
	}
	req, proxy, err := a.buildSendRequest()
	if err != nil {
		a.errMsg = err.Error()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.sendCancel = cancel
	a.sending = true
	a.errMsg = ""
	a.statusMsg = "发送中…"
	go func() {
		resp := transport.SendContext(ctx, req, proxy)
		mygo.RunOnMain(func() {
			a.sending = false
			a.sendCancel = nil
			a.hasResp = true
			a.respSaved = false
			a.resp = resp
			switch {
			case resp.Error == "已取消":
				a.errMsg = ""
				a.statusMsg = "已取消"
			case resp.Error != "":
				a.errMsg = "发送失败：" + resp.Error
				a.statusMsg = ""
			default:
				a.statusMsg = fmt.Sprintf("%d %s · %d ms · %s", resp.Status, resp.StatusText, resp.DurationMS, humanSize(resp.Size))
			}
			a.syncResponse()
			if a.win != nil {
				a.win.Invalidate()
			}
		})
	}()
}

// applyWindowButtons makes the native window controls readable on the
// custom title bar: light glyphs on the dark theme, dark ones on the light.
func (a *app) applyWindowButtons() {
	if a.win == nil {
		return
	}
	if a.dark {
		a.win.SetWindowButtonsTheme(mygo.ThemeDark)
	} else {
		a.win.SetWindowButtonsTheme(mygo.ThemeLight)
	}
}
// cancelSend aborts the request in flight, if any.
func (a *app) cancelSend() {
	if !a.sending || a.sendCancel == nil {
		return
	}
	a.sendCancel()
	a.statusMsg = "正在取消…"
}

// syncResponse writes the response body into the read-only editor.
func (a *app) syncResponse() {
	text := a.responseText()
	a.respEd.Buffer.Reset(text)
	a.respEd.Buffer.SetLanguage(editor.LanguageForText("response.xml", text))
}

func (a *app) responseText() string {
	if !a.hasResp {
		return ""
	}
	if a.resp.Error != "" {
		return a.resp.Error
	}
	switch a.respTab {
	case 1: // 原始
		return a.resp.Body
	case 2: // 请求
		return requestPreview(a)
	case 3: // Headers
		var b strings.Builder
		for k, v := range a.resp.Headers {
			b.WriteString(k + ": " + v + "\n")
		}
		return b.String()
	default: // 格式化
		return prettyXML(a.resp.Body)
	}
}

func requestPreview(a *app) string {
	hdrs := map[string]string{}
	for _, h := range a.projHeaders {
		hdrs[h.Name] = h.Value
	}
	for _, h := range a.headers {
		hdrs[h.Name] = h.Value
	}
	if a.action != "" {
		hdrs["SOAPAction"] = a.action
	}
	hdrs["Content-Type"] = a.contentType
	var b strings.Builder
	b.WriteString("POST " + a.endpoint + " HTTP/1.1\n")
	for k, v := range hdrs {
		b.WriteString(k + ": " + v + "\n")
	}
	b.WriteString("\n")
	b.WriteString(a.xmlEd.Buffer.String())
	return b.String()
}

// formatBody pretty-prints the request XML.
func (a *app) formatBody() {
	out, err := formatXML(a.xmlEd.Buffer.String())
	if err != nil {
		a.errMsg = "格式化失败：" + err.Error()
		return
	}
	a.xmlEd.Buffer.Reset(out)
}

// minifyBody collapses inter-tag whitespace.
func (a *app) minifyBody() {
	re := regexp.MustCompile(`>\s+<`)
	s := strings.TrimSpace(a.xmlEd.Buffer.String())
	a.xmlEd.Buffer.Reset(re.ReplaceAllString(s, "><"))
}

// copyCurl puts an equivalent curl command on the clipboard.
func (a *app) copyCurl() {
	var b strings.Builder
	b.WriteString("curl -X POST " + shellQuote(a.endpoint))
	for _, h := range a.projHeaders {
		b.WriteString(" \\\n  -H " + shellQuote(h.Name+": "+h.Value))
	}
	for _, h := range a.headers {
		b.WriteString(" \\\n  -H " + shellQuote(h.Name+": "+h.Value))
	}
	if a.action != "" {
		b.WriteString(" \\\n  -H " + shellQuote("SOAPAction: "+a.action))
	}
	b.WriteString(" \\\n  -H " + shellQuote("Content-Type: "+a.contentType))
	b.WriteString(" \\\n  --data-binary " + shellQuote(a.xmlEd.Buffer.String()))
	mygo.Clipboard.WriteText(b.String())
	a.statusMsg = "curl 已复制"
}

// ---- project & workspace management ----

func (a *app) createProject(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "新建项目"
	}
	saved, err := a.st.AddProject(model.Project{Name: name, Source: "native"})
	if err != nil {
		a.errMsg = "新建项目失败：" + err.Error()
		return
	}
	a.pid, a.sid, a.oid, a.cid = saved.ID, "", "", ""
	a.reload()
	a.statusMsg = "已新建项目：" + saved.Name
}

func (a *app) deleteProject() {
	p := a.selectedProject()
	if p == nil {
		a.errMsg = "请先选择一个项目"
		return
	}
	a.askConfirm("确定删除整个项目「"+p.Name+"」？", func() {
		if err := a.st.DeleteProject(p.ID); err != nil {
			a.errMsg = "删除项目失败：" + err.Error()
			return
		}
		a.clearAfterDelete()
		a.statusMsg = "已删除项目：" + p.Name
		a.reload()
	})
}

func (a *app) renameProject(name string) {
	p := a.selectedProject()
	if p == nil {
		a.errMsg = "请先选择一个项目"
		return
	}
	if err := a.st.RenameProject(p.ID, strings.TrimSpace(name)); err != nil {
		a.errMsg = "重命名失败：" + err.Error()
		return
	}
	a.statusMsg = "项目已重命名"
	a.reload()
}

// importSoapUI parses a SoapUI project file and adds it.
func (a *app) importSoapUI(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		a.errMsg = "读取文件失败：" + err.Error()
		return
	}
	project, err := soapuiproj.Parse(data)
	if err != nil {
		a.errMsg = "解析 SoapUI 项目失败：" + err.Error()
		return
	}
	if project.Name == "" {
		project.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if _, err := a.st.AddProject(*project); err != nil {
		a.errMsg = "保存项目失败：" + err.Error()
		return
	}
	a.reload()
	a.statusMsg = "已导入 SoapUI 项目：" + project.Name
}

// importWSDLFile parses a WSDL file and adds a project.
func (a *app) importWSDLFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		a.errMsg = "读取文件失败：" + err.Error()
		return
	}
	res, err := wsdl.Parse(data)
	if err != nil {
		a.errMsg = "解析 WSDL 失败：" + err.Error()
		return
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	a.persistWSDL(res, name, path, "wsdl")
}

// importWSDLURL downloads and parses a WSDL.
func (a *app) importWSDLURL(rawURL string) {
	proxy := &model.ProxySettings{Mode: "direct"}
	switch a.importProxySel {
	case "系统代理":
		proxy = &model.ProxySettings{Mode: "system"}
	case "当前项目代理":
		if p := a.selectedProject(); p != nil && p.Proxy != nil {
			proxy = p.Proxy
		}
	}
	fetched, err := wsdlfetch.Fetch(strings.TrimSpace(rawURL), proxy)
	if err != nil {
		a.errMsg = "获取 WSDL 失败：" + err.Error()
		return
	}
	res, err := wsdl.Parse(fetched.Data)
	if err != nil {
		a.errMsg = "解析 WSDL 失败：" + err.Error()
		return
	}
	a.persistWSDL(res, fetched.Name, fetched.FinalURL, "wsdl-url")
}

// persistWSDL saves a parsed WSDL as a project with one service and one
// generated sample per operation, as the browser app does.
func (a *app) persistWSDL(res *wsdl.Result, fallbackName, source, projectSource string) {
	name := res.Name
	if name == "" {
		name = fallbackName
	}
	for i := range res.Operations {
		op := &res.Operations[i]
		if op.Template == "" {
			op.Template = op.Request
		}
		if len(op.Calls) == 0 {
			if call, ok := model.SeedCall(op); ok {
				op.Calls = []model.Call{call}
			}
		}
	}
	if name == "" {
		name = "WSDL 项目"
	}
	project := model.Project{
		Name:   name,
		Source: projectSource,
		Services: []model.Service{{
			Name:       name,
			Wsdl:       source,
			Operations: res.Operations,
		}},
	}
	if _, err := a.st.AddProject(project); err != nil {
		a.errMsg = "保存 WSDL 项目失败：" + err.Error()
		return
	}
	a.reload()
	a.statusMsg = "已导入 WSDL：" + name
}

// updateProxy saves the proxy form on the selected project.
func (a *app) updateProxy() {
	p := a.selectedProject()
	if p == nil {
		a.errMsg = "请先选择一个项目"
		return
	}
	var cfg *model.ProxySettings
	switch a.proxyModeSel {
	case "直连":
		cfg = &model.ProxySettings{Mode: "direct"}
	case "自定义代理":
		port, _ := strconv.Atoi(strings.TrimSpace(a.proxyPort))
		typ := "http"
		if a.proxyTypeSel == "SOCKS5" {
			typ = "socks5"
		}
		cfg = &model.ProxySettings{
			Mode: "custom", Type: typ, Host: strings.TrimSpace(a.proxyHost),
			Port: port, Username: a.proxyUser, Password: a.proxyPass,
		}
	default:
		cfg = &model.ProxySettings{Mode: "system"}
	}
	if err := cfg.Validate(); err != nil {
		a.errMsg = "代理配置无效：" + err.Error()
		return
	}
	if err := a.st.UpdateProjectProxy(p.ID, cfg); err != nil {
		a.errMsg = "保存代理失败：" + err.Error()
		return
	}
	a.statusMsg = "代理设置已保存"
	a.proxyOpen = false
	a.reload()
}

// openProxy loads the selected project's proxy settings into the form.
func (a *app) openProxy() {
	p := a.selectedProject()
	if p == nil {
		a.errMsg = "请先选择一个项目"
		return
	}
	a.proxyModeSel, a.proxyTypeSel = "系统代理", "HTTP"
	a.proxyHost, a.proxyPort, a.proxyUser, a.proxyPass = "", "", "", ""
	if p.Proxy != nil {
		switch p.Proxy.Mode {
		case "direct":
			a.proxyModeSel = "直连"
		case "custom":
			a.proxyModeSel = "自定义代理"
		default:
			a.proxyModeSel = "系统代理"
		}
		if p.Proxy.Type == "socks5" {
			a.proxyTypeSel = "SOCKS5"
		}
		a.proxyHost = p.Proxy.Host
		if p.Proxy.Port > 0 {
			a.proxyPort = strconv.Itoa(p.Proxy.Port)
		}
		a.proxyUser = p.Proxy.Username
		a.proxyPass = p.Proxy.Password
	}
	a.proxyOpen = true
}

// ---- workspace management ----

func (a *app) switchWorkspace(id string) {
	if a.isDirty() {
		a.askConfirm("当前请求有未保存的修改，切换工作空间会丢失这些修改。确定继续？", func() {
			a.doSwitchWorkspace(id)
		})
		return
	}
	a.doSwitchWorkspace(id)
}

func (a *app) doSwitchWorkspace(id string) {
	info, err := a.reg.Switch(id)
	if err != nil {
		a.errMsg = "切换工作空间失败：" + err.Error()
		return
	}
	a.st, a.info = a.reg.Active()
	a.pid, a.sid, a.oid, a.cid = "", "", "", ""
	a.tabs = nil
	a.active = -1
	a.hasResp = false
	a.reload()
	a.statusMsg = "已切换工作空间：" + info.Name
}

func (a *app) createWorkspace(name string) {
	if a.isDirty() {
		a.askConfirm("当前请求有未保存的修改，新建工作空间会切换并丢失这些修改。确定继续？", func() {
			a.doCreateWorkspace(name)
		})
		return
	}
	a.doCreateWorkspace(name)
}

func (a *app) doCreateWorkspace(name string) {
	info, err := a.reg.Create(name)
	if err != nil {
		a.errMsg = "新建工作空间失败：" + err.Error()
		return
	}
	a.st, a.info = a.reg.Active()
	a.pid, a.sid, a.oid, a.cid = "", "", "", ""
	a.tabs = nil
	a.active = -1
	a.reload()
	a.statusMsg = "已新建工作空间：" + info.Name
}

func (a *app) renameWorkspace(name string) {
	if _, err := a.reg.Rename(a.info.ID, name); err != nil {
		a.errMsg = "重命名工作空间失败：" + err.Error()
		return
	}
	a.info = a.reg.CurrentInfo()
	a.statusMsg = "工作空间已重命名"
}

func (a *app) deleteWorkspace() {
	a.askConfirm("确定删除当前工作空间「"+a.info.Name+"」？\n工作空间文件会先备份到 trash。", func() {
		if err := a.reg.Delete(a.info.ID); err != nil {
			a.errMsg = "删除工作空间失败：" + err.Error()
			return
		}
		a.st, a.info = a.reg.Active()
		a.pid, a.sid, a.oid, a.cid = "", "", "", ""
		a.tabs = nil
		a.active = -1
		a.hasResp = false
		a.reload()
		a.statusMsg = "已删除工作空间"
	})
}

// exportWorkspace writes the active workspace as a project array.
func (a *app) exportWorkspace(path string) {
	projects := a.st.List()
	if !a.keepProxy {
		for i := range projects {
			projects[i].Proxy = nil
		}
	}
	data, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		a.errMsg = "导出失败：" + err.Error()
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		a.errMsg = "写出文件失败：" + err.Error()
		return
	}
	a.statusMsg = "已导出工作空间：" + filepath.Base(path)
}

// importWorkspace reads a project array and creates a workspace from it.
func (a *app) importWorkspace(path string) {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	a.importWorkspaceNamed(path, name)
}

// importWorkspaceNamed imports with an explicit workspace name.
func (a *app) importWorkspaceNamed(path, name string) {
	if a.isDirty() {
		a.askConfirm("当前请求有未保存的修改，导入工作空间会切换并丢失这些修改。确定继续？", func() {
			a.doImportWorkspace(path, name)
		})
		return
	}
	a.doImportWorkspace(path, name)
}

func (a *app) doImportWorkspace(path, name string) {
	data, err := os.ReadFile(path)
	if err != nil {
		a.errMsg = "读取文件失败：" + err.Error()
		return
	}
	var projects []model.Project
	if err := json.Unmarshal(stripBOM(data), &projects); err != nil {
		a.errMsg = "不是有效的工作空间 JSON：" + err.Error()
		return
	}
	if strings.TrimSpace(name) == "" {
		name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if _, err := a.reg.Import(name, projects); err != nil {
		a.errMsg = "导入工作空间失败：" + err.Error()
		return
	}
	a.st, a.info = a.reg.Active()
	a.reload()
	a.statusMsg = "已导入工作空间：" + name
}

// ---- helpers ----

func parseTimeout(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n <= 0 {
		return model.DefaultTimeoutMS
	}
	return model.ClampTimeout(n)
}

func headersFromRows(rows []headerRow) []model.Header {
	out := make([]model.Header, 0, len(rows))
	for _, r := range rows {
		if strings.TrimSpace(r.Name) == "" {
			continue
		}
		out = append(out, model.Header{Name: r.Name, Value: r.Value})
	}
	return out
}

func humanSize(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func stripBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

var tagRe = regexp.MustCompile(`(?s)<[^>]*>|[^<]+`)

// formatXML re-indents an XML document while keeping every tag and prefix as
// written (encoding/xml would rewrite names, which breaks SOAP).
func formatXML(s string) (string, error) {
	tokens := tagRe.FindAllString(strings.TrimSpace(s), -1)
	if len(tokens) == 0 {
		return s, nil
	}
	var b strings.Builder
	depth := 0
	for _, tok := range tokens {
		t := strings.TrimSpace(tok)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "<") {
			closing := strings.HasPrefix(t, "</")
			self := strings.HasSuffix(t, "/>")
			decl := strings.HasPrefix(t, "<?") || strings.HasPrefix(t, "<!")
			if closing {
				depth--
				if depth < 0 {
					depth = 0
				}
			}
			b.WriteString(strings.Repeat("  ", depth))
			b.WriteString(t)
			b.WriteString("\n")
			if !closing && !self && !decl {
				depth++
			}
		} else {
			b.WriteString(strings.Repeat("  ", depth))
			b.WriteString(t)
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// prettyXML formats a response for display, leaving malformed bodies as-is.
func prettyXML(s string) string {
	out, err := formatXML(s)
	if err != nil {
		return s
	}
	return out
}
