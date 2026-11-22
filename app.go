package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"soapgo/internal/fileutil"
	"soapgo/internal/model"
	"soapgo/internal/openapi"
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

// queryRow is one editable query-string parameter. The rows are derived from
// and written back to the endpoint URL, so they need no separate persistence.
type queryRow struct {
	Key   string
	Value string
}

// bodyField is one editable name/value pair of a urlencoded or multipart body.
// Type is "" or "text" for a text value, and "file" when Value names a file to
// upload (multipart only).
type bodyField struct {
	Name  string
	Value string
	Type  string
}

// authForm is the request form's authentication: the same fields as
// model.AuthSettings plus the "none" type the UI shows for no auth.
type authForm struct {
	Type     string // "none", "basic" or "bearer"
	Enabled  bool
	Username string
	Password string
	Token    string
	Prefix   string
}

// modelAuth converts the form to what the store persists; "none" is nil.
func (f authForm) modelAuth() *model.AuthSettings {
	if f.Type != "basic" && f.Type != "bearer" {
		return nil
	}
	prefix := f.Prefix
	if f.Type == "bearer" && strings.TrimSpace(prefix) == "" {
		prefix = "Bearer"
	}
	return &model.AuthSettings{
		Type: f.Type, Enabled: f.Enabled,
		Username: f.Username, Password: f.Password,
		Token: f.Token, Prefix: prefix,
	}
}

// authFormOf loads a stored call's auth into the form, defaulting the type to
// "none" and the Bearer prefix to "Bearer".
func authFormOf(a *model.AuthSettings) authForm {
	f := authForm{Type: "none", Enabled: true}
	if a == nil {
		return f
	}
	if a.Type == "basic" || a.Type == "bearer" {
		f.Type = a.Type
	}
	f.Enabled = a.Enabled
	f.Username, f.Password = a.Username, a.Password
	f.Token, f.Prefix = a.Token, a.Prefix
	if f.Type == "bearer" && strings.TrimSpace(f.Prefix) == "" {
		f.Prefix = "Bearer"
	}
	return f
}

// setAuthType switches the authentication type. Picking Basic or Bearer turns
// it on and defaults the Bearer scheme word; picking 无认证 carries no header.
func (a *app) setAuthType(kind string) {
	if kind != "basic" && kind != "bearer" {
		kind = "none"
	}
	if a.auth.Type == kind {
		return
	}
	a.auth.Type = kind
	if kind != "none" {
		a.auth.Enabled = true
		if kind == "bearer" && strings.TrimSpace(a.auth.Prefix) == "" {
			a.auth.Prefix = "Bearer"
		}
	}
	a.showAuthSecret = false
}

// formValues is the editable part of a request form: exactly the fields a tab's
// draft copies on a switch and the values the dirty check compares.
type formValues struct {
	endpoint, action, timeout    string
	method, contentType, reqKind string
	bodyType, bodyFile           string
	queryRows                    []queryRow
	bodyFields                   []bodyField
	headers                      []headerRow
	auth                         authForm
}

// savedValues is a form as it was last saved. The fields keep their "saved"
// prefix so they can sit beside the live ones in app and callDraft without a
// name clash.
type savedValues struct {
	savedSnapshot, savedAction    string
	savedTimeout, savedEndpoint   string
	savedMethod, savedContentType string
	savedBodyType, savedBodyFile  string
	savedBodyFields               []bodyField
	savedHeaders                  []headerRow
	savedAuth                     authForm
}

// callDraft is one open tab's editable request form and the values it was last
// saved with. Keeping a draft per tab means switching tabs keeps unsaved edits,
// and the tab still knows whether it differs from what is stored. body is the
// request body text, which lives in the editor while the tab is active.
type callDraft struct {
	formValues
	savedValues
	body string
}

// uiState is the part of the UI kept between runs: the theme, split layout and
// open tabs, persisted beside the workspace.
type uiState struct {
	Dark      bool    `json:"dark"`
	SplitSide int     `json:"splitSide"`
	SideW     float32 `json:"sideW"`
	SplitV    float32 `json:"splitV"`
	SplitH    float32 `json:"splitH"`
	// Font preferences: the family ("" is the system's) and the size in DIPs,
	// kept separately for the interface and for the code editor.
	UIFont         string  `json:"uiFont,omitempty"`
	UIFontSize     float64 `json:"uiFontSize,omitempty"`
	EditorFont     string  `json:"editorFont,omitempty"`
	EditorFontSize float64 `json:"editorFontSize,omitempty"`
	// Accent is the picked theme color, or "" for the theme's default.
	Accent string `json:"accent,omitempty"`
	// Tabs are the request tabs left open, as "pid/sid/oid/cid" keys joined by
	// newlines, and ActiveTab the index of the one shown; TabsSaved tells an
	// empty tab set (the user closed everything) from a state never saved.
	Tabs      string `json:"tabs,omitempty"`
	ActiveTab int    `json:"activeTab,omitempty"`
	TabsSaved bool   `json:"tabsSaved,omitempty"`
	// MaxTabs is the tab-count limit; 0 in a file written before the setting
	// existed means the default applies.
	MaxTabs int `json:"maxTabs,omitempty"`
}

// The tab-count limit: opening a tab past it closes the earliest opened one
// that has no unsaved edits.
const (
	defaultTabLimit = 20
	minTabLimit     = 5
	maxTabLimit     = 100
)

// app is the whole application state; the view is a function of it.
type app struct {
	win *mygo.Window

	reg  *workspace.Registry
	st   *store.Store
	info workspace.Info

	dark       bool
	darkTheme  *ui.Theme
	lightTheme *ui.Theme
	// accent is the picked theme color ("" for the theme's default).
	accent string

	sideW     float32
	splitV    float32
	splitH    float32
	splitSide int // 0 上下, 1 左右

	projects []model.Project

	// projectPage shows the selected project's overview page in the main area
	// instead of the active request tab; hostInput is its host draft.
	projectPage bool
	hostInput   string

	// Selection.
	pid, sid, oid, cid string
	// selGroup is the selected folder id in the native tree ("" when the
	// selection is a project, service, operation or call).
	selGroup string
	// newGroupParent is the parent a pending 新建目录 dialog will create under.
	newGroupParent string

	// Current request form: the editable values and the values last saved, which
	// the dirty check compares. Embedded so the rest of the app keeps writing
	// a.endpoint, a.savedAction and the like.
	formValues
	savedValues
	// reqDescription is the selected request's notes (carried over from an
	// OpenAPI import), shown read-only in the 设置 tab.
	reqDescription string
	// querySrc is the endpoint the query rows were parsed from, so an edit in
	// the URL bar is noticed and re-parsed instead of being overwritten.
	querySrc string
	// showAuthSecret reveals the auth password/token for as long as the
	// authentication page stays open.
	showAuthSecret bool
	projHeaders    []model.Header
	xmlEd          *editor.State
	respEd         *editor.State
	reqSection     int
	timeoutRef     ui.Ref
	// reqLangKey is the kind/body-type/content-type the request editor's
	// syntax highlighting was last set for, so it only re-resolves when one
	// of them changes rather than every frame.
	reqLangKey string
	// binCache* keep the last binary body read, keyed by path, size and
	// modification time, so switching to the 请求 response tab does not re-read
	// a large file every time.
	binCachePath string
	binCacheSize int64
	binCacheMod  time.Time
	binCacheData string

	// Response.
	hasResp    bool
	resp       model.SendResponse
	respTab    int
	sending    bool
	sendCancel context.CancelFunc

	// Tabs of open calls, keyed "pid/sid/oid/cid", and their unsaved drafts.
	tabs   []string
	active int
	drafts map[string]*callDraft
	// maxTabs is the tab-count limit set in the settings dialog.
	maxTabs int
	// maxTabsText is the manual input for that limit, kept as text so a
	// half-typed value is not clamped mid-edit.
	maxTabsText string
	// tabScroll keeps the horizontal scroll position of the tab strip, so a
	// wheel over the tabs moves them sideways and a new tab scrolls into view.
	tabScroll ui.ScrollState
	// revealTab asks the tab strip to scroll the active tab into view on the
	// next frame, set whenever the editor switches tabs (a tree click on an
	// already open request, Ctrl+Tab, and so on).
	revealTab bool
	// pendingTabs is the tab state loaded from disk before the projects are
	// read, restored once they are; pendingTabsSaved tells whether it was set.
	pendingTabs      []string
	pendingActive    int
	pendingTabsSaved bool
	// quitting is set once the user confirmed quitting with unsaved edits, so
	// the quit and window-close guards let the next attempt through.
	quitting bool

	// Tree.
	expanded  map[string]bool
	search    string
	searchRef ui.Ref

	// Inline rename in the tree: the node id being edited and its draft name.
	renamingID    string
	renameInput   string
	renameFocused bool
	renameRef     ui.Ref

	// Dialogs.
	importOpen     bool
	importKind     int // 0 chooser, 1 soapUI, 2 WSDL file, 3 WSDL URL
	importURL      string
	importProxySel string
	// importing is set while a WSDL URL import downloads in the background, so
	// the dialog can show progress and refuse a second start.
	importing      bool
	exportOpen     bool
	keepProxy      bool
	newProjectOpen bool
	treeModalOpen  bool
	treeModalKind  int // 1 add service, 2 add operation
	confirmOpen    bool
	confirmText    string
	confirmLabel   string
	confirmAction  func()
	// confirmAltLabel / confirmAltAction are the optional second action of the
	// confirmation dialog (e.g. "save all" beside "quit").
	confirmAltLabel  string
	confirmAltAction func()
	// Bulk edit of the multipart / urlencoded field rows, as text.
	bulkOpen     bool
	bulkText     string
	wsOpen       bool
	wsKind       int // 0 create, 1 rename, 2 import
	wsImportPath string
	nameInput    string

	// Settings dialog: the section shown, the search filter, and the font
	// preferences (a choice is "系统默认" or a family name).
	settingsOpen     bool
	settingsSection  int
	settingsQuery    string
	uiFontChoice     string
	uiFontSize       float64
	editorFontChoice string
	editorFontSize   float64

	// Editor preferences.
	maskSecrets bool

	// Proxy form.
	proxyModeSel, proxyTypeSel, proxyHost, proxyUser, proxyPass string
	proxyPort                                                   string

	statusMsg string
	// statusAt is when statusMsg was set, and statusHold keeps it on screen
	// until replaced (for long-running actions such as sending).
	statusAt   time.Time
	statusHold bool
	errMsg     string

	uiPath     string
	savedUI    uiState
	lastUISave time.Time
}

func newApp(reg *workspace.Registry) *app {
	st, info := reg.Active()
	a := &app{
		reg:        reg,
		st:         st,
		info:       info,
		dark:       true,
		darkTheme:  newDarkTheme(),
		lightTheme: newLightTheme(),
		sideW:      272,
		splitV:     60,
		splitH:     62,
		splitSide:  0,
		formValues: formValues{
			timeout: "60000", method: "POST", reqKind: model.KindHTTP,
			bodyType: model.BodyNone, auth: authForm{Type: "none", Enabled: true},
		},
		savedValues:      savedValues{savedAuth: authForm{Type: "none", Enabled: true}},
		xmlEd:            newXMLState("", false),
		respEd:           newXMLState("", true),
		expanded:         map[string]bool{},
		drafts:           map[string]*callDraft{},
		proxyModeSel:     "系统代理",
		proxyTypeSel:     "HTTP",
		importProxySel:   "直连（默认）",
		maskSecrets:      true,
		uiFontChoice:     systemFontLabel,
		uiFontSize:       uiBaseFontSize(),
		editorFontChoice: systemFontLabel,
		editorFontSize:   13,
		maxTabs:          defaultTabLimit,
		maxTabsText:      strconv.Itoa(defaultTabLimit),
	}
	a.uiPath = filepath.Join(reg.DataDir(), "ui-state.json")
	a.loadUIState()
	a.reload()
	if !a.restoreTabs() {
		a.openFirstCall()
	}
	return a
}

func (a *app) currentUI() uiState {
	return uiState{
		Dark: a.dark, SplitSide: a.splitSide, SideW: a.sideW, SplitV: a.splitV, SplitH: a.splitH,
		UIFont: a.uiFontChoice, UIFontSize: a.uiFontSize,
		EditorFont: a.editorFontChoice, EditorFontSize: a.editorFontSize,
		Accent:    a.accent,
		Tabs:      strings.Join(a.tabs, "\n"),
		ActiveTab: a.active,
		TabsSaved: true,
		MaxTabs:   a.maxTabs,
	}
}

func (a *app) loadUIState() {
	data, err := os.ReadFile(a.uiPath)
	if err == nil {
		var st uiState
		if json.Unmarshal(fileutil.StripBOM(data), &st) == nil {
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
			a.uiFontChoice = normalizeFontChoice(st.UIFont, uiFontOptions)
			if st.UIFontSize > 0 {
				a.uiFontSize = st.UIFontSize
			}
			a.editorFontChoice = normalizeFontChoice(st.EditorFont, editorFontOptions)
			if st.EditorFontSize > 0 {
				a.editorFontSize = st.EditorFontSize
			}
			a.accent = normalizeAccent(st.Accent)
			if st.MaxTabs >= minTabLimit && st.MaxTabs <= maxTabLimit {
				a.maxTabs = st.MaxTabs
			}
			a.maxTabsText = strconv.Itoa(a.maxTabs)
			a.pendingTabsSaved = st.TabsSaved
			a.pendingActive = st.ActiveTab
			for _, k := range strings.Split(st.Tabs, "\n") {
				if k = strings.TrimSpace(k); k != "" {
					a.pendingTabs = append(a.pendingTabs, k)
				}
			}
		}
	}
	a.savedUI = a.currentUI()
}

// saveUIState persists the UI state, through a temp file and a rename so an
// interrupted write cannot leave a half file behind.
func (a *app) saveUIState() {
	st := a.currentUI()
	if st == a.savedUI {
		return
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return
	}
	if fileutil.WriteAtomic(a.uiPath, data, 0o644) == nil {
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
	th := a.lightTheme
	if a.dark {
		th = a.darkTheme
	}
	// Apply the user's interface font ("" is the system's) and size. The size
	// also becomes Context's default text size, and the explicit sizes written
	// in the view are scaled by us() to match it.
	th.Font = fontFamily(a.uiFontChoice)
	if a.uiFontSize > 0 {
		th.FontSize = float32(a.uiFontSize)
	}
	// The accent covers POST tags, primary buttons, splitters and focus rings,
	// so one pick repaints the whole chrome.
	applyAccent(th, accentHex(a.accent))
	return th
}

// uiBaseFontSize is the theme's default text size on this platform: the size
// the view's explicit FontSize literals were written against, so us() leaves
// them unchanged at the default setting.
func uiBaseFontSize() float64 {
	if runtime.GOOS == "darwin" {
		return 13
	}
	return 14
}

// us scales a font size written in the view by the user's interface font-size
// setting, so every label grows or shrinks in proportion. At the default the
// base is returned unchanged.
func (a *app) us(base float32) float32 {
	size := a.uiFontSize
	if size <= 0 {
		size = uiBaseFontSize()
	}
	return base * float32(size/uiBaseFontSize())
}

// syncEditorFont applies the editor font (a "" family is the system's
// monospaced font) and size to the request and response editors. It runs each
// frame so a change in the settings dialog shows at once.
func (a *app) syncEditorFont() {
	f := ui.Font{Family: fontFamily(a.editorFontChoice)}
	if a.editorFontSize > 0 {
		f.Size = float32(a.editorFontSize)
	}
	if a.xmlEd != nil {
		a.xmlEd.Font = f
	}
	if a.respEd != nil {
		a.respEd.Font = f
	}
}

// resetFontSettings restores the interface and editor fonts to their defaults.
func (a *app) resetFontSettings() {
	a.uiFontChoice = systemFontLabel
	a.uiFontSize = uiBaseFontSize()
	a.editorFontChoice = systemFontLabel
	a.editorFontSize = 13
}

// systemFontLabel is the dropdown choice that means the system's own font.
const systemFontLabel = "系统默认"

// appVersion is the release version shown in the settings dialog's About
// section.
const appVersion = "v0.6.0"

// uiFontOptions and editorFontOptions are the families the settings dialog
// offers; the first choice is the system's own font. They differ per platform
// because each OS ships its own common typefaces (PingFang on macOS, Segoe UI
// on Windows), so the list only names fonts the user actually has.
var uiFontOptions, editorFontOptions []string

func init() {
	if runtime.GOOS == "darwin" {
		uiFontOptions = []string{
			systemFontLabel, "PingFang SC", "Helvetica Neue", "Helvetica", "Hiragino Sans GB", "Songti SC", "Arial",
		}
		editorFontOptions = []string{
			systemFontLabel, "SF Mono", "Menlo", "Monaco", "JetBrains Mono", "Fira Code", "Courier New",
		}
	} else {
		uiFontOptions = []string{
			systemFontLabel, "Segoe UI", "Microsoft YaHei UI", "微软雅黑", "Arial", "Tahoma", "Verdana",
		}
		editorFontOptions = []string{
			systemFontLabel, "Cascadia Code", "Consolas", "JetBrains Mono", "Fira Code", "Courier New", "Source Code Pro",
		}
	}
}

// fontFamily maps a dropdown choice to a theme font family; "" is the
// system's.
func fontFamily(choice string) string {
	if choice == "" || choice == systemFontLabel {
		return ""
	}
	return choice
}

// normalizeFontChoice maps a stored family back to a dropdown choice from the
// given options. A family those options do not offer—for example one saved on
// another OS—maps to the system font, so the dropdown never shows a blank
// entry.
func normalizeFontChoice(family string, options []string) string {
	if family == "" {
		return systemFontLabel
	}
	for _, f := range options {
		if f == family {
			return family
		}
	}
	return systemFontLabel
}

// setDark switches the theme and persists the choice; both the top bar toggle
// and the settings dialog use it.
func (a *app) setDark(dark bool) {
	if a.dark == dark {
		return
	}
	a.dark = dark
	a.applyWindowButtons()
	a.saveUIState()
}

// setAccent picks the theme color and persists it. An empty string restores
// the current theme's default accent.
func (a *app) setAccent(hex string) {
	if hex != "" && normalizeAccent(hex) == "" {
		return
	}
	if a.accent == hex {
		return
	}
	a.accent = hex
	a.saveUIState()
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
// For a native request (one stored under Project.Calls, not under an operation)
// s and o are nil and c is looked up among the project's own calls.
func (a *app) locate() (p *model.Project, s *model.Service, o *model.Operation, c *model.Call) {
	p = a.findProject(a.pid)
	if p == nil {
		return
	}
	if a.sid != "" {
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
	for i := range p.Calls {
		if p.Calls[i].ID == a.cid {
			c = &p.Calls[i]
			break
		}
	}
	return
}

// findCallByKey resolves an open tab's call, whether it is an operation sample
// or a native project/group request.
func (a *app) findCallByKey(key string) *model.Call {
	pid, sid, oid, cid := splitKey(key)
	p := a.findProject(pid)
	if p == nil {
		return nil
	}
	if sid != "" {
		for si := range p.Services {
			if p.Services[si].ID != sid {
				continue
			}
			for oi := range p.Services[si].Operations {
				if p.Services[si].Operations[oi].ID != oid {
					continue
				}
				for ci := range p.Services[si].Operations[oi].Calls {
					if p.Services[si].Operations[oi].Calls[ci].ID == cid {
						return &p.Services[si].Operations[oi].Calls[ci]
					}
				}
			}
		}
		return nil
	}
	for i := range p.Calls {
		if p.Calls[i].ID == cid {
			return &p.Calls[i]
		}
	}
	return nil
}

// openFirstCall opens the first saved call so the workbench starts populated.
// With no call anywhere, it shows the first project's page instead of a blank
// workbench, so a project's settings stay one click away.
func (a *app) openFirstCall() {
	for _, p := range a.projects {
		if len(p.Calls) > 0 {
			a.openCall(p.ID, "", "", p.Calls[0].ID)
			return
		}
		for _, s := range p.Services {
			for _, o := range s.Operations {
				if len(o.Calls) > 0 {
					a.openCall(p.ID, s.ID, o.ID, o.Calls[0].ID)
					return
				}
			}
		}
	}
	if len(a.projects) > 0 {
		a.openProjectPage(a.projects[0].ID)
	}
}

// restoreTabs reopens the tabs a previous run left open, dropping keys whose
// request no longer exists. It reports whether the saved state was applied:
// false falls back to openFirstCall, as a first run or a workspace whose saved
// tabs have all been deleted does.
func (a *app) restoreTabs() bool {
	if !a.pendingTabsSaved {
		return false
	}
	if len(a.pendingTabs) == 0 {
		// The user had closed every tab: keep the workbench empty.
		a.savedUI = a.currentUI()
		return true
	}
	seen := make(map[string]bool, len(a.pendingTabs))
	kept := make([]string, 0, len(a.pendingTabs))
	for _, k := range a.pendingTabs {
		if seen[k] || a.findCallByKey(k) == nil {
			continue
		}
		seen[k] = true
		kept = append(kept, k)
	}
	if len(kept) == 0 {
		return false
	}
	// A state saved before the limit was lowered (or written by an older
	// version) may hold more tabs than allowed: drop the oldest, which are the
	// first in open order.
	if limit := a.tabLimit(); len(kept) > limit {
		drop := len(kept) - limit
		kept = kept[drop:]
		a.pendingActive -= drop
	}
	a.tabs = kept
	a.active = a.pendingActive
	if a.active < 0 || a.active >= len(a.tabs) {
		a.active = 0
	}
	pid, sid, oid, cid := splitKey(a.tabs[a.active])
	a.pid, a.sid, a.oid, a.cid = pid, sid, oid, cid
	a.selGroup = ""
	a.loadTab(a.tabs[a.active])
	// The restored state is what disk holds, so later tab changes compare
	// against it and get persisted.
	a.savedUI = a.currentUI()
	return true
}

// tabLimit returns the configured tab-count limit, falling back to the default
// when the stored value is unset or out of range.
func (a *app) tabLimit() int {
	if a.maxTabs >= minTabLimit && a.maxTabs <= maxTabLimit {
		return a.maxTabs
	}
	return defaultTabLimit
}

// applyMaxTabsText applies the manually entered tab limit when it is a valid
// number; anything else leaves the current limit in place.
func (a *app) applyMaxTabsText() {
	if v, err := strconv.Atoi(strings.TrimSpace(a.maxTabsText)); err == nil && v >= minTabLimit && v <= maxTabLimit {
		a.maxTabs = v
	}
}

// enforceTabLimit closes the earliest opened tabs with no unsaved edits until
// the strip is within its limit, never closing keep (the tab just opened). When
// every other tab has unsaved edits the limit is exceeded rather than losing
// changes, so the caller keeps the user's work.
func (a *app) enforceTabLimit(keep string) {
	closed := 0
	for len(a.tabs) > a.tabLimit() {
		victim := -1
		for i, k := range a.tabs {
			if k == keep || a.dirtyKey(k) {
				continue
			}
			victim = i
			break
		}
		if victim < 0 {
			break
		}
		a.removeTabAt(victim)
		closed++
	}
	if closed > 0 {
		// Say what happened: a tab vanishing on its own otherwise looks like
		// the app lost it.
		a.setStatus(fmt.Sprintf("标签页超过上限，已自动关闭 %d 个最早打开的标签", closed))
	}
}

// openCall opens a call in a tab, or switches to its tab if it is already open.
// Each tab keeps its own draft, so switching never discards unsaved edits.
func (a *app) openCall(pid, sid, oid, cid string) {
	if a.renamingID != "" {
		a.cancelInlineRename()
	}
	a.projectPage = false
	key := callKey(pid, sid, oid, cid)
	if prev := a.activeTabKey(); prev != "" {
		if prev == key {
			// Already the active tab: keep the editor's unsaved edits.
			a.pid, a.sid, a.oid, a.cid = pid, sid, oid, cid
			return
		}
		// Remember the tab we are leaving, edits included.
		a.drafts[prev] = a.captureDraft()
	}
	if !a.hasTabKey(key) {
		a.tabs = append(a.tabs, key)
		// A new tab goes at the end; scroll there so it is visible at once
		// instead of opening off the right edge of the strip.
		a.tabScroll.X = 1e9
		a.enforceTabLimit(key)
	}
	for i, k := range a.tabs {
		if k == key {
			a.active = i
		}
	}
	a.loadTab(key)
}

// activeTabKey is the key of the tab shown in the editor, "" when none.
func (a *app) activeTabKey() string {
	if a.active >= 0 && a.active < len(a.tabs) {
		return a.tabs[a.active]
	}
	return ""
}

// switchTab moves the selection delta tabs along the strip, wrapping around.
func (a *app) switchTab(delta int) {
	n := len(a.tabs)
	if n == 0 {
		return
	}
	i := a.active
	if i < 0 {
		i = 0
	} else {
		i = (i + delta) % n
		if i < 0 {
			i += n
		}
	}
	pid, sid, oid, cid := splitKey(a.tabs[i])
	a.openCall(pid, sid, oid, cid)
}

// closeActiveTab closes the tab in the editor, as its close button does.
func (a *app) closeActiveTab() {
	if key := a.activeTabKey(); key != "" {
		a.closeTabByKey(key)
	}
}

// captureDraft snapshots the live request form for the active tab.
func (a *app) captureDraft() *callDraft {
	return &callDraft{
		formValues: formValues{
			endpoint: a.endpoint, action: a.action, timeout: a.timeout,
			method: a.method, contentType: a.contentType, reqKind: a.reqKind,
			bodyType: a.bodyType, bodyFile: a.bodyFile,
			queryRows:  append([]queryRow(nil), a.queryRows...),
			bodyFields: append([]bodyField(nil), a.bodyFields...),
			headers:    append([]headerRow(nil), a.headers...),
			auth:       a.auth,
		},
		savedValues: savedValues{
			savedSnapshot: a.savedSnapshot, savedAction: a.savedAction,
			savedTimeout: a.savedTimeout, savedEndpoint: a.savedEndpoint,
			savedMethod: a.savedMethod, savedContentType: a.savedContentType,
			savedBodyType: a.savedBodyType, savedBodyFile: a.savedBodyFile,
			savedBodyFields: append([]bodyField(nil), a.savedBodyFields...),
			savedHeaders:    append([]headerRow(nil), a.savedHeaders...),
			savedAuth:       a.savedAuth,
		},
		body: a.xmlEd.Buffer.String(),
	}
}

// applyDraft puts a tab's saved form back into the editor.
func (a *app) applyDraft(d *callDraft) {
	a.formValues = d.formValues
	// The draft's slices are shared with it, so copy the ones the editor edits
	// in place; the rest are values.
	a.formValues.queryRows = append([]queryRow(nil), d.queryRows...)
	a.formValues.bodyFields = append([]bodyField(nil), d.bodyFields...)
	a.formValues.headers = append([]headerRow(nil), d.headers...)
	a.savedValues = d.savedValues
	a.savedValues.savedBodyFields = append([]bodyField(nil), d.savedBodyFields...)
	a.savedValues.savedHeaders = append([]headerRow(nil), d.savedHeaders...)
	a.querySrc = d.endpoint
	a.xmlEd.Buffer.Reset(d.body)
	a.xmlEd.Buffer.SetLanguage(a.editorLanguage(d.body))
}

// editorLanguage picks the code editor's syntax highlighting for a body based
// on the request's kind and content type: JSON, XML, or plain text.
func (a *app) editorLanguage(body string) *editor.Language {
	if a.isJSONRequest() {
		return editor.LanguageForText("request.json", body)
	}
	if a.isXMLRequest() {
		return editor.LanguageForText("request.xml", body)
	}
	return editor.LanguageForText("request.txt", body)
}

// syncRequestLanguage re-resolves the request editor's highlighting when the
// kind, body type or content type changed. force applies it unconditionally,
// for callers that just changed the body type themselves.
func (a *app) syncRequestLanguage(force bool) {
	key := a.reqKind + "\x00" + a.bodyType + "\x00" + a.contentType
	if !force && key == a.reqLangKey {
		return
	}
	a.reqLangKey = key
	a.xmlEd.Buffer.SetLanguage(a.editorLanguage(a.xmlEd.Buffer.String()))
}

// isJSONRequest reports whether the current request body is JSON, by body type
// first, then by content type, then by how the body is written.
func (a *app) isJSONRequest() bool {
	if a.reqKind == model.KindSOAP {
		return false
	}
	switch a.bodyType {
	case model.BodyJSON:
		return true
	case model.BodyXML, model.BodyText, model.BodyURLEncoded, model.BodyMultipart, model.BodyBinary, model.BodyNone:
		return false
	}
	ct := strings.ToLower(a.contentType)
	if strings.Contains(ct, "json") {
		return true
	}
	if strings.TrimSpace(a.contentType) != "" {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(a.xmlEd.Buffer.String()), "{") ||
		strings.HasPrefix(strings.TrimSpace(a.xmlEd.Buffer.String()), "[")
}

// isXMLRequest reports whether the request body is XML/SOAP.
func (a *app) isXMLRequest() bool {
	if a.reqKind == model.KindSOAP {
		return true
	}
	switch a.bodyType {
	case model.BodyXML:
		return true
	case model.BodyJSON, model.BodyText, model.BodyURLEncoded, model.BodyMultipart, model.BodyBinary, model.BodyNone:
		return false
	}
	ct := strings.ToLower(a.contentType)
	return strings.Contains(ct, "xml")
}

// loadTab makes key the active tab and shows its draft, or loads it from the
// store the first time. It does not capture the outgoing tab; callers do that
// first when they must keep it.
func (a *app) loadTab(key string) {
	pid, sid, oid, cid := splitKey(key)
	a.pid, a.sid, a.oid, a.cid = pid, sid, oid, cid
	_, _, _, c := a.locate()
	if c == nil {
		return
	}
	// The tab strip should bring this tab into view, even when it is an old
	// tab outside the strip's current scroll window.
	a.revealTab = true
	a.reqDescription = c.Description
	if p := a.findProject(pid); p != nil {
		a.projHeaders = append([]model.Header(nil), p.Headers...)
	}
	if d, ok := a.drafts[key]; ok {
		a.applyDraft(d)
	} else {
		defKind := model.KindHTTP
		if sid != "" {
			defKind = model.KindSOAP
		}
		a.loadCallIntoForm(c, defKind)
		a.drafts[key] = a.captureDraft()
	}
	a.loadResponse(c)
	a.syncResponse()
}

// loadCallIntoForm fills the editable request fields from a stored call.
func (a *app) loadCallIntoForm(c *model.Call, defaultKind string) {
	kind := c.KindOrDefault(defaultKind)
	a.reqKind = kind
	a.reqDescription = c.Description
	a.endpoint = c.Endpoint
	a.action = c.Action
	a.method = model.EffectiveMethod(kind, c.Method)
	a.bodyFile = c.BodyFile
	a.bodyFields = bodyFieldsOf(c.BodyFields)
	bt := model.NormalizeBodyType(c.BodyType)
	if bt == "" {
		if kind == model.KindSOAP {
			bt = model.BodyXML
		} else {
			bt = model.InferBodyType(c.ContentType, c.Request)
		}
	}
	if kind == model.KindSOAP {
		bt = model.BodyXML
	}
	a.bodyType = bt
	a.contentType = c.ContentType
	if kind == model.KindSOAP {
		if strings.TrimSpace(a.contentType) == "" {
			a.contentType = model.DefaultContentType(kind)
		}
	} else if bt != model.BodyNone && strings.TrimSpace(a.contentType) == "" {
		a.contentType = model.BodyTypeContentType(bt)
	}
	a.timeout = strconv.Itoa(model.ClampTimeout(c.TimeoutMS))
	if a.timeout == "0" {
		a.timeout = strconv.Itoa(model.DefaultTimeoutMS)
	}
	a.headers = make([]headerRow, 0, len(c.Headers))
	for _, h := range c.Headers {
		a.headers = append(a.headers, headerRow{Name: h.Name, Value: h.Value})
	}
	body := c.Request
	a.xmlEd.Buffer.Reset(body)
	a.xmlEd.Buffer.SetLanguage(a.editorLanguage(body))
	a.auth = authFormOf(c.Auth)
	a.queryRows = parseQueryRows(a.endpoint)
	a.querySrc = a.endpoint
	a.savedSnapshot = body
	a.savedHeaders = append([]headerRow(nil), a.headers...)
	a.savedTimeout = a.timeout
	a.savedAction = a.action
	a.savedEndpoint = a.endpoint
	a.savedMethod = a.method
	a.savedContentType = a.contentType
	a.savedBodyType = a.bodyType
	a.savedBodyFields = append([]bodyField(nil), a.bodyFields...)
	a.savedBodyFile = a.bodyFile
	a.savedAuth = a.auth
}

// bodyFieldsOf converts the persisted body fields into editable rows.
func bodyFieldsOf(in []model.BodyField) []bodyField {
	if in == nil {
		return nil
	}
	out := make([]bodyField, len(in))
	for i, f := range in {
		out[i] = bodyField{Name: f.Name, Value: f.Value, Type: f.Type}
	}
	return out
}

// modelBodyFields converts editable rows back to the persisted form, dropping
// completely empty rows.
func modelBodyFields(rows []bodyField) []model.BodyField {
	var out []model.BodyField
	for _, r := range rows {
		if strings.TrimSpace(r.Name) == "" && strings.TrimSpace(r.Value) == "" {
			continue
		}
		typ := ""
		if r.Type == "file" {
			typ = "file"
		}
		out = append(out, model.BodyField{Name: r.Name, Value: r.Value, Type: typ})
	}
	return out
}

// openBulkEdit shows the current fields as editable text, one "name: value" per
// line, so a list copied from elsewhere can replace them all at once.
func (a *app) openBulkEdit() {
	lines := make([]string, 0, len(a.bodyFields))
	for _, f := range a.bodyFields {
		if strings.TrimSpace(f.Name) == "" && strings.TrimSpace(f.Value) == "" {
			continue
		}
		lines = append(lines, f.Name+": "+f.Value)
	}
	a.bulkText = strings.Join(lines, "\n")
	a.bulkOpen = true
}

// applyBulkEdit replaces the field rows from the bulk text. A line reads
// "name: value" or "name=value"; a line without either is a bare field name.
func (a *app) applyBulkEdit() {
	// Keep a file part's type when its name survives the bulk edit, so pasting
	// the same text back does not silently turn uploads into text fields.
	types := make(map[string]string, len(a.bodyFields))
	for _, f := range a.bodyFields {
		if f.Type != "" {
			types[f.Name] = f.Type
		}
	}
	var out []bodyField
	for _, line := range strings.Split(a.bulkText, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" {
			continue
		}
		name, value := line, ""
		if i := strings.IndexAny(line, ":="); i >= 0 {
			name = strings.TrimSpace(line[:i])
			value = strings.TrimSpace(line[i+1:])
		}
		if name == "" {
			continue
		}
		out = append(out, bodyField{Name: name, Value: value, Type: types[name]})
	}
	a.bodyFields = out
	a.bulkOpen = false
}

// fieldIsFile reports whether a field row uploads a file rather than text.
func fieldIsFile(typ string) bool { return typ == "file" }

// setBodyType switches the plain-HTTP body shape and aligns the content type
// with it, seeding one empty field row for the form-based types.
func (a *app) setBodyType(bt string) {
	bt = model.NormalizeBodyType(bt)
	if bt == "" {
		return
	}
	a.bodyType = bt
	if bt == model.BodyNone {
		a.contentType = ""
	} else if ct := model.BodyTypeContentType(bt); ct != "" {
		a.contentType = ct
	}
	if model.BodyTypeHasFields(bt) && len(a.bodyFields) == 0 {
		a.bodyFields = append(a.bodyFields, bodyField{})
	}
	a.syncRequestLanguage(true)
}

// loadResponse shows the call's last saved response, if any.
func (a *app) loadResponse(c *model.Call) {
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
	} else {
		a.hasResp = false
		a.resp = model.SendResponse{}
	}
}

// closeTab closes tab i, asking first when it holds unsaved edits.
func (a *app) closeTab(i int) {
	if i < 0 || i >= len(a.tabs) {
		return
	}
	key := a.tabs[i]
	if a.dirtyKey(key) {
		a.askConfirm("该请求有未保存的修改，关闭标签会丢失。确定关闭？", "关闭标签", func() {
			for j, k := range a.tabs {
				if k == key {
					a.removeTabAt(j)
					return
				}
			}
		})
		return
	}
	a.removeTabAt(i)
}

// removeTabAt removes tab i and keeps the active tab in step, discarding only
// that tab's draft.
func (a *app) removeTabAt(i int) {
	if i < 0 || i >= len(a.tabs) {
		return
	}
	key := a.tabs[i]
	closingActive := i == a.active
	a.tabs = append(a.tabs[:i], a.tabs[i+1:]...)
	delete(a.drafts, key)
	if len(a.tabs) == 0 {
		a.clearSelection()
		return
	}
	if i < a.active {
		a.active--
	}
	if closingActive {
		if a.active >= len(a.tabs) {
			a.active = len(a.tabs) - 1
		}
		a.loadTab(a.tabs[a.active])
	}
}

// closeTabsUnder closes every tab whose key matches, keeping the others and
// their drafts, and moving the selection back to a surviving tab.
func (a *app) closeTabsUnder(match func(key string) bool) {
	activeKey := a.activeTabKey()
	if activeKey != "" {
		a.drafts[activeKey] = a.captureDraft()
	}
	kept := make([]string, 0, len(a.tabs))
	closed := false
	for _, k := range a.tabs {
		if match(k) {
			delete(a.drafts, k)
			closed = true
			continue
		}
		kept = append(kept, k)
	}
	if !closed {
		return
	}
	a.tabs = kept
	if len(a.tabs) == 0 {
		a.clearSelection()
		return
	}
	if activeKey != "" && a.hasTabKey(activeKey) {
		a.pid, a.sid, a.oid, a.cid = splitKey(activeKey)
		for i, k := range a.tabs {
			if k == activeKey {
				a.active = i
			}
		}
		return
	}
	if a.active >= len(a.tabs) {
		a.active = len(a.tabs) - 1
	}
	if a.active < 0 {
		a.active = 0
	}
	a.loadTab(a.tabs[a.active])
}

// closeTabByKey closes the tab with this key, asking first when it is dirty.
func (a *app) closeTabByKey(key string) {
	for i, k := range a.tabs {
		if k == key {
			a.closeTab(i)
			return
		}
	}
}

// closeTabsConfirm closes the tabs selected by match, asking once when any of
// them holds unsaved edits so a bulk close cannot silently drop changes.
func (a *app) closeTabsConfirm(match func(key string) bool, label string) {
	keys := map[string]bool{}
	dirty := 0
	for _, k := range a.tabs {
		if !match(k) || keys[k] {
			continue
		}
		keys[k] = true
		if a.dirtyKey(k) {
			dirty++
		}
	}
	if len(keys) == 0 {
		return
	}
	closeThem := func() {
		a.closeTabsUnder(func(k string) bool { return keys[k] })
	}
	if dirty > 0 {
		a.askConfirm(fmt.Sprintf("选中的标签中有 %d 个未保存的修改，关闭会丢失。确定关闭？", dirty), label, closeThem)
		return
	}
	closeThem()
}

// closeOtherTabs closes every tab except key.
func (a *app) closeOtherTabs(key string) {
	a.closeTabsConfirm(func(k string) bool { return k != key }, "关闭其他标签")
}

// closeTabsOnSide closes the tabs left (side < 0) or right (side > 0) of key.
func (a *app) closeTabsOnSide(key string, side int) {
	at := -1
	for i, k := range a.tabs {
		if k == key {
			at = i
			break
		}
	}
	if at < 0 {
		return
	}
	if side < 0 {
		a.closeTabsConfirm(func(k string) bool {
			for i := 0; i < at; i++ {
				if a.tabs[i] == k {
					return true
				}
			}
			return false
		}, "关闭左侧标签")
		return
	}
	a.closeTabsConfirm(func(k string) bool {
		for i := at + 1; i < len(a.tabs); i++ {
			if a.tabs[i] == k {
				return true
			}
		}
		return false
	}, "关闭右侧标签")
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

// callTitle returns the name shown for a call key.
func (a *app) callTitle(key string) string {
	pid, sid, oid, cid := splitKey(key)
	p := a.findProject(pid)
	if p == nil {
		return "请求"
	}
	if sid != "" {
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
	for _, c := range p.Calls {
		if c.ID == cid {
			return c.Title
		}
	}
	return "请求"
}

// callMethod returns the HTTP method shown on a tab's badge.
func (a *app) callMethod(key string) string {
	c := a.findCallByKey(key)
	if c == nil {
		return "POST"
	}
	_, sid, _, _ := splitKey(key)
	defKind := model.KindHTTP
	if sid != "" {
		defKind = model.KindSOAP
	}
	return model.EffectiveMethod(c.KindOrDefault(defKind), c.Method)
}

// selectedProject returns the project of the selection, if any.
func (a *app) selectedProject() *model.Project {
	return a.findProject(a.pid)
}

// selectedProjectHost returns the selected project's host, "" when none.
func (a *app) selectedProjectHost() string {
	if p := a.selectedProject(); p != nil {
		return p.Host
	}
	return ""
}

// projectCounts reports the size of a project for its page: folders, requests
// (native calls plus operation samples), services and operations.
func projectCounts(p *model.Project) (groups, calls, services, ops int) {
	if p == nil {
		return
	}
	groups = len(p.Groups)
	calls = len(p.Calls)
	for _, s := range p.Services {
		services++
		for _, o := range s.Operations {
			ops++
			calls += len(o.Calls)
		}
	}
	return
}

// startInlineRename turns the tree node id into an input holding name, focused
// with its text selected, instead of opening a dialog.
func (a *app) startInlineRename(id, name string) {
	if id == "" {
		return
	}
	a.renamingID = id
	a.renameInput = name
	a.renameFocused = false
	a.renameRef.SelectAll()
}

// cancelInlineRename drops an inline rename without changing the tree.
func (a *app) cancelInlineRename() {
	a.renamingID = ""
	a.renameInput = ""
	a.renameFocused = false
}

// commitInlineRename writes the draft name back to the node being edited.
func (a *app) commitInlineRename(it treeItem) {
	id := renameTargetID(it)
	if id == "" || a.renamingID != id {
		return
	}
	name := strings.TrimSpace(a.renameInput)
	a.renamingID = ""
	a.renameInput = ""
	a.renameFocused = false
	if name == "" {
		return
	}
	switch it.kind {
	case "project":
		a.selGroup = ""
		a.pid, a.sid, a.oid, a.cid = it.pid, "", "", ""
		a.renameProject(name)
	case "group":
		a.renameGroupAt(it.pid, it.gid, name)
	case "call":
		// Rename by id, without moving the selection: the editor may still hold
		// another call, and leaving cid on this one would let Ctrl+S overwrite it.
		a.renameCallAt(it.pid, it.sid, it.oid, it.cid, name)
	}
}

// renameTargetID returns the id an inline rename edits, or "" when the row is
// not renameable.
func renameTargetID(it treeItem) string {
	switch it.kind {
	case "project":
		return it.pid
	case "group":
		return it.gid
	case "call":
		return it.cid
	}
	return ""
}

// saveCall writes the form back to the selected call.
func (a *app) saveCall() {
	p, s, o, c := a.locate()
	if c == nil {
		// The tree selection may be a project/service while a tab is open;
		// save the tab's sample the editor is showing.
		if key := a.activeTabKey(); key != "" {
			a.pid, a.sid, a.oid, a.cid = splitKey(key)
			p, s, o, c = a.locate()
		}
	}
	if p == nil || c == nil {
		a.errMsg = "请先选择一个请求"
		return
	}
	call := *c
	call.Endpoint = strings.TrimSpace(a.endpoint)
	call.Action = strings.TrimSpace(a.action)
	call.Kind = a.reqKind
	call.Method = model.EffectiveMethod(a.reqKind, a.method)
	call.ContentType = strings.TrimSpace(a.contentType)
	if a.reqKind == model.KindHTTP {
		call.BodyType = a.bodyType
		call.BodyFields = modelBodyFields(a.bodyFields)
		call.BodyFile = a.bodyFile
	} else {
		call.BodyType = ""
		call.BodyFields = nil
		call.BodyFile = ""
	}
	call.Request = a.xmlEd.Buffer.String()
	call.TimeoutMS = parseTimeout(a.timeout)
	call.Headers = headersFromRows(a.headers)
	call.Auth = a.auth.modelAuth()
	var err error
	if s != nil && o != nil {
		err = a.st.UpdateCall(p.ID, s.ID, o.ID, call)
	} else {
		err = a.st.UpdateProjectCall(p.ID, call)
	}
	if err != nil {
		a.errMsg = "无法保存：" + err.Error()
		return
	}
	a.savedSnapshot = call.Request
	a.savedHeaders = append([]headerRow(nil), a.headers...)
	a.savedAuth = a.auth
	a.savedTimeout = a.timeout
	a.savedAction = call.Action
	a.savedEndpoint = call.Endpoint
	a.savedMethod = call.Method
	a.savedContentType = call.ContentType
	a.savedBodyType = a.bodyType
	a.savedBodyFields = append([]bodyField(nil), a.bodyFields...)
	a.savedBodyFile = a.bodyFile
	// Keep the editor's kind/method in step with what was persisted.
	a.method = call.Method
	if key := a.activeTabKey(); key != "" {
		a.drafts[key] = a.captureDraft()
	}
	a.errMsg = ""
	a.setStatus("已保存：" + call.Title)
	a.reload()
}

// deleteCall removes the selected sample, after confirmation.
func (a *app) deleteCall() {
	p, s, o, c := a.locate()
	if p == nil || c == nil {
		a.errMsg = "请先选择一个请求"
		return
	}
	pid, cid := p.ID, c.ID
	sid, oid := "", ""
	if s != nil && o != nil {
		sid, oid = s.ID, o.ID
	}
	a.askConfirm("确定删除该请求？", "删除", func() {
		var err error
		if sid != "" {
			err = a.st.DeleteCall(pid, sid, oid, cid)
		} else {
			err = a.st.DeleteProjectCall(pid, cid)
		}
		if err != nil {
			a.errMsg = "无法删除：" + err.Error()
			return
		}
		a.setStatus("已删除请求")
		a.reload()
		a.aFromCallRemoved(pid, sid, oid, cid)
	})
}

// aFromCallRemoved closes the deleted call's tab, or keeps the tab actually
// shown when the deleted call was not open, and leaves other tabs alone.
func (a *app) aFromCallRemoved(pid, sid, oid, cid string) {
	key := callKey(pid, sid, oid, cid)
	if a.hasTabKey(key) {
		for i, k := range a.tabs {
			if k == key {
				a.removeTabAt(i)
				break
			}
		}
		if at := a.activeTabKey(); at != "" {
			a.pid, a.sid, a.oid, a.cid = splitKey(at)
		}
		return
	}
	// Not open: the tree selection was moved to it only to target the delete, so
	// keep showing the active tab (and its unsaved edits) instead of blanking it.
	if at := a.activeTabKey(); at != "" {
		a.drafts[at] = a.captureDraft()
		a.loadTab(at)
		return
	}
	if a.pid == pid && a.sid == sid && a.oid == oid && a.cid == cid {
		a.clearEditor()
	}
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
		a.errMsg = "无法新建请求：" + err.Error()
		return
	}
	a.reload()
	a.openCall(p.ID, s.ID, o.ID, call.ID)
	a.setStatus("已新建请求样例")
}

// newProjectCall adds a plain HTTP request under the selected project or one of
// its groups (folderID "" is the project root).
func (a *app) newProjectCall(folderID string) {
	p := a.selectedProject()
	if p == nil {
		a.errMsg = "请先选择一个项目"
		return
	}
	call, err := a.st.AddProjectCall(p.ID, folderID, model.Call{
		Title:    "新请求",
		Method:   "GET",
		BodyType: model.BodyNone,
	})
	if err != nil {
		a.errMsg = "无法新建请求：" + err.Error()
		return
	}
	a.reload()
	a.openCall(p.ID, "", "", call.ID)
	a.setStatus("已新建请求")
}

// moveProjectCall moves a native request into folderID ("" for the project
// root) when it is dragged onto a folder or the project row.
func (a *app) moveProjectCall(pid, cid, folderID string) {
	if err := a.st.MoveProjectCall(pid, cid, folderID); err != nil {
		a.errMsg = "无法移动请求：" + err.Error()
		return
	}
	if folderID != "" {
		a.expanded[folderID] = true
	}
	a.setStatus("已移动请求")
	a.reload()
}

// newGroup creates a folder under the selected project or group.
func (a *app) newGroup(parentID, name string) {
	p := a.selectedProject()
	if p == nil {
		a.errMsg = "请先选择一个项目"
		return
	}
	if _, err := a.st.AddGroup(p.ID, name, parentID); err != nil {
		a.errMsg = "无法新建目录：" + err.Error()
		return
	}
	if parentID != "" {
		a.expanded[parentID] = true
	}
	a.setStatus("已新建目录")
	a.reload()
}

// renameGroupAt renames a folder by id without moving the selection.
func (a *app) renameGroupAt(pid, gid, name string) {
	if err := a.st.RenameGroup(pid, gid, name); err != nil {
		a.errMsg = "无法重命名目录：" + err.Error()
		return
	}
	a.setStatus("已重命名目录")
	a.reload()
}

// deleteGroup removes a folder and everything inside it, after confirmation,
// then closes the tabs of the removed requests.
func (a *app) deleteGroup(pid, gid, name string) {
	a.askConfirm("确定删除目录「"+name+"」及其下所有请求？", "删除", func() {
		removed, err := a.st.DeleteGroup(pid, gid)
		if err != nil {
			a.errMsg = "无法删除目录：" + err.Error()
			return
		}
		a.setStatus("已删除目录")
		a.reload()
		doomed := map[string]bool{}
		for _, id := range removed {
			doomed[id] = true
		}
		a.closeTabsUnder(func(k string) bool {
			kp, _, _, kc := splitKey(k)
			return kp == pid && doomed[kc]
		})
	})
}

// duplicateCall copies the selected sample.
func (a *app) duplicateCall() {
	p, s, o, c := a.locate()
	if p == nil || c == nil {
		a.errMsg = "请先选择一个请求"
		return
	}
	var copied model.Call
	var err error
	if s != nil && o != nil {
		copied, err = a.st.DuplicateCall(p.ID, s.ID, o.ID, c.ID)
		if err == nil {
			a.reload()
			a.openCall(p.ID, s.ID, o.ID, copied.ID)
		}
	} else {
		copied, err = a.st.DuplicateProjectCall(p.ID, c.ID)
		if err == nil {
			a.reload()
			a.openCall(p.ID, "", "", copied.ID)
		}
	}
	if err != nil {
		a.errMsg = "无法复制：" + err.Error()
		return
	}
	a.setStatus("已复制请求")
}

// deleteOperation removes the selected operation and everything under it.
func (a *app) deleteOperation() {
	p, s, o, _ := a.locate()
	if p == nil || s == nil || o == nil {
		a.errMsg = "请先选择一个操作"
		return
	}
	pid, sid, oid := p.ID, s.ID, o.ID
	a.askConfirm("确定删除该操作及其下所有请求？", "删除", func() {
		if err := a.st.DeleteOperation(pid, sid, oid); err != nil {
			a.errMsg = "无法删除操作：" + err.Error()
			return
		}
		a.setStatus("已删除操作")
		a.reload()
		a.closeTabsUnder(func(k string) bool {
			kp, ks, ko, _ := splitKey(k)
			return kp == pid && ks == sid && ko == oid
		})
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
		a.errMsg = "无法新建服务：" + err.Error()
		return
	}
	a.setStatus("已新建服务：" + name)
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
		a.errMsg = "无法新建操作：" + err.Error()
		return
	}
	a.setStatus("已新建操作：" + name)
	a.reload()
}

// renameCall changes the selected sample's title.
func (a *app) renameCall(title string) {
	a.renameCallAt(a.pid, a.sid, a.oid, a.cid, title)
}

// renameCallAt renames one sample by id without touching the current selection,
// so a context-menu rename cannot leave the selection on a sample whose body
// the editor does not hold. sid/oid are empty for a native project request.
func (a *app) renameCallAt(pid, sid, oid, cid, title string) {
	title = strings.TrimSpace(title)
	if title == "" {
		a.errMsg = "请求名称不能为空"
		return
	}
	p := a.findProject(pid)
	if p == nil {
		a.errMsg = "请先选择一个请求"
		return
	}
	if sid != "" {
		for si := range p.Services {
			if p.Services[si].ID != sid {
				continue
			}
			for oi := range p.Services[si].Operations {
				if p.Services[si].Operations[oi].ID != oid {
					continue
				}
				op := &p.Services[si].Operations[oi]
				for ci := range op.Calls {
					if op.Calls[ci].ID != cid {
						continue
					}
					call := op.Calls[ci]
					call.Title = title
					if err := a.st.UpdateCall(pid, sid, oid, call); err != nil {
						a.errMsg = "无法重命名：" + err.Error()
						return
					}
					a.setStatus("已重命名请求")
					a.reload()
					return
				}
			}
		}
	} else {
		for ci := range p.Calls {
			if p.Calls[ci].ID != cid {
				continue
			}
			call := p.Calls[ci]
			call.Title = title
			if err := a.st.UpdateProjectCall(pid, call); err != nil {
				a.errMsg = "无法重命名：" + err.Error()
				return
			}
			a.setStatus("已重命名请求")
			a.reload()
			return
		}
	}
	a.errMsg = "找不到要重命名的请求"
}

// exportProject writes the selected project as SoapUI-compatible XML. Native
// (plain HTTP) projects have no SoapUI representation and are refused.
func (a *app) exportProject(path string) {
	p := a.selectedProject()
	if p == nil {
		a.errMsg = "请先选择一个项目"
		return
	}
	if !p.CanExportSoapUI() {
		a.errMsg = "普通 HTTP 项目不支持导出 SoapUI XML"
		return
	}
	data, err := soapuiproj.Marshal(*p)
	if err != nil {
		a.errMsg = "无法生成 SoapUI XML：" + err.Error()
		return
	}
	if err := fileutil.WriteAtomic(path, data, 0o644); err != nil {
		a.errMsg = "无法写出文件：" + err.Error()
		return
	}
	a.setStatus("已导出 SoapUI XML：" + filepath.Base(path))
}

// isDirty reports whether the editor holds changes not yet saved.
func (a *app) isDirty() bool { return a.currentDirty() }

// formDirty reports whether live differs from the values saved for it. body is
// the request body: the editor's text for the live form, the draft's copy for a
// stored tab.
func formDirty(live formValues, saved savedValues, body string) bool {
	if body != saved.savedSnapshot ||
		live.timeout != saved.savedTimeout ||
		live.action != saved.savedAction ||
		live.endpoint != saved.savedEndpoint ||
		live.method != saved.savedMethod ||
		live.contentType != saved.savedContentType ||
		live.bodyType != saved.savedBodyType ||
		live.bodyFile != saved.savedBodyFile {
		return true
	}
	if !slices.Equal(live.bodyFields, saved.savedBodyFields) {
		return true
	}
	if live.auth != saved.savedAuth {
		return true
	}
	return !slices.Equal(live.headers, saved.savedHeaders)
}

// currentDirty compares the live editor to the active tab's saved values.
func (a *app) currentDirty() bool {
	if a.activeTabKey() == "" {
		return false
	}
	return formDirty(a.formValues, a.savedValues, a.xmlEd.Buffer.String())
}

// draftDirty reports whether a stored tab draft differs from its saved values.
func draftDirty(d *callDraft) bool {
	if d == nil {
		return false
	}
	return formDirty(d.formValues, d.savedValues, d.body)
}

// dirtyKey reports whether a tab holds unsaved changes.
func (a *app) dirtyKey(key string) bool {
	if key == a.activeTabKey() {
		return a.currentDirty()
	}
	return draftDirty(a.drafts[key])
}

// statusTTL is how long a transient status message stays in the status bar.
const statusTTL = 4 * time.Second

// setStatus shows a transient message in the status bar; it clears itself after
// statusTTL instead of staying there for the rest of the session.
func (a *app) setStatus(msg string) {
	a.statusMsg, a.statusAt, a.statusHold = msg, time.Now(), false
}

// setStickyStatus shows a message that stays until replaced, for long-running
// actions whose progress should not vanish.
func (a *app) setStickyStatus(msg string) {
	a.statusMsg, a.statusAt, a.statusHold = msg, time.Now(), true
}

// askConfirm opens the confirmation dialog, whose commit button says label,
// and runs fn if the user agrees.
func (a *app) askConfirm(text, label string, fn func()) {
	a.askConfirmAlt(text, label, fn, "", nil)
}

// askConfirmAlt is askConfirm with a second, optional action shown beside the
// main one (as quit's "save all" next to "quit"), so a dialog can offer two
// ways out of the same decision.
func (a *app) askConfirmAlt(text, label string, fn func(), altLabel string, altFn func()) {
	a.confirmText = text
	a.confirmLabel = label
	a.confirmAction = fn
	a.confirmAltLabel = altLabel
	a.confirmAltAction = altFn
	a.confirmOpen = true
}

// emptyWorkbench reports whether nothing is open to edit: no tab, no selection,
// no response and no body, as a fresh start or after closing every tab.
func (a *app) emptyWorkbench() bool {
	return len(a.tabs) == 0 && a.pid == "" && a.cid == "" && a.selGroup == "" &&
		!a.hasResp && !a.sending && strings.TrimSpace(a.xmlEd.Buffer.String()) == ""
}

// hasUnsavedTabs reports whether any open tab holds unsaved changes.
func (a *app) hasUnsavedTabs() bool {
	for _, key := range a.tabs {
		if a.dirtyKey(key) {
			return true
		}
	}
	return false
}

// unsavedTabsNote names how many open tabs would lose edits if the workspace
// changes, or "" when every tab is saved. The workspace dialogs use it so edits
// to a tab other than the active one are never lost without a warning.
func (a *app) unsavedTabsNote(verb string) string {
	n := 0
	for _, key := range a.tabs {
		if a.dirtyKey(key) {
			n++
		}
	}
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("有 %d 个标签有未保存的修改，%s会丢失这些修改。", n, verb)
}

// askQuit confirms quitting when a tab holds unsaved edits, offering to save
// every changed tab first.
func (a *app) askQuit() {
	a.askConfirmAlt(
		"有未保存的请求修改。可以全部保存后再退出，或直接退出丢弃这些修改。",
		"全部保存并退出",
		func() { a.saveAllAndQuit() },
		"退出",
		func() {
			a.quitting = true
			mygo.App.Quit()
		},
	)
}

// saveAllTabs saves every tab with unsaved edits. It reports false and leaves
// errMsg set when a save fails, so the quit is aborted instead of losing data.
func (a *app) saveAllTabs() bool {
	for _, key := range append([]string(nil), a.tabs...) {
		if !a.hasTabKey(key) || !a.dirtyKey(key) {
			continue
		}
		if a.activeTabKey() != key {
			pid, sid, oid, cid := splitKey(key)
			a.openCall(pid, sid, oid, cid)
		}
		a.errMsg = ""
		a.saveCall()
		if a.errMsg != "" {
			return false
		}
		// The tab's draft still holds the pre-save values; refresh it so the
		// tab reads as clean afterwards.
		a.drafts[key] = a.captureDraft()
	}
	return true
}

// saveAllAndQuit saves every changed tab, then quits. A failed save keeps the
// app open with the error instead of dropping the changes.
func (a *app) saveAllAndQuit() {
	if !a.saveAllTabs() {
		if a.errMsg == "" {
			a.errMsg = "保存失败，已取消退出"
		}
		return
	}
	a.quitting = true
	mygo.App.Quit()
}

// deleteService removes the selected service.
func (a *app) deleteService() {
	p, s, _, _ := a.locate()
	if p == nil || s == nil {
		a.errMsg = "请先选择一个服务"
		return
	}
	pid, sid := p.ID, s.ID
	a.askConfirm("确定删除该服务及其下所有操作？", "删除", func() {
		if err := a.st.DeleteService(pid, sid); err != nil {
			a.errMsg = "无法删除服务：" + err.Error()
			return
		}
		a.setStatus("已删除服务")
		a.reload()
		a.closeTabsUnder(func(k string) bool {
			kp, ks, _, _ := splitKey(k)
			return kp == pid && ks == sid
		})
	})
}

// clearEditor empties the request form and response view without touching the
// open tabs or their drafts.
func (a *app) clearEditor() {
	a.pid, a.sid, a.oid, a.cid = "", "", "", ""
	a.selGroup = ""
	a.endpoint, a.action = "", ""
	a.method = "POST"
	a.reqKind = model.KindHTTP
	a.reqDescription = ""
	a.bodyType = model.BodyNone
	a.bodyFields = nil
	a.bodyFile = ""
	a.contentType = ""
	a.queryRows = nil
	a.querySrc = ""
	a.headers = nil
	a.auth = authForm{Type: "none", Enabled: true}
	a.showAuthSecret = false
	a.projHeaders = nil
	a.xmlEd.Buffer.Reset("")
	a.respEd.Buffer.Reset("")
	a.hasResp = false
	a.resp = model.SendResponse{}
	a.savedSnapshot, a.savedTimeout, a.savedAction = "", "", ""
	a.savedEndpoint = ""
	a.savedMethod, a.savedContentType = "", ""
	a.savedBodyType, a.savedBodyFile = "", ""
	a.savedBodyFields = nil
	a.savedHeaders = nil
	a.savedAuth = authForm{Type: "none", Enabled: true}
}

// clearSelection resets the editor and closes every tab, for actions that
// replace the active workspace.
func (a *app) clearSelection() {
	a.projectPage = false
	a.hostInput = ""
	a.clearEditor()
	a.tabs = nil
	a.active = -1
	a.drafts = map[string]*callDraft{}
}

// persistResponse stores a sent response with its sample, so reopening it shows
// the last response without sending again.
func (a *app) persistResponse(pid, sid, oid, cid string, resp model.SendResponse) {
	a.persistResponseTo(a.st, pid, sid, oid, cid, resp)
}

// persistResponseTo saves a response in st, the store the send started in. A
// workspace switch while the request is in flight must not move the result into
// the newly active store.
func (a *app) persistResponseTo(st *store.Store, pid, sid, oid, cid string, resp model.SendResponse) {
	if resp.Error != "" {
		return
	}
	saved := &model.SavedResponse{
		Status:     resp.Status,
		StatusText: resp.StatusText,
		Headers:    resp.Headers,
		Body:       resp.Body,
		DurationMS: resp.DurationMS,
		Size:       resp.Size,
		Truncated:  resp.Truncated,
	}
	if err := storeSaveResponse(st, pid, sid, oid, cid, saved); err != nil {
		if st == a.st {
			a.errMsg = "无法保存响应：" + err.Error()
		}
		return
	}
	if st == a.st {
		a.reload()
	}
}

// storeSaveResponse routes a saved response to the right store method: an
// operation sample (sid set) or a native project request (sid empty).
func storeSaveResponse(st *store.Store, pid, sid, oid, cid string, saved *model.SavedResponse) error {
	if sid != "" {
		return st.UpdateCallResponse(pid, sid, oid, cid, saved)
	}
	return st.UpdateProjectCallResponse(pid, cid, saved)
}

// buildSendRequest assembles the payload the transport sends: project headers,
// sample headers, the method, content type, auth header and the editor body.
func (a *app) buildSendRequest() (model.SendRequest, *model.ProxySettings, error) {
	p, _, _, c := a.locate()
	if c == nil {
		if key := a.activeTabKey(); key != "" {
			a.pid, a.sid, a.oid, a.cid = splitKey(key)
			p, _, _, c = a.locate()
		}
	}
	if p == nil || c == nil {
		return model.SendRequest{}, nil, fmt.Errorf("请先选择一个请求")
	}
	endpoint := strings.TrimSpace(a.endpoint)
	// A relative endpoint is appended to the project's host; a full address is
	// sent as typed.
	url := model.JoinProjectURL(p.Host, endpoint)
	if err := model.ValidateEndpoint(url); err != nil {
		if !model.EndpointAbsolute(endpoint) && strings.TrimSpace(p.Host) == "" {
			return model.SendRequest{}, nil, fmt.Errorf("接口地址不是完整地址：请填写 http(s) 开头的地址，或在项目页面设置 Host")
		}
		return model.SendRequest{}, nil, err
	}
	kind := a.reqKind
	if kind != model.KindSOAP && kind != model.KindHTTP {
		kind = c.KindOrDefault(model.KindHTTP)
	}
	method := model.EffectiveMethod(kind, a.method)

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
	if kind == model.KindSOAP {
		if a.action != "" {
			hdrs["SOAPAction"] = a.action
		}
	}
	body, contentType, filePath := "", "", ""
	if kind == model.KindHTTP && model.MethodHasBody(method) && a.requestBodyType(kind) == model.BodyBinary {
		// Leave the read to the sending goroutine: only the path travels here.
		path := strings.TrimSpace(a.bodyFile)
		if path == "" {
			return model.SendRequest{}, nil, fmt.Errorf("请选择要上传的文件")
		}
		filePath = path
		contentType = strings.TrimSpace(a.contentType)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
	} else {
		var err error
		body, contentType, err = a.requestBody(kind, method)
		if err != nil {
			return model.SendRequest{}, nil, err
		}
	}
	// Only send a Content-Type when the request actually has a body (or is
	// SOAP); a bodyless GET must not carry the default JSON type.
	if contentType != "" {
		hdrs["Content-Type"] = contentType
	}
	// The authentication page wins over a hand-written Authorization header,
	// which the panel's hint tells the user.
	if h := a.auth.modelAuth().Authorization(); h != "" {
		hdrs["Authorization"] = h
	}

	req := model.SendRequest{
		ProjectID:   p.ID,
		URL:         url,
		Method:      method,
		Headers:     hdrs,
		Body:        body,
		ContentType: contentType,
		TimeoutMS:   parseTimeout(a.timeout),
		FilePath:    filePath,
	}
	var proxy *model.ProxySettings
	if p.Proxy != nil {
		proxy = p.Proxy
	}
	return req, proxy, nil
}

// readBinaryBody reads a binary request body, caching the last file read so the
// request preview does not re-read it on every switch. A change of size or
// modification time invalidates the cache.
func (a *app) readBinaryBody(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("无法读取文件：%w", err)
	}
	if a.binCachePath == path && a.binCacheSize == info.Size() && a.binCacheMod.Equal(info.ModTime()) {
		return a.binCacheData, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("无法读取文件：%w", err)
	}
	a.binCachePath, a.binCacheSize, a.binCacheMod = path, info.Size(), info.ModTime()
	a.binCacheData = string(data)
	return a.binCacheData, nil
}

// requestBodyType returns the effective body shape of the form, inferring it
// from the content type and body text when the user did not pick one.
func (a *app) requestBodyType(kind string) string {
	if kind == model.KindSOAP {
		return ""
	}
	bt := a.bodyType
	if bt == "" {
		bt = model.InferBodyType(a.contentType, a.xmlEd.Buffer.String())
	}
	return bt
}

// requestBody resolves the request body and its content type from the form's
// body type and method.
func (a *app) requestBody(kind, method string) (body, contentType string, err error) {
	if kind == model.KindSOAP {
		ct := strings.TrimSpace(a.contentType)
		if ct == "" {
			ct = model.DefaultContentType(kind)
		}
		return a.xmlEd.Buffer.String(), ct, nil
	}
	bt := a.requestBodyType(kind)
	// A method without a body (GET/HEAD) or an explicit "No Body" sends nothing
	// and carries no content type.
	if bt == model.BodyNone || !model.MethodHasBody(method) {
		return "", "", nil
	}
	switch bt {
	case model.BodyURLEncoded:
		return encodeForm(a.bodyFields), "application/x-www-form-urlencoded", nil
	case model.BodyMultipart:
		b, ct, berr := buildMultipart(a.bodyFields)
		if berr != nil {
			return "", "", berr
		}
		return b, ct, nil
	case model.BodyBinary:
		if strings.TrimSpace(a.bodyFile) == "" {
			return "", "", fmt.Errorf("请选择要上传的文件")
		}
		data, rerr := a.readBinaryBody(a.bodyFile)
		if rerr != nil {
			return "", "", rerr
		}
		ct := strings.TrimSpace(a.contentType)
		if ct == "" {
			ct = "application/octet-stream"
		}
		return data, ct, nil
	default: // text-edited bodies: json / xml / text
		ct := strings.TrimSpace(a.contentType)
		if ct == "" {
			ct = model.BodyTypeContentType(bt)
		}
		return a.xmlEd.Buffer.String(), ct, nil
	}
}

// encodeForm renders name/value pairs as an application/x-www-form-urlencoded
// body.
func encodeForm(fields []bodyField) string {
	var parts []string
	for _, f := range fields {
		if f.Name == "" && f.Value == "" {
			continue
		}
		parts = append(parts, url.QueryEscape(f.Name)+"="+url.QueryEscape(f.Value))
	}
	return strings.Join(parts, "&")
}

// multipartBoundary is fixed per request; a single request is assembled at a
// time, so a constant cannot collide with an in-flight one.
const multipartBoundary = "----soapgoFormBoundary7MA4YWxkTrZu0gW"

// buildMultipart renders name/value pairs as a multipart/form-data body and the
// matching Content-Type (including the boundary). A field of type "file" reads
// its file from disk and adds the filename and a guessed content type.
func buildMultipart(fields []bodyField) (body, contentType string, err error) {
	var b strings.Builder
	for _, f := range fields {
		if f.Name == "" {
			continue
		}
		b.WriteString("--" + multipartBoundary + "\r\n")
		name := strings.ReplaceAll(f.Name, `"`, `\"`)
		if f.Type == "file" {
			path := strings.TrimSpace(f.Value)
			if path == "" {
				return "", "", fmt.Errorf("字段「%s」未选择文件", f.Name)
			}
			data, rerr := os.ReadFile(path)
			if rerr != nil {
				return "", "", fmt.Errorf("无法读取文件 %s：%w", filepath.Base(path), rerr)
			}
			filename := filepath.Base(path)
			ct := mime.TypeByExtension(filepath.Ext(path))
			if ct == "" {
				ct = "application/octet-stream"
			}
			b.WriteString(`Content-Disposition: form-data; name="` + name + `"; filename="` +
				strings.ReplaceAll(filename, `"`, `\"`) + `"` + "\r\n")
			b.WriteString("Content-Type: " + ct + "\r\n\r\n")
			b.Write(data)
			b.WriteString("\r\n")
			continue
		}
		b.WriteString(`Content-Disposition: form-data; name="` + name + `"` + "\r\n\r\n")
		b.WriteString(f.Value + "\r\n")
	}
	b.WriteString("--" + multipartBoundary + "--\r\n")
	return b.String(), "multipart/form-data; boundary=" + multipartBoundary, nil
}

// sendRequest runs the request in a goroutine, so a slow server cannot freeze
// the window while the request is in flight.
func (a *app) sendRequest() {
	if a.sending || a.projectPage {
		// Nothing to send while the project page is shown.
		return
	}
	req, proxy, err := a.buildSendRequest()
	if err != nil {
		a.errMsg = err.Error()
		return
	}
	// Remember which sample and which store this send belongs to: the auto-save
	// must land on them even if the user opens another sample or switches
	// workspace while the request is in flight.
	pid, sid, oid, cid := a.pid, a.sid, a.oid, a.cid
	st := a.st
	ctx, cancel := context.WithCancel(context.Background())
	a.sendCancel = cancel
	a.sending = true
	a.errMsg = ""
	a.setStickyStatus("发送中…")
	go func() {
		resp := transport.SendContext(ctx, req, proxy)
		mygo.RunOnMain(func() {
			a.sending = false
			a.sendCancel = nil
			// Only show the response under the sample it was sent for; if the
			// user switched while it was in flight, keep their current view and
			// still save the result with the original sample.
			sameSample := a.pid == pid && a.sid == sid && a.oid == oid && a.cid == cid
			if sameSample {
				a.hasResp = true
				a.resp = resp
				switch {
				case resp.Error == "已取消":
					a.errMsg = ""
					a.setStatus("已取消")
				case resp.Error != "":
					a.errMsg = "无法发送请求：" + friendlySendError(resp.Error)
					a.setStatus("")
				default:
					a.setStatus(fmt.Sprintf("%d %s · %d ms · %s", resp.Status, resp.StatusText, resp.DurationMS, humanSize(resp.Size)))
				}
				a.syncResponse()
			} else {
				switch {
				case resp.Error == "已取消":
					a.setStatus("后台请求已取消")
				case resp.Error != "":
					a.errMsg = "后台请求无法完成：" + friendlySendError(resp.Error)
				default:
					a.setStatus(fmt.Sprintf("后台请求完成：%d %s · %d ms", resp.Status, resp.StatusText, resp.DurationMS))
				}
			}
			a.persistResponseTo(st, pid, sid, oid, cid, resp)
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
	a.setStickyStatus("正在取消…")
}

// syncResponse writes the response body into the read-only editor.
func (a *app) syncResponse() {
	text := a.responseText()
	file := "response.xml"
	switch a.respTab {
	case 2, 3: // the request preview and the headers are plain text
		file = "response.txt"
	default:
		if a.responseIsJSON() {
			file = "response.json"
		}
	}
	a.respEd.Buffer.Reset(text)
	a.respEd.Buffer.SetLanguage(editor.LanguageForText(file, text))
}

// normalizeTimeout rewrites the timeout input to the value actually used.
func (a *app) normalizeTimeout() {
	if s := strconv.Itoa(parseTimeout(a.timeout)); a.timeout != s {
		a.timeout = s
	}
}

// stepTimeout nudges the timeout by delta milliseconds, clamped to the range.
func (a *app) stepTimeout(delta int) {
	n := parseTimeout(a.timeout) + delta
	if n < model.MinTimeoutMS {
		n = model.MinTimeoutMS
	}
	if n > model.MaxTimeoutMS {
		n = model.MaxTimeoutMS
	}
	a.timeout = strconv.Itoa(n)
}

// responseIsJSON reports whether the response body should be treated as JSON:
// its Content-Type says so, or, without one, its body is written as JSON.
func (a *app) responseIsJSON() bool {
	if !a.hasResp || a.resp.Error != "" {
		return false
	}
	ct := strings.ToLower(a.responseContentType())
	if strings.Contains(ct, "json") {
		return true
	}
	if strings.Contains(ct, "xml") {
		return false
	}
	body := strings.TrimSpace(a.resp.Body)
	return strings.HasPrefix(body, "{") || strings.HasPrefix(body, "[")
}

// responseContentType returns the response's Content-Type header, if any.
func (a *app) responseContentType() string {
	for k, v := range a.resp.Headers {
		if strings.EqualFold(strings.TrimSpace(k), "Content-Type") {
			return v
		}
	}
	return ""
}

// friendlySendError explains a transport error in terms a user can act on,
// keeping the original text for errors it does not recognize.
func friendlySendError(msg string) string {
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "no such host"), strings.Contains(low, "name resolution"),
		strings.Contains(low, "dns"), strings.Contains(low, "lookup"):
		return "无法解析服务器地址，请检查域名和网络"
	case strings.Contains(low, "connection refused"):
		return "服务器拒绝连接，请确认地址和端口"
	case strings.Contains(low, "timeout"), strings.Contains(low, "deadline exceeded"),
		strings.Contains(low, "超时"):
		return "请求超时，可调大超时时间或检查网络"
	case strings.Contains(low, "certificate"), strings.Contains(low, "tls"),
		strings.Contains(low, "x509"):
		return "TLS 证书校验失败，请检查证书或代理设置"
	case strings.Contains(low, "eof"), strings.Contains(low, "connection reset"):
		return "服务器提前关闭了连接"
	default:
		return msg
	}
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
		return headerLines(a.resp.Headers)
	default: // 格式化
		if a.responseIsJSON() {
			return prettyJSON(a.resp.Body)
		}
		return prettyXML(a.resp.Body)
	}
}

// saveResponseFile asks where to save the last response body and writes it
// atomically, so a truncated 16 MB response does not have to be copied by hand.
func (a *app) saveResponseFile() {
	if !a.hasResp || a.resp.Error != "" {
		return
	}
	ext := ".txt"
	switch {
	case a.responseIsJSON():
		ext = ".json"
	case strings.Contains(strings.ToLower(a.resp.Headers["Content-Type"]), "xml"):
		ext = ".xml"
	}
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Parent: a.win, Title: "保存响应", DefaultPath: "response" + ext,
		Filters: []mygo.FileFilter{{Name: "所有文件", Extensions: []string{"*"}}},
	})
	if err != nil || path == "" {
		return
	}
	if err := a.saveResponseTo(path); err != nil {
		a.setStatus("无法保存响应：" + err.Error())
		return
	}
	a.setStatus("响应已保存：" + filepath.Base(path))
}

// saveResponseTo writes the response body to path, through a temp file and a
// rename so an interrupted write leaves no half file.
func (a *app) saveResponseTo(path string) error {
	return fileutil.WriteAtomic(path, []byte(a.resp.Body), 0o644)
}

// headerLines renders headers in a stable order, as map iteration is random
// and would shuffle the Headers tab on every sync.
func headerLines(headers map[string]string) string {
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		// Repeated headers are stored with their values joined by newlines, so
		// print each value as its own header line.
		for _, v := range strings.Split(headers[k], "\n") {
			b.WriteString(k + ": " + v + "\n")
		}
	}
	return b.String()
}

func requestPreview(a *app) string {
	kind := a.reqKind
	method := model.EffectiveMethod(kind, a.method)
	body, contentType, err := a.requestBody(kind, method)
	if err != nil {
		body = a.xmlEd.Buffer.String()
		contentType = strings.TrimSpace(a.contentType)
	}
	hdrs := map[string]string{}
	for _, h := range a.projHeaders {
		hdrs[h.Name] = h.Value
	}
	for _, h := range a.headers {
		hdrs[h.Name] = h.Value
	}
	if kind == model.KindSOAP && a.action != "" {
		hdrs["SOAPAction"] = a.action
	}
	if contentType != "" {
		hdrs["Content-Type"] = contentType
	}
	if h := a.auth.modelAuth().Authorization(); h != "" {
		hdrs["Authorization"] = h
	}
	var b strings.Builder
	b.WriteString(method + " " + a.endpoint + " HTTP/1.1\n")
	b.WriteString(headerLines(hdrs))
	b.WriteString("\n")
	b.WriteString(body)
	return b.String()
}

// formatBody pretty-prints the request body: JSON when the request is JSON,
// otherwise XML.
func (a *app) formatBody() {
	if a.isJSONRequest() {
		out, err := formatJSON(a.xmlEd.Buffer.String())
		if err != nil {
			a.errMsg = "无法格式化：" + err.Error()
			return
		}
		a.xmlEd.Buffer.Reset(out)
		return
	}
	out, err := formatXML(a.xmlEd.Buffer.String())
	if err != nil {
		a.errMsg = "无法格式化：" + err.Error()
		return
	}
	a.xmlEd.Buffer.Reset(out)
}

// minifyBody removes structural whitespace: JSON when the request is JSON,
// otherwise XML.
func (a *app) minifyBody() {
	if a.isJSONRequest() {
		out, err := minifyJSON(a.xmlEd.Buffer.String())
		if err != nil {
			a.errMsg = "无法压缩：" + err.Error()
			return
		}
		a.xmlEd.Buffer.Reset(out)
		return
	}
	toks, err := xmlTokenize(strings.TrimSpace(a.xmlEd.Buffer.String()))
	if err != nil {
		a.errMsg = "无法压缩：" + err.Error()
		return
	}
	var b strings.Builder
	for _, tk := range toks {
		if tk.kind == 't' {
			if strings.TrimSpace(tk.raw) == "" {
				continue // structural whitespace only
			}
			b.WriteString(tk.raw) // keep the text value's own spaces
			continue
		}
		b.WriteString(strings.TrimSpace(tk.raw))
	}
	a.xmlEd.Buffer.Reset(b.String())
}

// copyCurl puts an equivalent curl command on the clipboard.
func (a *app) copyCurl() {
	cmd, err := a.curlCommand()
	if err != nil {
		a.errMsg = "无法生成 curl：" + err.Error()
		return
	}
	mygo.Clipboard.WriteText(cmd)
	a.setStatus("curl 已复制")
}

// curlCommand builds the curl command for the current request. A multipart body
// becomes -F fields and a binary body --data-binary @path, so file bytes are
// never pasted into the command; other bodies go through requestBody as before.
func (a *app) curlCommand() (string, error) {
	kind := a.reqKind
	method := model.EffectiveMethod(kind, a.method)
	bt := a.requestBodyType(kind)
	multipart := kind != model.KindSOAP && bt == model.BodyMultipart
	binary := kind != model.KindSOAP && bt == model.BodyBinary
	body, contentType := "", ""
	if !multipart && !binary {
		var err error
		body, contentType, err = a.requestBody(kind, method)
		if err != nil {
			body = a.xmlEd.Buffer.String()
			contentType = strings.TrimSpace(a.contentType)
		}
	}
	authHeader := a.auth.modelAuth().Authorization()
	// Copy the address the request actually goes to, so a relative endpoint
	// carries the project host it is joined to.
	url := model.JoinProjectURL(a.selectedProjectHost(), a.endpoint)
	var b strings.Builder
	b.WriteString("curl -X " + method + " " + shellQuote(url))
	for _, h := range a.projHeaders {
		if authHeader != "" && strings.EqualFold(strings.TrimSpace(h.Name), "Authorization") {
			continue
		}
		b.WriteString(" \\\n  -H " + shellQuote(h.Name+": "+h.Value))
	}
	for _, h := range a.headers {
		// The auth page's header replaces a hand-written Authorization, so do
		// not send both.
		if authHeader != "" && strings.EqualFold(strings.TrimSpace(h.Name), "Authorization") {
			continue
		}
		b.WriteString(" \\\n  -H " + shellQuote(h.Name+": "+h.Value))
	}
	if kind == model.KindSOAP && a.action != "" {
		b.WriteString(" \\\n  -H " + shellQuote("SOAPAction: "+a.action))
	}
	if authHeader != "" {
		b.WriteString(" \\\n  -H " + shellQuote("Authorization: "+authHeader))
	}
	if binary {
		// curl reads the file itself; the command stays short and pasteable.
		path := strings.TrimSpace(a.bodyFile)
		if path == "" {
			return "", fmt.Errorf("请选择要上传的文件")
		}
		if contentType == "" {
			contentType = strings.TrimSpace(a.contentType)
		}
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		b.WriteString(" \\\n  --data-binary " + shellQuote("@"+path))
	}
	if contentType != "" {
		b.WriteString(" \\\n  -H " + shellQuote("Content-Type: "+contentType))
	}
	switch {
	case multipart:
		for _, f := range a.bodyFields {
			name := strings.TrimSpace(f.Name)
			if name == "" {
				continue
			}
			if f.Type == "file" {
				path := strings.TrimSpace(f.Value)
				if path == "" {
					return "", fmt.Errorf("字段「%s」未选择文件", f.Name)
				}
				b.WriteString(" \\\n  -F " + shellQuote(name+"=@"+path))
			} else {
				b.WriteString(" \\\n  -F " + shellQuote(name+"="+f.Value))
			}
		}
	case !binary && strings.TrimSpace(body) != "":
		b.WriteString(" \\\n  --data-binary " + shellQuote(body))
	}
	return b.String(), nil
}

// ---- project & workspace management ----

func (a *app) createProject(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "新建项目"
	}
	saved, err := a.st.AddProject(model.Project{Name: name, Source: "native"})
	if err != nil {
		a.errMsg = "无法新建项目：" + err.Error()
		return
	}
	a.pid, a.sid, a.oid, a.cid = saved.ID, "", "", ""
	a.reload()
	a.openProjectPage(saved.ID)
	a.setStatus("已新建项目：" + saved.Name)
}

func (a *app) deleteProject() {
	p := a.selectedProject()
	if p == nil {
		a.errMsg = "请先选择一个项目"
		return
	}
	pid, name := p.ID, p.Name
	a.askConfirm("确定删除整个项目「"+name+"」？", "删除", func() {
		if err := a.st.DeleteProject(pid); err != nil {
			a.errMsg = "无法删除项目：" + err.Error()
			return
		}
		a.setStatus("已删除项目：" + name)
		a.reload()
		a.closeTabsUnder(func(k string) bool {
			kp, _, _, _ := splitKey(k)
			return kp == pid
		})
		if a.pid == pid {
			a.pid, a.sid, a.oid, a.cid = "", "", "", ""
			a.projectPage = false
		}
	})
}

func (a *app) renameProject(name string) {
	p := a.selectedProject()
	if p == nil {
		a.errMsg = "请先选择一个项目"
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		a.errMsg = "项目名称不能为空"
		return
	}
	if err := a.st.RenameProject(p.ID, name); err != nil {
		a.errMsg = "无法重命名：" + err.Error()
		return
	}
	a.setStatus("项目已重命名")
	a.reload()
}

// importSoapUI parses a SoapUI project file and adds it.
func (a *app) importSoapUI(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		a.errMsg = "无法读取文件：" + err.Error()
		return
	}
	project, err := soapuiproj.Parse(data)
	if err != nil {
		a.errMsg = "无法解析 SoapUI 项目：" + err.Error()
		return
	}
	if project.Name == "" {
		project.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if _, err := a.st.AddProject(*project); err != nil {
		a.errMsg = "无法保存项目：" + err.Error()
		return
	}
	a.reload()
	a.setStatus("已导入 SoapUI 项目：" + project.Name)
}

// importOpenAPI parses an OpenAPI 3.x document (JSON or YAML) and adds it as a
// native project: one folder per tag, one request per operation.
func (a *app) importOpenAPI(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		a.errMsg = "无法读取文件：" + err.Error()
		return
	}
	project, err := openapi.Parse(data, path)
	if err != nil {
		a.errMsg = "无法解析 OpenAPI：" + err.Error()
		return
	}
	saved, err := a.st.AddProject(*project)
	if err != nil {
		a.errMsg = "无法保存项目：" + err.Error()
		return
	}
	a.reload()
	a.setStatus(fmt.Sprintf("已导入 OpenAPI：%s（%d 个接口）", saved.Name, len(saved.Calls)))
}

// exportOpenAPI writes the selected native project as an OpenAPI 3.x document
// in the requested format ("json" or "yaml").
func (a *app) exportOpenAPI(format string) {
	p := a.selectedProject()
	if p == nil {
		a.errMsg = "请先选择一个项目"
		return
	}
	if !p.CanExportOpenAPI() {
		a.errMsg = "只有普通 HTTP 项目可以导出 OpenAPI"
		return
	}
	doc, err := openapi.Build(*p)
	if err != nil {
		a.errMsg = "无法生成 OpenAPI：" + err.Error()
		return
	}
	data, err := openapi.Marshal(doc, format)
	if err != nil {
		a.errMsg = "无法生成 OpenAPI：" + err.Error()
		return
	}
	ext := "yaml"
	if strings.EqualFold(format, "json") {
		ext = "json"
	}
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Parent: a.win, Title: "导出 OpenAPI",
		DefaultPath: p.Name + ".openapi." + ext,
		Filters:     []mygo.FileFilter{{Name: "OpenAPI " + strings.ToUpper(ext), Extensions: []string{ext}}},
	})
	if err != nil || path == "" {
		return
	}
	if err := fileutil.WriteAtomic(path, data, 0o644); err != nil {
		a.errMsg = "无法写入文件：" + err.Error()
		return
	}
	a.setStatus("已导出 OpenAPI：" + filepath.Base(path))
}

// importWSDLFile parses a WSDL file and adds a project.
func (a *app) importWSDLFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		a.errMsg = "无法读取文件：" + err.Error()
		return
	}
	res, err := wsdl.Parse(data)
	if err != nil {
		a.errMsg = "无法解析 WSDL：" + err.Error()
		return
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	a.persistWSDL(a.st, res, name, path, "wsdl")
}

// importWSDLURL downloads and parses a WSDL in the background, so a slow server
// cannot freeze the window while the import dialog waits.
func (a *app) importWSDLURL(rawURL string) {
	url := strings.TrimSpace(rawURL)
	if url == "" {
		a.errMsg = "请填写 WSDL URL"
		return
	}
	if a.importing {
		return
	}
	// Resolve the proxy on the UI thread; the fetch only reads this copy.
	proxy := a.importProxy()
	a.importing = true
	a.errMsg = ""
	// The import belongs to the workspace it started in: switching while the
	// download runs must not move the new project to the newly active store.
	st := a.st
	go func() {
		fetched, err := wsdlfetch.Fetch(url, proxy)
		var res *wsdl.Result
		if err == nil {
			res, err = wsdl.Parse(fetched.Data)
			if err != nil {
				err = fmt.Errorf("无法解析 WSDL：%w", err)
			}
		} else {
			err = fmt.Errorf("无法获取 WSDL：%w", err)
		}
		mygo.RunOnMain(func() {
			a.importing = false
			if err != nil {
				a.errMsg = err.Error()
			} else {
				a.persistWSDL(st, res, fetched.Name, fetched.FinalURL, "wsdl-url")
				a.importOpen = false
			}
			if a.win != nil {
				a.win.Invalidate()
			}
		})
	}()
}

// importProxy resolves the proxy choice of the import dialog.
func (a *app) importProxy() *model.ProxySettings {
	switch a.importProxySel {
	case "系统代理":
		return &model.ProxySettings{Mode: "system"}
	case "当前项目代理":
		if p := a.selectedProject(); p != nil && p.Proxy != nil {
			cp := *p.Proxy
			return &cp
		}
	}
	return &model.ProxySettings{Mode: "direct"}
}

// persistWSDL saves a parsed WSDL as a project in st, the store the import
// started in, with one service and one generated sample per operation.
func (a *app) persistWSDL(st *store.Store, res *wsdl.Result, fallbackName, source, projectSource string) {
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
	if _, err := st.AddProject(project); err != nil {
		a.errMsg = "无法保存 WSDL 项目：" + err.Error()
		return
	}
	if st == a.st {
		a.reload()
		a.setStatus("已导入 WSDL：" + name)
	} else {
		a.setStatus("WSDL 已导入到原工作空间：" + name)
	}
}

// openProjectPage shows pid's project page in the main area: the project's
// counts, its host and its proxy settings. It replaces the request editor until
// a request is opened again.
func (a *app) openProjectPage(pid string) {
	if pid == "" {
		return
	}
	if a.renamingID != "" {
		a.cancelInlineRename()
	}
	a.selGroup = ""
	a.pid, a.sid, a.oid, a.cid = pid, "", "", ""
	a.projectPage = true
	a.loadProjectForms()
}

// loadProjectForms fills the project page's host and proxy drafts from the
// selected project.
func (a *app) loadProjectForms() {
	p := a.selectedProject()
	if p == nil {
		a.hostInput = ""
		return
	}
	a.hostInput = p.Host
	a.loadProxyForm(p)
}

// loadProxyForm fills the proxy form from p.
func (a *app) loadProxyForm(p *model.Project) {
	a.proxyModeSel, a.proxyTypeSel = "系统代理", "HTTP"
	a.proxyHost, a.proxyPort, a.proxyUser, a.proxyPass = "", "", "", ""
	if p == nil || p.Proxy == nil {
		return
	}
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
		a.errMsg = "无法保存代理：" + err.Error()
		return
	}
	a.setStatus("代理设置已保存")
	a.reload()
}

// applyProjectHost validates and saves the project page's host draft.
func (a *app) applyProjectHost() {
	p := a.selectedProject()
	if p == nil {
		a.errMsg = "请先选择一个项目"
		return
	}
	host := strings.TrimSpace(a.hostInput)
	if err := model.ValidateHost(host); err != nil {
		a.errMsg = "Host 设置无效：" + err.Error()
		return
	}
	if err := a.st.UpdateProjectHost(p.ID, host); err != nil {
		a.errMsg = "无法保存 Host：" + err.Error()
		return
	}
	a.hostInput = model.NormalizeHost(host)
	a.setStatus("项目 Host 已保存")
	a.reload()
}

// ---- workspace management ----

func (a *app) switchWorkspace(id string) {
	if note := a.unsavedTabsNote("切换工作空间"); note != "" {
		a.askConfirm(note+"确定继续？", "切换", func() {
			a.doSwitchWorkspace(id)
		})
		return
	}
	a.doSwitchWorkspace(id)
}

func (a *app) doSwitchWorkspace(id string) {
	info, err := a.reg.Switch(id)
	if err != nil {
		a.errMsg = "无法切换工作空间：" + err.Error()
		return
	}
	a.st, a.info = a.reg.Active()
	a.clearSelection()
	a.reload()
	a.setStatus("已切换工作空间：" + info.Name)
}

func (a *app) createWorkspace(name string) {
	if note := a.unsavedTabsNote("新建工作空间"); note != "" {
		a.askConfirm(note+"确定继续？", "继续", func() {
			a.doCreateWorkspace(name)
		})
		return
	}
	a.doCreateWorkspace(name)
}

func (a *app) doCreateWorkspace(name string) {
	info, err := a.reg.Create(name)
	if err != nil {
		a.errMsg = "无法新建工作空间：" + err.Error()
		return
	}
	a.st, a.info = a.reg.Active()
	a.clearSelection()
	a.reload()
	a.setStatus("已新建工作空间：" + info.Name)
}

func (a *app) renameWorkspace(name string) {
	if _, err := a.reg.Rename(a.info.ID, name); err != nil {
		a.errMsg = "无法重命名工作空间：" + err.Error()
		return
	}
	a.info = a.reg.CurrentInfo()
	a.setStatus("工作空间已重命名")
}

func (a *app) deleteWorkspace() {
	text := "确定删除当前工作空间「" + a.info.Name + "」？\n工作空间文件会先备份到 trash。"
	if note := a.unsavedTabsNote("删除工作空间"); note != "" {
		text += "\n" + note
	}
	a.askConfirm(text, "删除", func() {
		if err := a.reg.Delete(a.info.ID); err != nil {
			a.errMsg = "无法删除工作空间：" + err.Error()
			return
		}
		a.st, a.info = a.reg.Active()
		a.clearSelection()
		a.reload()
		a.setStatus("已删除工作空间")
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
		a.errMsg = "无法导出：" + err.Error()
		return
	}
	if err := fileutil.WriteAtomic(path, data, 0o644); err != nil {
		a.errMsg = "无法写出文件：" + err.Error()
		return
	}
	a.setStatus("已导出工作空间：" + filepath.Base(path))
}

// importWorkspaceNamed imports with an explicit workspace name.
func (a *app) importWorkspaceNamed(path, name string) {
	if note := a.unsavedTabsNote("导入工作空间"); note != "" {
		a.askConfirm(note+"确定继续？", "导入", func() {
			a.doImportWorkspace(path, name)
		})
		return
	}
	a.doImportWorkspace(path, name)
}

func (a *app) doImportWorkspace(path, name string) {
	data, err := os.ReadFile(path)
	if err != nil {
		a.errMsg = "无法读取文件：" + err.Error()
		return
	}
	var projects []model.Project
	if err := json.Unmarshal(fileutil.StripBOM(data), &projects); err != nil {
		a.errMsg = "不是有效的工作空间 JSON：" + err.Error()
		return
	}
	if strings.TrimSpace(name) == "" {
		name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if _, err := a.reg.Import(name, projects); err != nil {
		a.errMsg = "无法导入工作空间：" + err.Error()
		return
	}
	a.st, a.info = a.reg.Active()
	a.clearSelection()
	a.reload()
	a.setStatus("已导入工作空间：" + name)
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

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// xmlTok is one lexical token of an XML document, with its original bytes.
type xmlTok struct {
	kind byte   // 's' start, 'e' end, 'x' self-closing, 't' text, 'q' comment/CDATA/PI/directive
	raw  string // the token exactly as written
}

// xmlTokenize splits a document into tokens with encoding/xml's tokenizer, but
// keeps each token's original bytes (via Decoder.InputOffset), so namespace
// prefixes, attribute quoting, entity references and CDATA sections survive.
// A regex cannot tokenize XML correctly: a ">" may sit inside an attribute
// value, and "<" inside CDATA or a comment.
func xmlTokenize(s string) ([]xmlTok, error) {
	s = strings.TrimPrefix(s, "\ufeff")
	dec := xml.NewDecoder(strings.NewReader(s))
	dec.Strict = false
	var toks []xmlTok
	prev := int64(0)
	for {
		t, err := dec.RawToken()
		if err == io.EOF {
			return toks, nil
		}
		if err != nil {
			return nil, err
		}
		end := dec.InputOffset()
		raw := s[prev:end]
		prev = end
		switch t.(type) {
		case xml.StartElement:
			if strings.HasSuffix(strings.TrimSpace(raw), "/>") {
				toks = append(toks, xmlTok{'x', raw})
			} else {
				toks = append(toks, xmlTok{'s', raw})
			}
		case xml.EndElement:
			if strings.TrimSpace(raw) == "" {
				continue // synthetic end tag of a self-closing element
			}
			toks = append(toks, xmlTok{'e', raw})
		case xml.CharData:
			if strings.HasPrefix(strings.TrimSpace(raw), "<![CDATA[") {
				toks = append(toks, xmlTok{'q', raw})
			} else {
				toks = append(toks, xmlTok{'t', raw})
			}
		default: // xml.Comment, xml.ProcInst, xml.Directive
			toks = append(toks, xmlTok{'q', raw})
		}
	}
}

// formatXML re-indents an XML document while keeping every tag, namespace
// prefix, attribute and CDATA section as written. An element whose only content
// is text stays on one line (`<a>test</a>`), and an empty `<a></a>` keeps that
// form; a source `<b/>` stays self-closing.
func formatXML(s string) (string, error) {
	toks, err := xmlTokenize(strings.TrimSpace(s))
	if err != nil {
		return "", err
	}
	if len(toks) == 0 {
		return s, nil
	}
	var b strings.Builder
	depth := 0
	indent := func() { b.WriteString(strings.Repeat("  ", depth)) }
	for i := 0; i < len(toks); i++ {
		tk := toks[i]
		raw := strings.TrimSpace(tk.raw)
		if tk.kind == 's' {
			// Look ahead past structural whitespace: an empty element or an
			// element with only inline content (text/CDATA/comment) is one line.
			j := i + 1
			var inline strings.Builder
			for j < len(toks) {
				n := toks[j]
				if n.kind == 't' && strings.TrimSpace(n.raw) == "" {
					j++
					continue
				}
				if isInlineXMLToken(n) {
					inline.WriteString(n.raw)
					j++
					continue
				}
				break
			}
			if j < len(toks) && toks[j].kind == 'e' {
				indent()
				b.WriteString(raw)
				b.WriteString(inline.String())
				b.WriteString(strings.TrimSpace(toks[j].raw))
				b.WriteString("\n")
				i = j
				continue
			}
			indent()
			b.WriteString(raw)
			b.WriteString("\n")
			depth++
			continue
		}
		if raw == "" {
			continue
		}
		if tk.kind == 'e' {
			depth--
			if depth < 0 {
				depth = 0
			}
		}
		indent()
		b.WriteString(raw)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// isInlineXMLToken reports whether a token is content that may sit inside an
// otherwise empty element on one line: text, a CDATA section or a comment.
func isInlineXMLToken(tk xmlTok) bool {
	switch tk.kind {
	case 't':
		return strings.TrimSpace(tk.raw) != ""
	case 'q':
		r := strings.TrimSpace(tk.raw)
		return strings.HasPrefix(r, "<![CDATA[") || strings.HasPrefix(r, "<!--")
	}
	return false
}

// prettyXML formats a response for display, leaving malformed bodies as-is.
func prettyXML(s string) string {
	out, err := formatXML(s)
	if err != nil {
		return s
	}
	return out
}

// prettyJSON pretty-prints a JSON body, leaving it unchanged when it is not
// valid JSON.
func prettyJSON(s string) string {
	out, err := formatJSON(s)
	if err != nil {
		return s
	}
	return out
}

// formatJSON pretty-prints a JSON body with two-space indentation.
func formatJSON(s string) (string, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return s, nil
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(trimmed), "", "  "); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// minifyJSON removes insignificant whitespace from a JSON body.
func minifyJSON(s string) (string, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return s, nil
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(trimmed)); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// parseQueryRows splits a URL's query string into editable rows, preserving
// order and duplicates. The values are URL-decoded.
func parseQueryRows(rawURL string) []queryRow {
	_, query, _ := splitURL(rawURL)
	if query == "" {
		return nil
	}
	var rows []queryRow
	for _, pair := range strings.Split(query, "&") {
		if pair == "" {
			continue
		}
		key, val := pair, ""
		if i := strings.IndexByte(pair, '='); i >= 0 {
			key, val = pair[:i], pair[i+1:]
		}
		k, err := url.QueryUnescape(key)
		if err != nil {
			k = key
		}
		v, err := url.QueryUnescape(val)
		if err != nil {
			v = val
		}
		rows = append(rows, queryRow{Key: k, Value: v})
	}
	return rows
}

// buildQueryURL rewrites rawURL's query string from rows, keeping the path and
// the fragment untouched. Rows with an empty key and value are dropped.
func buildQueryURL(rawURL string, rows []queryRow) string {
	base, _, frag := splitURL(rawURL)
	var parts []string
	for _, r := range rows {
		if r.Key == "" && r.Value == "" {
			continue
		}
		part := url.QueryEscape(r.Key)
		if r.Value != "" {
			part += "=" + url.QueryEscape(r.Value)
		}
		parts = append(parts, part)
	}
	out := base
	if len(parts) > 0 {
		out += "?" + strings.Join(parts, "&")
	}
	if frag != "" {
		out += "#" + frag
	}
	return out
}

// splitURL returns the URL without its query, the query, and the fragment.
func splitURL(rawURL string) (base, query, fragment string) {
	rest := rawURL
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		fragment = rest[i+1:]
		rest = rest[:i]
	}
	if i := strings.IndexByte(rest, '?'); i >= 0 {
		query = rest[i+1:]
		rest = rest[:i]
	}
	return rest, query, fragment
}
