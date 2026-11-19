package main

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"soapgo/internal/model"
)

// Icons drawn as SVG, so the chrome does not fall back to the look of a
// platform button. They take the text color of their element.
var (
	iconSliders      = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><line x1="4" y1="21" x2="4" y2="14"/><line x1="4" y1="10" x2="4" y2="3"/><line x1="12" y1="21" x2="12" y2="12"/><line x1="12" y1="8" x2="12" y2="3"/><line x1="20" y1="21" x2="20" y2="16"/><line x1="20" y1="12" x2="20" y2="3"/><line x1="1" y1="14" x2="7" y2="14"/><line x1="9" y1="8" x2="15" y2="8"/><line x1="17" y1="16" x2="23" y2="16"/></svg>`))
	iconCode         = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>`))
	iconInfo         = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><line x1="12" y1="11" x2="12" y2="16.5"/><line x1="12" y1="7.5" x2="12" y2="7.6"/></svg>`))
	iconKeyboard     = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="4.5" width="20" height="15" rx="2.5"/><path d="M6 8.5h.01M10 8.5h.01M14 8.5h.01M18 8.5h.01M6 12h.01M10 12h.01M14 12h.01M18 12h.01M7.5 15.5h9"/></svg>`))
	iconMore         = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor"><circle cx="5" cy="12" r="1.7"/><circle cx="12" cy="12" r="1.7"/><circle cx="19" cy="12" r="1.7"/></svg>`))
	iconSplitV       = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="3.5" y="4.5" width="17" height="15" rx="3"/><path d="M3.5 12h17"/></svg>`))
	iconSplitH       = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="3.5" y="4.5" width="17" height="15" rx="3"/><path d="M12 4.5v15"/></svg>`))
	iconPlus         = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M12 5v14M5 12h14"/></svg>`))
	iconCopy         = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="12" height="12" rx="2.5"/><path d="M6 15H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v1"/></svg>`))
	iconClose        = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>`))
	iconFolder       = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/></svg>`))
	iconFolderOpen   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="m6 14 1.5-2.9A2 2 0 0 1 9.24 10H20a2 2 0 0 1 1.94 2.5l-1.55 6a2 2 0 0 1-1.94 1.5H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h3.93a2 2 0 0 1 1.66.9l.82 1.2a2 2 0 0 0 1.66.9H18a2 2 0 0 1 2 2v2"/></svg>`))
	iconChevronRight = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m9 6 6 6-6 6"/></svg>`))
	iconChevronDown  = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m6 9 6 6 6-6"/></svg>`))
	iconSun          = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><circle cx="12" cy="12" r="4.2"/><path d="M12 2.6v2M12 19.4v2M2.6 12h2M19.4 12h2M5.1 5.1l1.4 1.4M17.5 17.5l1.4 1.4M18.9 5.1l-1.4 1.4M6.5 17.5l-1.4 1.4"/></svg>`))
	iconMoon         = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M20 14.5A8.5 8.5 0 1 1 9.5 4a7 7 0 0 0 10.5 10.5Z"/></svg>`))
	iconSettings     = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3.2"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>`))
	iconEye          = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/></svg>`))
	iconEyeOff       = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M3 3l18 18"/><path d="M10.6 5.1A10.9 10.9 0 0 1 12 5c6.5 0 10 7 10 7a17.6 17.6 0 0 1-3.2 4.1"/><path d="M6.6 6.6C3.9 8.4 2 12 2 12s3.5 7 10 7a10.8 10.8 0 0 0 4.4-.9"/><path d="M9.9 9.9a3 3 0 0 0 4.2 4.2"/></svg>`))
)

// ---- view ----

func (a *app) view(c *ui.Context) {
	c.SetTheme(a.theme())
	a.syncEditorFont()
	// Editor shortcuts, as the browser app's: send and save.
	c.OnWindowShortcut(ui.Ctrl, ui.KeyEnter, func() { a.sendRequest() })
	c.OnWindowShortcut(ui.Ctrl, ui.KeyS, func() { a.saveCall() })
	c.OnWindowShortcut(ui.Super, ui.KeyEnter, func() { a.sendRequest() })
	c.OnWindowShortcut(ui.Super, ui.KeyS, func() { a.saveCall() })
	c.OnWindowShortcut(ui.Ctrl|ui.Shift, ui.KeyF, func() { a.formatBody() })
	c.OnWindowShortcut(ui.Ctrl, ui.KeyK, func() { a.searchRef.SelectAll() })
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		a.topBar(c)
		// A draggable divider between the tree and the request area. Before it
		// was a plain row, so once the light theme made the sidebar surface and
		// the workbench background the same white there was nothing left to see
		// between them. Split draws that line and lets the user resize it.
		ui.Split(c, &a.sideW, func() {
			a.sidebar(c)
		}, func() {
			a.main(c)
		}).Grow(1).Fill().SplitReset(272)
		a.statusBar(c)
		a.importModal(c)
		a.exportModal(c)
		a.proxyModal(c)
		a.newProjectModal(c)
		a.workspaceModal(c)
		a.treeModal(c)
		a.confirmModal(c)
		a.settingsModal(c)
	})
	a.maybeSaveUIState()
}

// topBarHeight is the height of the custom title bar in DIPs. The traffic
// lights are centered in it on macOS (see trafficLightPosition).
const topBarHeight = 44

// topBar is a slim, draggable breadcrumb bar with the workspace menu, after
// Yaak.
func (a *app) topBar(c *ui.Context) {
	t := c.Theme()
	bar := c.TitleBar()
	h := bar.Height
	if h < topBarHeight {
		h = topBarHeight
	}
	// Keep the content clear of the window controls: on macOS the traffic
	// lights sit at the top-left and span bar.Left pixels, so a fixed left
	// pad would draw the icon on top of them. On Windows and Linux bar.Left
	// is 0 and the pad below is the ordinary 14.
	padTop, padRight, padBottom, padLeft := titleBarPad(bar, 14)
	ui.Row(c).Shrink(0).Height(h).
		Padding(padTop, padRight, padBottom, padLeft).Gap(8).AlignItems(ui.Center).
		Background(t.Surface).DragWindow().Children(func() {
		ui.Row(c).Width(22).Height(22).Center().Justify(ui.Center).
			Gradient(ui.Hex("#6366f1"), ui.Hex("#a855f7"), 135).Radius(6).Children(func() {
			ui.Text(c, "S").FontSize(a.us(12)).Bold().TextColor(t.AccentText)
		})
		ui.Text(c, "soap-go").FontSize(a.us(13)).Bold()
		ui.Text(c, "/").FontSize(a.us(12)).TextColor(t.Border)
		ui.Text(c, a.info.Name).FontSize(a.us(12)).TextColor(t.TextMuted)
		if p := a.selectedProject(); p != nil {
			ui.Text(c, "›").FontSize(a.us(12)).TextColor(t.Border)
			ui.Text(c, p.Name).FontSize(a.us(12)).TextColor(t.TextMuted).SingleLine()
		}

		// Workspace menu.
		ui.PopoverMenuButton(c, "工作空间 · "+a.info.Name, func(m *ui.Menu) {
			for _, w := range a.reg.List() {
				w := w
				if m.Item(w.Name).Radio(w.ID == a.info.ID).Chosen() && w.ID != a.info.ID {
					a.switchWorkspace(w.ID)
				}
			}
			m.Separator()
			if m.Item("新建工作空间…").Chosen() {
				a.wsOpen, a.wsKind, a.nameInput = true, 0, ""
			}
			if m.Item("重命名当前工作空间…").Chosen() {
				a.wsOpen, a.wsKind, a.nameInput = true, 1, a.info.Name
			}
			if m.Item("导入工作空间…").Chosen() {
				if path, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
					Parent: a.win, Title: "选择工作空间 JSON",
					Filters: []mygo.FileFilter{{Name: "工作空间", Extensions: []string{"json"}}},
				}); err == nil && len(path) > 0 {
					a.wsImportPath = path[0]
					base := path[0]
					if i := strings.LastIndexAny(base, "\\/"); i >= 0 {
						base = base[i+1:]
					}
					if i := strings.LastIndex(base, "."); i > 0 {
						base = base[:i]
					}
					a.wsOpen, a.wsKind, a.nameInput = true, 2, base
				}
			}
			if m.Item("导出工作空间…").Chosen() {
				a.exportOpen = true
			}
			if m.Item("删除当前工作空间").Danger().Chosen() {
				a.deleteWorkspace()
			}
		}).BorderWidth(0)
		ui.Spacer(c)

		a.iconBtn(c, iconSettings, "设置", func() { a.settingsOpen = true })

		toggleTheme := func() {
			a.setDark(!a.dark)
		}
		if a.dark {
			a.iconBtn(c, iconSun, "切换到浅色主题", toggleTheme)
		} else {
			a.iconBtn(c, iconMoon, "切换到深色主题", toggleTheme)
		}
	})
	ui.Divider(c)
}

// titleBarPad turns the room the window controls take (ui.TitleBar) into the
// top bar's inner padding, so its content starts to the right of the traffic
// lights on macOS instead of under them. bar.Left is the width of the
// controls from the left edge (about the close-to-zoom span on macOS, 0 on
// Windows and Linux), and bar.Bottom is how far down they reach. base is the
// ordinary left pad used when the platform reserves no space there.
func titleBarPad(bar ui.TitleBar, base float32) (top, right, bottom, left float32) {
	left = base
	if bar.Left > 0 {
		// A small gap between the last light and the app's own content.
		left = bar.Left + 10
	}
	return 0, bar.Right + 10, 0, left
}

// macTrafficLightButton is the diameter macOS draws its window controls at; it
// centers the lights in the custom title bar.
const macTrafficLightButton = 14

// trafficLightPosition is the position macOS should give its window controls
// (the traffic lights) so they line up with the top bar's content: they start
// trafficLightMargin from the left edge, the same rhythm as the content pad,
// and sit vertically centered in the topBarHeight bar—aligned with the icons on
// the right instead of hugging the top-left corner. It returns nil off macOS,
// where there are no traffic lights to move.
func trafficLightPosition() *mygo.Point {
	if runtime.GOOS != "darwin" {
		return nil
	}
	margin := 14
	top := int(math.Round((topBarHeight - macTrafficLightButton) / 2))
	return &mygo.Point{X: margin, Y: top}
}

func (a *app) sidebar(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).FillHeight().FillWidth().Background(t.Surface).Children(func() {
		ui.Row(c).Padding(10, 12, 4, 14).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "项目").FontSize(a.us(11.5)).Bold()
			ui.Badge(c, fmt.Sprint(len(a.projects)))
			ui.Spacer(c)
			a.iconBtn(c, iconPlus, "新建项目", func() {
				a.newProjectOpen, a.nameInput = true, ""
			})
			a.iconMenuBtn(c, iconMore, "更多", func(m *ui.Menu) {
				if m.Item("导入…").Chosen() {
					a.importOpen, a.importKind = true, 0
				}
				if p := a.selectedProject(); p != nil {
					if p.CanExportSoapUI() {
						if m.Item("导出 SoapUI XML…").Chosen() {
							a.saveProjectXML()
						}
					}
					if p.CanExportOpenAPI() {
						if m.Item("导出 OpenAPI (JSON)…").Chosen() {
							a.exportOpenAPI("json")
						}
						if m.Item("导出 OpenAPI (YAML)…").Chosen() {
							a.exportOpenAPI("yaml")
						}
					}
				}
			})
		})
		ui.Row(c).Padding(0, 12, 8, 12).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &a.search).Placeholder("搜索接口").Grow(1).Ref(&a.searchRef).
				Label("搜索接口").
				OnShortcut(0, ui.KeyEscape, func() { a.search = "" })
			if a.search != "" {
				a.closeIconButton(c, 20, "清空搜索", func() { a.search = "" })
			}
		})
		ui.Scroll(c).Grow(1).Padding(4, 6, 6).Children(func() {
			items := a.treeItems()
			if len(items) == 0 {
				ui.Column(c).Padding(16).Gap(6).Children(func() {
					ui.Text(c, "还没有项目").FontSize(a.us(12.5)).Bold()
					ui.Text(c, "用「更多」菜单导入 SoapUI 项目或 WSDL。").FontSize(a.us(10.5)).TextColor(t.TextMuted).Wrap()
				})
				return
			}
			matched := 0
			for _, it := range items {
				if a.search != "" && !strings.Contains(strings.ToLower(it.label), strings.ToLower(a.search)) {
					continue
				}
				a.treeRow(c, it)
				matched++
			}
			if matched == 0 && a.search != "" {
				ui.Column(c).Padding(16).Gap(8).Children(func() {
					ui.Text(c, "没有匹配的接口").FontSize(a.us(12.5)).Bold()
					ui.Text(c, "换个关键词，或清空搜索查看全部。").FontSize(a.us(10.5)).TextColor(t.TextMuted).Wrap()
					if a.plainButton(c, "清空搜索") {
						a.search = ""
					}
				})
			}
		})
	})
}

// treeItem is one rendered row of the project tree.
type treeItem struct {
	kind          string // project | group | service | operation | call
	pid, sid, oid string
	gid           string // group id, for kind == "group"
	cid           string
	label, badge  string
	method        string
	depth         int
	hasChildren   bool
	expanded      bool
	status        int
}

// callDrag is the value a request row carries while dragged onto a folder or a
// project row, to move it there.
type callDrag struct {
	projectID, callID string
}

// sourceLabel is the short badge shown next to a project name.
func sourceLabel(source string) string {
	switch source {
	case "soapui":
		return "SoapUI"
	case "wsdl":
		return "WSDL"
	case "wsdl-url":
		return "WSDL URL"
	default:
		return "HTTP"
	}
}

func (a *app) treeItems() []treeItem {
	var out []treeItem
	for _, p := range a.projects {
		pe := a.expandedAt(p.ID, true)
		hasKids := len(p.Groups) > 0 || len(p.Calls) > 0 || len(p.Services) > 0
		out = append(out, treeItem{kind: "project", pid: p.ID, label: p.Name, badge: sourceLabel(p.Source), hasChildren: hasKids, expanded: pe})
		if !pe {
			continue
		}
		a.appendGroupRows(&out, p, "", 1)
		for _, cl := range p.Calls {
			if cl.FolderID == "" {
				out = append(out, callTreeItem(p.ID, "", "", cl, 1))
			}
		}
		for _, s := range p.Services {
			se := a.expandedAt(s.ID, true)
			out = append(out, treeItem{kind: "service", pid: p.ID, sid: s.ID, label: s.Name, depth: 1, hasChildren: len(s.Operations) > 0, expanded: se})
			if !se {
				continue
			}
			for _, o := range s.Operations {
				oe := a.expandedAt(o.ID, true)
				out = append(out, treeItem{
					kind: "operation", pid: p.ID, sid: s.ID, oid: o.ID, label: o.Name,
					badge: fmt.Sprintf("%d 请求", len(o.Calls)), depth: 2,
					hasChildren: len(o.Calls) > 0, expanded: oe,
				})
				if !oe {
					continue
				}
				for _, cl := range o.Calls {
					out = append(out, callTreeItem(p.ID, s.ID, o.ID, cl, 3))
				}
			}
		}
	}
	return out
}

// callTreeItem builds a call row, showing its HTTP method.
func callTreeItem(pid, sid, oid string, cl model.Call, depth int) treeItem {
	st := 0
	if cl.Response != nil {
		st = cl.Response.Status
	}
	defKind := model.KindHTTP
	if sid != "" {
		defKind = model.KindSOAP
	}
	return treeItem{
		kind: "call", pid: pid, sid: sid, oid: oid, cid: cl.ID,
		label: cl.Title, method: model.EffectiveMethod(cl.KindOrDefault(defKind), cl.Method),
		depth: depth, status: st,
	}
}

// appendGroupRows renders the groups directly under parentID, recursing into
// each group's subfolders and requests. A group is expanded by default.
func (a *app) appendGroupRows(out *[]treeItem, p model.Project, parentID string, depth int) {
	for _, g := range p.Groups {
		if g.ParentID != parentID {
			continue
		}
		ge := a.expandedAt(g.ID, true)
		*out = append(*out, treeItem{
			kind: "group", pid: p.ID, gid: g.ID, label: g.Name, depth: depth,
			hasChildren: a.groupHasChildren(p, g.ID), expanded: ge,
		})
		if !ge {
			continue
		}
		a.appendGroupRows(out, p, g.ID, depth+1)
		for _, cl := range p.Calls {
			if cl.FolderID == g.ID {
				*out = append(*out, callTreeItem(p.ID, "", "", cl, depth+1))
			}
		}
	}
}

// groupHasChildren reports whether a group holds any subfolder or request.
func (a *app) groupHasChildren(p model.Project, gid string) bool {
	for _, g := range p.Groups {
		if g.ParentID == gid {
			return true
		}
	}
	for _, c := range p.Calls {
		if c.FolderID == gid {
			return true
		}
	}
	return false
}

func (a *app) treeRow(c *ui.Context, it treeItem) {
	t := c.Theme()
	selected := (it.kind == "call" && a.cid == it.cid && it.cid != "") ||
		(it.kind == "project" && it.pid == a.pid && a.cid == "" && a.sid == "" && a.selGroup == "") ||
		(it.kind == "group" && it.gid == a.selGroup && a.selGroup != "") ||
		(it.kind == "service" && it.pid == a.pid && it.sid == a.sid && a.oid == "") ||
		(it.kind == "operation" && it.oid == a.oid && a.cid == "")

	editing := a.renamingID != "" && a.renamingID == renameTargetID(it)
	row := ui.Row(c).Gap(6).AlignItems(ui.Center).
		Padding(4, 6, 4, float32(4+it.depth*14))
	if !editing {
		row = row.OnClick(func() { a.selectTree(it) }).
			ContextMenu(func(m *ui.Menu) { a.treeContextMenu(m, it) }).
			Focusable().Label(it.label).AccessSelected(selected)
	}
	switch {
	case selected:
		row = row.Background(t.Accent.Alpha(0.14)).Radius(6)
	case !editing && row.Hovered():
		row = row.Background(t.SurfaceHover).Radius(6)
	}
	row = row.Children(func() {
		if selected {
			ui.Column(c).Width(2).Height(15).Background(t.Accent).Radius(1)
		} else {
			ui.Text(c, "").Width(2)
		}
		if it.hasChildren {
			box := ui.Row(c).Width(20).Height(20).Center().Justify(ui.Center).Radius(4).
				Tooltip("展开/收起")
			if !editing {
				box = box.Cursor(ui.CursorPointer).OnClick(func() {
					switch it.kind {
					case "project", "service", "operation", "group":
						a.toggleExpanded(nodeKey(it), true)
					}
				})
			}
			box.Children(func() {
				chev := iconChevronRight
				if it.expanded {
					chev = iconChevronDown
				}
				ui.Icon(c, chev).FontSize(a.us(11)).TextColor(t.TextMuted)
			})
		} else {
			ui.Text(c, "").Width(20)
		}
		if it.kind == "call" {
			ui.Text(c, it.method).FontSize(a.us(9)).Bold().TextColor(t.Accent).Width(42)
		} else if icon := kindIcon(it.kind, it.expanded); icon != nil {
			ui.Icon(c, icon).FontSize(a.us(15)).TextColor(kindColor(t, it.kind)).Width(16)
		} else {
			ui.Text(c, kindGlyph(it.kind)).FontSize(a.us(11)).TextColor(kindColor(t, it.kind)).Width(16)
		}
		if editing {
			inp := ui.TextInput(c, &a.renameInput).Grow(1).MinWidth(0).Ref(&a.renameRef).
				OnSubmit(func() { a.commitInlineRename(it) }).
				OnShortcut(0, ui.KeyEscape, func() { a.cancelInlineRename() })
			if inp.Focused() {
				a.renameFocused = true
			} else if a.renameFocused {
				// Clicking outside the tree commits the draft, like blur in an
				// inline editor.
				a.commitInlineRename(it)
			}
		} else {
			lc := t.Text
			if it.kind == "call" {
				lc = t.TextMuted
			}
			lbl := ui.Text(c, it.label).FontSize(a.us(12.5)).TextColor(lc).Grow(1).MinWidth(0).SingleLine()
			if selected || it.kind == "project" || it.kind == "operation" {
				lbl = lbl.Bold()
			}
			_ = lbl
		}
		switch {
		case it.kind == "call" && it.status > 0:
			col := t.Success
			if it.status >= 400 {
				col = t.Danger
			}
			ui.Text(c, fmt.Sprint(it.status)).FontSize(a.us(10)).TextColor(col).Shrink(0)
		case it.badge != "":
			ui.Text(c, it.badge).FontSize(a.us(9.5)).TextColor(t.TextMuted).Shrink(0)
		}
	})
	// A native request row drags to a folder or to its project (the root);
	// both take it, and the folder under the pointer lights up.
	if it.kind == "call" && it.cid != "" && it.sid == "" {
		row = row.Drag(callDrag{projectID: it.pid, callID: it.cid}).
			Tooltip("拖动可移动到其他目录")
	}
	if it.kind == "project" || it.kind == "group" {
		folderID := it.gid // "" on a project row means the project root
		if v, over := ui.DragOver[callDrag](row); over && v.projectID == it.pid {
			row = row.Background(t.Accent.Alpha(0.22)).Radius(6)
		}
		if v, ok := ui.Drop[callDrag](row); ok && v.projectID == it.pid {
			a.moveProjectCall(v.projectID, v.callID, folderID)
		}
	}
}

// treeContextMenu is the right-click menu of a tree row: the create/rename/
// delete actions of that node, where the node is.
//
// Merely opening the menu must not move the selection: a right-click that only
// looks at another node used to make Ctrl+S save the editor's content into that
// node, overwriting it. Each action selects its own target first instead.
func (a *app) treeContextMenu(m *ui.Menu, it treeItem) {
	if a.renamingID != "" {
		a.cancelInlineRename()
	}
	switch it.kind {
	case "project":
		pick := func() { a.selGroup = ""; a.pid, a.sid, a.oid, a.cid = it.pid, "", "", "" }
		proj := a.findProject(it.pid)
		if m.Item("新建请求").Chosen() {
			pick()
			a.newProjectCall("")
		}
		if m.Item("新建目录…").Chosen() {
			pick()
			a.newGroupParent = ""
			a.treeModalOpen, a.treeModalKind, a.nameInput = true, 3, ""
		}
		// SOAP operations only come from import; a native project hides this.
		if proj != nil && proj.CanExportSoapUI() {
			if m.Item("新建服务…").Chosen() {
				pick()
				a.treeModalOpen, a.treeModalKind, a.nameInput = true, 1, ""
			}
		}
		if m.Item("重命名项目…").Chosen() {
			name := ""
			if proj != nil {
				name = proj.Name
			}
			a.startInlineRename(it.pid, name)
		}
		if m.Item("代理设置…").Chosen() {
			pick()
			a.openProxy()
		}
		if proj != nil && proj.CanExportSoapUI() {
			if m.Item("导出 SoapUI XML…").Chosen() {
				pick()
				a.saveProjectXML()
			}
		}
		if proj != nil && proj.CanExportOpenAPI() {
			if m.Item("导出 OpenAPI (JSON)…").Chosen() {
				pick()
				a.exportOpenAPI("json")
			}
			if m.Item("导出 OpenAPI (YAML)…").Chosen() {
				pick()
				a.exportOpenAPI("yaml")
			}
		}
		m.Separator()
		if m.Item("删除项目").Danger().Chosen() {
			pick()
			a.deleteProject()
		}
	case "group":
		pick := func() { a.selGroup = it.gid; a.pid, a.sid, a.oid, a.cid = it.pid, "", "", "" }
		if m.Item("新建请求").Chosen() {
			pick()
			a.newProjectCall(it.gid)
		}
		if m.Item("新建目录…").Chosen() {
			pick()
			a.newGroupParent = it.gid
			a.treeModalOpen, a.treeModalKind, a.nameInput = true, 3, ""
		}
		if m.Item("重命名目录…").Chosen() {
			a.startInlineRename(it.gid, it.label)
		}
		m.Separator()
		if m.Item("删除目录").Danger().Chosen() {
			pick()
			a.deleteGroup(it.pid, it.gid, it.label)
		}
	case "service":
		pick := func() { a.selGroup = ""; a.pid, a.sid, a.oid, a.cid = it.pid, it.sid, "", "" }
		if m.Item("新建操作…").Chosen() {
			pick()
			a.treeModalOpen, a.treeModalKind, a.nameInput = true, 2, ""
		}
		m.Separator()
		if m.Item("删除服务").Danger().Chosen() {
			pick()
			a.deleteService()
		}
	case "operation":
		pick := func() { a.selGroup = ""; a.pid, a.sid, a.oid, a.cid = it.pid, it.sid, it.oid, "" }
		if m.Item("新建请求样例").Chosen() {
			pick()
			a.newCall()
		}
		m.Separator()
		if m.Item("删除操作").Danger().Chosen() {
			pick()
			a.deleteOperation()
		}
	case "call":
		// A call action must target it by id, never leave cid pointing at it
		// while the editor still holds another call. Rename and open do not
		// need the selection; copy replaces the editor itself; delete drops it.
		if m.Item("打开").Chosen() {
			a.openCall(it.pid, it.sid, it.oid, it.cid)
		}
		if m.Item("重命名请求…").Chosen() {
			a.startInlineRename(it.cid, it.label)
		}
		if m.Item("复制请求").Chosen() {
			a.selGroup = ""
			a.pid, a.sid, a.oid, a.cid = it.pid, it.sid, it.oid, it.cid
			a.duplicateCall()
		}
		m.Separator()
		if m.Item("删除请求").Danger().Chosen() {
			a.selGroup = ""
			a.pid, a.sid, a.oid, a.cid = it.pid, it.sid, it.oid, it.cid
			a.deleteCall()
		}
	}
}

// saveProjectXML exports the selected project as SoapUI-compatible XML.
func (a *app) saveProjectXML() {
	p := a.selectedProject()
	if p == nil {
		return
	}
	if path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Parent: a.win, Title: "导出 SoapUI XML", DefaultPath: p.Name + "-soapui-project.xml",
		Filters: []mygo.FileFilter{{Name: "SoapUI 项目", Extensions: []string{"xml"}}},
	}); err == nil && path != "" {
		a.exportProject(path)
	}
}
func nodeKey(it treeItem) string {
	switch it.kind {
	case "project":
		return it.pid
	case "group":
		return it.gid
	case "service":
		return it.sid
	default:
		return it.oid
	}
}

func (a *app) selectTree(it treeItem) {
	if a.renamingID != "" {
		a.cancelInlineRename()
	}
	a.selGroup = ""
	switch it.kind {
	case "project":
		a.pid, a.sid, a.oid, a.cid = it.pid, "", "", ""
		a.toggleExpanded(it.pid, true)
	case "group":
		a.pid, a.sid, a.oid, a.cid = it.pid, "", "", ""
		a.selGroup = it.gid
		a.toggleExpanded(it.gid, true)
	case "service":
		a.pid, a.sid, a.oid, a.cid = it.pid, it.sid, "", ""
		a.toggleExpanded(it.sid, true)
	case "operation":
		a.pid, a.sid, a.oid, a.cid = it.pid, it.sid, it.oid, ""
		a.toggleExpanded(it.oid, true)
	case "call":
		a.openCall(it.pid, it.sid, it.oid, it.cid)
	}
}

func (a *app) main(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).FillHeight().Background(t.Background).Children(func() {
		a.tabStrip(c)
		if a.emptyWorkbench() {
			a.emptyWorkbenchPanel(c)
			return
		}
		a.urlBar(c)
		a.errorBanner(c)
		if a.splitSide == 1 {
			ui.SplitPercent(c, &a.splitH, func() {
				a.requestPanel(c)
			}, func() {
				a.responsePanel(c)
			}).Fill()
		} else {
			ui.SplitVerticalPercent(c, &a.splitV, func() {
				a.requestPanel(c)
			}, func() {
				a.responsePanel(c)
			}).Fill()
		}
	})
}

func (a *app) tabStrip(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Shrink(0).Height(36).Padding(0, 8).Gap(2).AlignItems(ui.Center).
		Background(t.Background).Role(ui.RoleTabList).Children(func() {
		for i, key := range a.tabs {
			i, key := i, key
			active := i == a.active
			tab := ui.Row(c).Shrink(0).Height(30).Padding(0, 6, 0, 8).Gap(6).AlignItems(ui.Center).
				Transition(ui.ElementTransition{Duration: 120 * time.Millisecond, Colors: true})
			// The open request takes the accent color so it stands out among
			// the tabs; the others stay quiet and only tint on hover.
			switch {
			case active:
				tab = tab.Background(t.Accent.Alpha(0.20)).Radius(6)
			case tab.Hovered():
				tab = tab.Background(t.SurfaceHover).Radius(6)
			}
			tab.Children(func() {
				title := a.callTitle(key)
				label := title
				if a.dirtyKey(key) {
					label += "（未保存）"
				}
				// The title is its own focusable tab, so assistive technology
				// reads it by name; the close button stays a sibling control
				// rather than being swallowed as content of the tab.
				open := ui.Row(c).Height(24).Padding(0, 4).Gap(8).AlignItems(ui.Center).
					Cursor(ui.CursorPointer).Focusable().Label(label).Tooltip(title).
					Role(ui.RoleTab).AccessSelected(active).
					OnClick(func() {
						pid, sid, oid, cid := splitKey(key)
						a.openCall(pid, sid, oid, cid)
					})
				open.Children(func() {
					ui.Text(c, a.callMethod(key)).FontSize(a.us(8.5)).Bold().TextColor(t.Accent)
					lc := t.TextMuted
					if active {
						lc = t.Accent
					}
					txt := ui.Text(c, title).FontSize(a.us(12)).TextColor(lc).SingleLine().MaxWidth(180)
					if active {
						txt.Bold()
					}
					if a.dirtyKey(key) {
						ui.Text(c, "●").FontSize(a.us(8)).TextColor(t.Accent).Shrink(0).
							Tooltip("有未保存的修改")
					}
				})
				closeBox := ui.Row(c).Width(24).Height(24).Center().Justify(ui.Center).Radius(12).
					Cursor(ui.CursorPointer).Label("关闭标签").Focusable().
					Transition(ui.ElementTransition{Duration: 100 * time.Millisecond, Colors: true})
				closeHovered := closeBox.Hovered()
				closeCol := t.TextMuted
				if closeHovered {
					closeBox = closeBox.Background(t.Danger.Alpha(0.20))
					closeCol = t.Danger
				}
				closeBox.OnClick(func() { a.closeTab(i) }).Children(func() {
					ui.Icon(c, iconClose).FontSize(a.us(10.5)).TextColor(closeCol)
				})
			})
		}
		if len(a.tabs) == 0 {
			ui.Text(c, "没有打开的请求").FontSize(a.us(11.5)).TextColor(t.TextMuted).Padding(0, 4)
		}
		ui.Spacer(c)
		a.splitSwitch(c)
		ui.Text(c, "").Width(4)
	})
	ui.Divider(c)
}

// emptyWorkbenchPanel guides a user with nothing open to their next step.
func (a *app) emptyWorkbenchPanel(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Center().Justify(ui.Center).Gap(10).Children(func() {
		ui.Text(c, "开始使用 soap-go").FontSize(a.us(16)).Bold().TextColor(t.Text)
		ui.Text(c, "新建项目，或导入 SoapUI / WSDL 定义；"+
			"打开请求后，这里会显示请求与响应。").
			FontSize(a.us(11.5)).TextColor(t.TextMuted).Wrap()
		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c, "新建项目").Clicked() {
				a.newProjectOpen, a.nameInput = true, ""
			}
			if ui.Button(c, "导入…").Clicked() {
				a.importOpen, a.importKind = true, 0
			}
		})
	})
}

func (a *app) urlBar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Shrink(0).Padding(10, 14).Gap(8).AlignItems(ui.Center).Children(func() {
		if a.reqKind == model.KindSOAP {
			ui.Text(c, "POST").FontSize(a.us(10)).Bold().TextColor(t.AccentText).
				Padding(6, 10).Background(t.Accent).Radius(7)
		} else {
			// A borderless, content-sized picker instead of ui.Select: ui.Select
			// forces a 35-unit minimum width, which left the control much wider
			// than the method it shows.
			ui.PopoverMenuButton(c, a.method, func(m *ui.Menu) {
				for _, mt := range model.HTTPMethods {
					if m.Item(mt).Radio(a.method == mt).Chosen() {
						a.method = mt
					}
				}
			}).BorderWidth(0)
		}
		ui.TextInput(c, &a.endpoint).Placeholder("http://host/api/path").Grow(1).Label("请求地址").OnSubmit(func() { a.sendRequest() })
		if a.sending {
			if ui.PrimaryButton(c, "取消  ■").Clicked() {
				a.cancelSend()
			}
		} else if ui.PrimaryButton(c, "发送  ▶").Clicked() {
			a.sendRequest()
		}
	})
	ui.Divider(c)
}

func (a *app) requestPanel(c *ui.Context) {
	t := c.Theme()
	labels, keys := a.requestSections()
	if a.reqSection < 0 || a.reqSection >= len(labels) {
		a.reqSection = 0
	}
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Row(c).Shrink(0).Padding(6, 14, 0).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Tabs(c, &a.reqSection, labels...).Shrink(0)
		})
		switch keys[a.reqSection] {
		case "query":
			a.queryPanel(c)
		case "headers":
			a.headersPanel(c)
		case "auth":
			a.authPanel(c)
		case "settings":
			a.settingsPanel(c)
		default:
			a.bodyPanel(c)
		}
	})
}

// requestSections returns the request-panel tab labels and their keys. The
// 查询 tab only applies to HTTP requests.
func (a *app) requestSections() (labels, keys []string) {
	labels = append(labels, "报文")
	keys = append(keys, "body")
	if a.reqKind != model.KindSOAP {
		labels = append(labels, "查询")
		keys = append(keys, "query")
	}
	// SOAP has SOAPAction + Content-Type always; HTTP only has Content-Type.
	baseHeaders := 1
	if a.reqKind == model.KindSOAP {
		baseHeaders = 2
	}
	labels = append(labels, fmt.Sprintf("请求头 %d", len(a.headers)+baseHeaders))
	keys = append(keys, "headers")
	labels = append(labels, "认证")
	keys = append(keys, "auth")
	labels = append(labels, "设置")
	keys = append(keys, "settings")
	return
}

// bodyPanel is the 报文 tab: a SOAP XML editor, or for plain HTTP a body-type
// picker plus the editor that fits the chosen shape.
func (a *app) bodyPanel(c *ui.Context) {
	a.syncRequestLanguage(false)
	if a.reqKind == model.KindSOAP {
		a.bodyEditorPanel(c, "XML 报文", true, true)
		return
	}
	t := c.Theme()
	ui.Column(c).Grow(1).Padding(10, 12, 10).Gap(8).Children(func() {
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			ui.PopoverMenuButton(c, bodyTypeLabel(a.bodyType), func(m *ui.Menu) {
				a.bodyTypeMenu(m)
			}).BorderWidth(0).Width(120)
			ui.Spacer(c)
			if a.bodyType == model.BodyJSON || a.bodyType == model.BodyGraphQL || a.bodyType == model.BodyXML {
				a.flatButton(c, "格式化", a.formatBody)
				a.flatButton(c, "压缩", a.minifyBody)
			}
			a.flatButton(c, "复制为 curl", a.copyCurl)
			a.flatButton(c, "保存请求", a.saveCall)
		})
		switch {
		case a.bodyType == model.BodyNone:
			ui.Column(c).Grow(1).Center().Justify(ui.Center).Children(func() {
				ui.Text(c, "无请求体").FontSize(a.us(12)).TextColor(t.TextMuted)
			})
		case a.bodyType == model.BodyBinary:
			a.binaryBodyPanel(c)
		case model.BodyTypeHasFields(a.bodyType):
			a.formBodyPanel(c)
		default:
			a.editorWithCopy(c, a.xmlEd, "请求报文已复制", func() string { return a.xmlEd.Buffer.String() })
		}
	})
}

// bodyEditorPanel is a plain editor with the format/minify/curl/save actions,
// used for SOAP and for HTTP text bodies.
func (a *app) bodyEditorPanel(c *ui.Context, title string, canFormat, _ bool) {
	t := c.Theme()
	ui.Column(c).Grow(1).Padding(10, 12, 10).Gap(8).Children(func() {
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(a.us(11)).TextColor(t.TextMuted).Bold()
			ui.Spacer(c)
			if canFormat {
				a.flatButton(c, "格式化", a.formatBody)
				a.flatButton(c, "压缩", a.minifyBody)
			}
			a.flatButton(c, "复制为 curl", a.copyCurl)
			a.flatButton(c, "保存请求", a.saveCall)
		})
		a.editorWithCopy(c, a.xmlEd, "请求报文已复制", func() string { return a.xmlEd.Buffer.String() })
	})
}

// binaryBodyPanel shows the chosen upload file with a picker.
func (a *app) binaryBodyPanel(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Gap(10).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "文件").FontSize(a.us(10.5)).TextColor(t.TextMuted).Width(48)
			path := a.bodyFile
			if path == "" {
				path = "（未选择文件）"
			}
			ui.Text(c, path).FontSize(a.us(11)).TextColor(t.TextMuted).Grow(1).SingleLine()
			if ui.Button(c, "选择文件…").Clicked() {
				if picked, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "选择上传文件"}); err == nil && len(picked) > 0 {
					a.bodyFile = picked[0]
				}
			}
		})
		ui.Text(c, "发送时按二进制原样上传，Content-Type 默认 application/octet-stream。").
			FontSize(a.us(10.5)).TextColor(t.TextMuted)
		ui.Spacer(c)
	})
}

// formBodyPanel edits urlencoded / multipart name-value pairs.
func (a *app) formBodyPanel(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Gap(8).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "字段").FontSize(a.us(11.5)).Bold()
			ui.Spacer(c)
			if a.plainButton(c, "＋ 添加字段") {
				a.bodyFields = append(a.bodyFields, bodyField{})
			}
		})
		ui.Divider(c)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "字段名").FontSize(a.us(10)).TextColor(t.TextMuted).Width(200)
			ui.Text(c, "值").FontSize(a.us(10)).TextColor(t.TextMuted).Grow(1)
			ui.Text(c, "").Width(24)
		})
		if a.bodyType == model.BodyMultipart {
			ui.Text(c, "多部分表单目前只支持文本字段；上传文件请改用“二进制文件”类型。").
				FontSize(a.us(10)).TextColor(t.TextMuted).Wrap()
		}
		for i := range a.bodyFields {
			i := i
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &a.bodyFields[i].Name).Placeholder("字段名").Width(200).Label("字段名")
				ui.TextInput(c, &a.bodyFields[i].Value).Placeholder("值").Grow(1).Label("字段值")
				a.closeIconButton(c, 24, "删除该字段", func() {
					if i >= 0 && i < len(a.bodyFields) {
						a.bodyFields = append(a.bodyFields[:i], a.bodyFields[i+1:]...)
					}
				})
			})
		}
		ui.Spacer(c)
	})
}

// bodyTypeLabel is the human name of a body type.
func bodyTypeLabel(bt string) string {
	switch bt {
	case model.BodyJSON:
		return "JSON"
	case model.BodyXML:
		return "XML"
	case model.BodyGraphQL:
		return "GraphQL"
	case model.BodyText:
		return "文本"
	case model.BodyURLEncoded:
		return "URL 编码"
	case model.BodyMultipart:
		return "多部分表单"
	case model.BodyBinary:
		return "二进制文件"
	default:
		return "无请求体"
	}
}

// bodyTypeMenu builds the body-type picker, grouped as in the reference UI.
func (a *app) bodyTypeMenu(m *ui.Menu) {
	choice := func(label, bt string) {
		if m.Item(label).Radio(a.bodyType == bt).Chosen() {
			a.setBodyType(bt)
		}
	}
	m.Item("表单数据").Disabled(true)
	choice("URL 编码", model.BodyURLEncoded)
	choice("多部分表单", model.BodyMultipart)
	m.Separator()
	m.Item("文本内容").Disabled(true)
	choice("GraphQL", model.BodyGraphQL)
	choice("JSON", model.BodyJSON)
	choice("XML", model.BodyXML)
	choice("其他", model.BodyText)
	m.Separator()
	m.Item("其他").Disabled(true)
	choice("二进制文件", model.BodyBinary)
	choice("无请求体", model.BodyNone)
}

func (a *app) headersPanel(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Padding(14, 14, 10).Gap(8).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "请求头").FontSize(a.us(11.5)).Bold()
			ui.Spacer(c)
			ui.Checkbox(c, &a.maskSecrets, "遮蔽敏感头")
			if a.plainButton(c, "＋ 添加请求头") {
				a.headers = append(a.headers, headerRow{})
			}
		})
		if a.reqKind == model.KindSOAP {
			ui.Text(c, "SOAPAction 与 Content-Type 默认存在；项目默认头会叠加，样例头可覆盖同名项。").
				FontSize(10.5).TextColor(t.TextMuted)
		} else {
			ui.Text(c, "项目默认头会叠加，样例头可覆盖同名项。").
				FontSize(10.5).TextColor(t.TextMuted)
		}
		ui.Divider(c)

		if a.reqKind == model.KindSOAP {
			// Default SOAP headers.
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "SOAPAction").FontSize(a.us(10.5)).TextColor(t.TextMuted).Width(110)
				ui.TextInput(c, &a.action).Placeholder("http://tempuri.org/Op").Grow(1)
				ui.Text(c, "默认").FontSize(a.us(9)).TextColor(t.Accent).Shrink(0)
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Content-Type").FontSize(a.us(10.5)).TextColor(t.TextMuted).Width(110)
				ui.Select(c, &a.contentType, []string{
					"text/xml; charset=utf-8",
					"application/soap+xml; charset=utf-8",
				}).Grow(1)
				ui.Text(c, "默认").FontSize(a.us(9)).TextColor(t.Accent).Shrink(0)
			})
		} else {
			switch {
			case a.bodyType == model.BodyNone:
				ui.Text(c, "该请求不发送请求体，也不设置 Content-Type。").
					FontSize(a.us(10.5)).TextColor(t.TextMuted)
			case model.BodyTypeHasFields(a.bodyType):
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Content-Type").FontSize(a.us(10.5)).TextColor(t.TextMuted).Width(110)
					ui.Text(c, model.BodyTypeContentType(a.bodyType)).FontSize(a.us(11)).TextColor(t.TextMuted).Grow(1)
				})
			default:
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Content-Type").FontSize(a.us(10.5)).TextColor(t.TextMuted).Width(110)
					ph := model.BodyTypeContentType(a.bodyType)
					if ph == "" {
						ph = "application/json"
					}
					ui.TextInput(c, &a.contentType).Placeholder(ph).Grow(1).Label("Content-Type")
				})
			}
		}

		if len(a.projHeaders) > 0 {
			ui.Text(c, "项目默认头").FontSize(a.us(10.5)).TextColor(t.TextMuted).Padding(4, 0, 0, 0)
			for _, h := range a.projHeaders {
				h := h
				value := h.Value
				if a.maskSecrets && isSensitiveName(h.Name) {
					value = "••••••（已遮蔽）"
				}
				ui.Row(c).Gap(8).Children(func() {
					ui.Text(c, h.Name+": "+value).FontSize(a.us(11)).TextColor(t.TextMuted)
				})
			}
		}

		ui.Text(c, "样例头").FontSize(a.us(10.5)).TextColor(t.TextMuted).Padding(4, 0, 0, 0)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "名称").FontSize(a.us(10)).TextColor(t.TextMuted).Width(190)
			ui.Text(c, "值").FontSize(a.us(10)).TextColor(t.TextMuted).Grow(1)
			ui.Text(c, "").Width(24)
		})
		for i := range a.headers {
			i := i
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &a.headers[i].Name).Placeholder("Header-Name").Width(190).Label("请求头名称")
				if a.maskSecrets && isSensitiveName(a.headers[i].Name) {
					ui.Text(c, "••••••（已遮蔽）").FontSize(a.us(11)).TextColor(t.TextMuted).Grow(1)
				} else {
					ui.TextInput(c, &a.headers[i].Value).Placeholder("值").Grow(1).Label("请求头值")
				}
				a.closeIconButton(c, 24, "删除该请求头", func() {
					if i >= 0 && i < len(a.headers) {
						a.headers = append(a.headers[:i], a.headers[i+1:]...)
					}
				})
			})
		}
		ui.Spacer(c)
	})
}

// authPanel is the 认证 section: the auth type, an enable switch, and the
// fields of the picked scheme, after Yaak's request auth panel.
// queryPanel is the 查询 tab of an HTTP request. Its rows are parsed from the
// endpoint URL and written back to it, so the URL stays the single source of
// truth and both directions stay in sync.
func (a *app) queryPanel(c *ui.Context) {
	t := c.Theme()
	if a.endpoint != a.querySrc {
		a.queryRows = parseQueryRows(a.endpoint)
		a.querySrc = a.endpoint
	}
	ui.Column(c).Grow(1).Padding(14, 14, 10).Gap(8).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "查询参数").FontSize(a.us(11.5)).Bold()
			ui.Spacer(c)
			if a.plainButton(c, "＋ 添加参数") {
				a.queryRows = append(a.queryRows, queryRow{})
			}
		})
		ui.Text(c, "查询参数与上方 URL 查询串双向同步。").
			FontSize(a.us(10.5)).TextColor(t.TextMuted)
		ui.Divider(c)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "参数名").FontSize(a.us(10)).TextColor(t.TextMuted).Width(200)
			ui.Text(c, "值").FontSize(a.us(10)).TextColor(t.TextMuted).Grow(1)
			ui.Text(c, "").Width(24)
		})
		for i := range a.queryRows {
			i := i
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &a.queryRows[i].Key).Placeholder("参数名").Width(200).Label("参数名")
				ui.TextInput(c, &a.queryRows[i].Value).Placeholder("值").Grow(1).Label("参数值")
				a.closeIconButton(c, 24, "删除该参数", func() {
					if i >= 0 && i < len(a.queryRows) {
						a.queryRows = append(a.queryRows[:i], a.queryRows[i+1:]...)
					}
				})
			})
		}
		ui.Spacer(c)
	})
	// Only rewrite the URL when the rows themselves changed; an untouched URL
	// (even one spelled with %20) must not be canonicalised on load.
	if produced := buildQueryURL(a.endpoint, a.queryRows); produced != a.endpoint &&
		produced != buildQueryURL(a.endpoint, parseQueryRows(a.endpoint)) {
		a.endpoint = produced
		a.querySrc = produced
	}
}

func (a *app) authPanel(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Padding(14, 14, 10).Gap(12).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "认证类型").FontSize(a.us(11.5)).Bold()
			ui.Spacer(c)
			ui.Row(c).Padding(2).Gap(2).Radius(8).Background(t.Background).Border(1, t.Border).Children(func() {
				a.segment(c, "无认证", a.auth.Type == "none", func() { a.setAuthType("none") })
				a.segment(c, "Basic", a.auth.Type == "basic", func() { a.setAuthType("basic") })
				a.segment(c, "Bearer", a.auth.Type == "bearer", func() { a.setAuthType("bearer") })
			})
		})
		if a.auth.Type == "none" {
			ui.Text(c, "当前请求不添加 Authorization 头。").
				FontSize(a.us(11)).TextColor(t.TextMuted)
			ui.Spacer(c)
			return
		}

		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "启用状态").FontSize(a.us(11)).TextColor(t.TextMuted)
			ui.Row(c).Padding(2).Gap(2).Radius(8).Background(t.Background).Border(1, t.Border).Children(func() {
				a.segment(c, "启用", a.auth.Enabled, func() { a.auth.Enabled = true })
				a.segment(c, "禁用", !a.auth.Enabled, func() { a.auth.Enabled = false })
			})
		})
		ui.Divider(c)

		switch a.auth.Type {
		case "basic":
			a.authTextField(c, "Username", &a.auth.Username, "用户名")
			a.authSecretField(c, "Password", &a.auth.Password, "密码")
		case "bearer":
			a.authSecretField(c, "Token", &a.auth.Token, "令牌")
			a.authTextField(c, "Prefix", &a.auth.Prefix, "Bearer")
		}
		ui.Spacer(c)
		ui.Text(c, "认证头会覆盖「请求头」里手写的 Authorization。").
			FontSize(a.us(10.5)).TextColor(t.TextMuted)
	})
}

// authTextField is one plain field of the auth panel.
func (a *app) authTextField(c *ui.Context, label string, value *string, placeholder string) {
	t := c.Theme()
	ui.Column(c).Gap(6).Children(func() {
		ui.Text(c, label).FontSize(a.us(11)).TextColor(t.TextMuted)
		ui.TextInput(c, value).Placeholder(placeholder).Grow(1)
	})
}

// authSecretField is a password or token field: hidden by default, with an eye
// button that reveals it while the user looks, as Yaak's does.
func (a *app) authSecretField(c *ui.Context, label string, value *string, placeholder string) {
	t := c.Theme()
	ui.Column(c).Gap(6).Children(func() {
		ui.Text(c, label).FontSize(a.us(11)).TextColor(t.TextMuted)
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			in := ui.TextInput(c, value).Placeholder(placeholder).Grow(1)
			if !a.showAuthSecret {
				in.Password()
			}
			icon, tip := iconEye, "显示"
			if a.showAuthSecret {
				icon, tip = iconEyeOff, "隐藏"
			}
			a.iconBtn(c, icon, tip, func() { a.showAuthSecret = !a.showAuthSecret })
		})
	})
}

func (a *app) settingsPanel(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Padding(14, 14, 10).Gap(14).Children(func() {
		ui.Text(c, "请求设置").FontSize(a.us(11.5)).Bold()
		if strings.TrimSpace(a.reqDescription) != "" {
			ui.Text(c, "说明").FontSize(a.us(11)).TextColor(t.TextMuted).Width(72)
			ui.Text(c, strings.TrimSpace(a.reqDescription)).FontSize(a.us(11)).TextColor(t.Text).Wrap()
			ui.Divider(c)
		}
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "超时").FontSize(a.us(11)).TextColor(t.TextMuted).Width(72)
			ui.TextInput(c, &a.timeout).Placeholder("60000").Width(110)
			ui.Text(c, "ms").FontSize(a.us(11)).TextColor(t.TextMuted)
			ui.Spacer(c)
			ui.Text(c, "范围 1000 – 600000").FontSize(a.us(10.5)).TextColor(t.TextMuted)
		})
		ui.Divider(c)
		ui.Text(c, "请求方式").FontSize(a.us(11.5)).Bold()
		if a.reqKind == model.KindSOAP {
			ui.Text(c, "SOAP 请求固定为 POST。").FontSize(a.us(10.5)).TextColor(t.TextMuted)
		} else {
			ui.Text(c, "普通 HTTP 请求的方法在地址栏左侧选择（当前 "+a.method+"）。").
				FontSize(a.us(10.5)).TextColor(t.TextMuted)
		}
		ui.Spacer(c)
	})
}

func (a *app) responsePanel(c *ui.Context) {
	t := c.Theme()
	side := a.splitSide == 1
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Divider(c)
		ui.Row(c).Shrink(0).Padding(6, 14).Gap(12).AlignItems(ui.Center).Children(func() {
			if a.hasResp && a.resp.Error != "" {
				ui.Text(c, "无法发送请求："+a.resp.Error).FontSize(a.us(10.5)).TextColor(t.Danger).SingleLine()
			} else if a.hasResp && a.resp.Error == "" {
				col := t.Success
				if a.resp.Status >= 400 {
					col = t.Danger
				}
				a.statusPill(c, fmt.Sprintf("%d %s", a.resp.Status, a.resp.StatusText), col)
				ui.Text(c, fmt.Sprintf("%d ms", a.resp.DurationMS)).FontSize(a.us(10.5)).TextColor(t.TextMuted)
				if !side {
					ui.Text(c, humanSize(a.resp.Size)).FontSize(a.us(10.5)).TextColor(t.TextMuted)
				}
			} else if a.sending {
				ui.Text(c, "发送中…").FontSize(a.us(10.5)).TextColor(t.TextMuted)
			} else {
				ui.Text(c, "尚未发送请求").FontSize(a.us(10.5)).TextColor(t.TextMuted)
			}
		})
		ui.Row(c).Shrink(0).Padding(0, 14, 0).Gap(8).AlignItems(ui.Center).Children(func() {
			if ui.Tabs(c, &a.respTab, "格式化", "原始", "请求", "响应头").Shrink(0).Changed() {
				a.syncResponse()
			}
			ui.Spacer(c)
			if a.resp.Truncated {
				ui.Text(c, "内容已截断").FontSize(a.us(10)).TextColor(t.Warning)
			}
		})
		if !a.hasResp {
			ui.Column(c).Grow(1).Center().Justify(ui.Center).Gap(6).Children(func() {
				ui.Text(c, "发送请求后，响应会显示在这里").FontSize(a.us(12)).TextColor(t.TextMuted)
			})
			return
		}
		ui.Column(c).Grow(1).Padding(8, 12, 10).Children(func() {
			a.editorWithCopy(c, a.respEd, "响应已复制", func() string { return a.respEd.Buffer.String() })
		})
	})
}

// errorBanner shows the latest error where the work happens, above the
// request and response, instead of only in the status bar.
func (a *app) errorBanner(c *ui.Context) {
	if a.errMsg == "" {
		return
	}
	t := c.Theme()
	ui.Row(c).Shrink(0).Padding(6, 12).Gap(8).AlignItems(ui.Center).
		Background(t.Danger.Alpha(0.12)).Children(func() {
		ui.Text(c, "⚠").FontSize(a.us(11)).TextColor(t.Danger).Shrink(0)
		ui.Text(c, a.errMsg).FontSize(a.us(11)).TextColor(t.Danger).Grow(1).Wrap()
		a.closeIconButton(c, 20, "关闭错误提示", func() { a.errMsg = "" })
	})
}

func (a *app) statusBar(c *ui.Context) {
	t := c.Theme()
	ui.Divider(c)
	ui.Row(c).Shrink(0).Height(26).Padding(0, 14).Gap(8).AlignItems(ui.Center).
		Background(t.Surface).Children(func() {
		ui.Text(c, "◈").FontSize(a.us(9)).TextColor(t.Accent)
		ui.Text(c, a.info.Name).FontSize(a.us(10.5)).TextColor(t.Text)
		ui.Text(c, "·").FontSize(a.us(10.5)).TextColor(t.Border)
		ui.Text(c, "UTF-8").FontSize(a.us(10.5)).TextColor(t.TextMuted)
		ui.Spacer(c)
		if a.statusMsg != "" {
			ui.Text(c, a.statusMsg).FontSize(a.us(10.5)).TextColor(t.Success).SingleLine()
		}
		ui.Text(c, fmt.Sprintf("%d 个项目", len(a.projects))).FontSize(a.us(10.5)).TextColor(t.TextMuted)
	})
}

// ---- dialogs ----

func (a *app) importModal(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.importOpen, func() {
		ui.Column(c).Width(520).Gap(14).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "导入").FontSize(a.us(17)).Bold()
				ui.Spacer(c)
				a.closeIconButton(c, 24, "关闭导入", func() { a.importOpen = false })
			})
			ui.Text(c, "选择一种方式把接口定义带进来。").FontSize(a.us(11)).TextColor(t.TextMuted)

			if a.importKind == 3 {
				ui.Column(c).Gap(8).Children(func() {
					ui.Text(c, "WSDL URL").FontSize(a.us(10.5)).TextColor(t.TextMuted)
					ui.TextInput(c, &a.importURL).Placeholder("http://127.0.0.1:8080/services/ws/api?wsdl").Grow(1)
					ui.Text(c, "代理").FontSize(a.us(10.5)).TextColor(t.TextMuted)
					ui.Select(c, &a.importProxySel, []string{"直连（默认）", "系统代理", "当前项目代理"})
				})
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					if ui.Button(c, "返回").Clicked() {
						a.importKind = 0
					}
					if ui.PrimaryButton(c, "获取并导入").Clicked() {
						a.errMsg = ""
						a.importWSDLURL(a.importURL)
						if a.errMsg == "" {
							a.importOpen = false
						}
					}
				})
			} else {
				a.importOption(c, "◇", "导入 SoapUI 项目",
					"从 .xml 项目导入，保留服务、操作与请求样例。", func() {
						if path, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
							Parent: a.win, Title: "选择 SoapUI 项目",
							Filters: []mygo.FileFilter{{Name: "SoapUI 项目", Extensions: []string{"xml"}}},
						}); err == nil && len(path) > 0 {
							a.importSoapUI(path[0])
							a.importOpen = false
						}
					})
				a.importOption(c, "{}", "导入 OpenAPI",
					"从 OpenAPI 3.x 的 .json / .yaml 生成请求，按 tag 分目录。", func() {
						if path, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
							Parent: a.win, Title: "选择 OpenAPI 文件",
							Filters: []mygo.FileFilter{{Name: "OpenAPI", Extensions: []string{"json", "yaml", "yml"}}},
						}); err == nil && len(path) > 0 {
							a.importOpenAPI(path[0])
							a.importOpen = false
						}
					})
				a.importOption(c, "ƒ", "解析 WSDL 文件",
					"从本地 .wsdl / .xml 生成接口与请求模板。", func() {
						if path, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
							Parent: a.win, Title: "选择 WSDL 文件",
							Filters: []mygo.FileFilter{{Name: "WSDL", Extensions: []string{"wsdl", "xml"}}},
						}); err == nil && len(path) > 0 {
							a.importWSDLFile(path[0])
							a.importOpen = false
						}
					})
				a.importOption(c, "↗", "从 URL 导入 WSDL",
					"抓取远程 WSDL，生成新的项目。", func() { a.importKind = 3 })
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					if ui.Button(c, "取消").Clicked() {
						a.importOpen = false
					}
				})
			}
		})
	})
}

func (a *app) importOption(c *ui.Context, icon, title, desc string, onClick func()) {
	t := c.Theme()
	row := ui.Row(c).Gap(12).AlignItems(ui.Center).Padding(12).Radius(9).
		Border(1, t.Border).OnClick(onClick).Focusable().Label(title)
	if row.Hovered() {
		row = row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		ui.Row(c).Width(34).Height(34).Center().Justify(ui.Center).
			Background(t.Accent.Alpha(0.12)).Radius(8).Children(func() {
			ui.Text(c, icon).FontSize(a.us(15)).TextColor(t.Accent)
		})
		ui.Column(c).Gap(2).Grow(1).Children(func() {
			ui.Text(c, title).FontSize(a.us(12.5)).Bold()
			ui.Text(c, desc).FontSize(a.us(10.5)).TextColor(t.TextMuted).Wrap()
		})
		ui.Text(c, "›").FontSize(a.us(15)).TextColor(t.TextMuted)
	})
}

func (a *app) exportModal(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.exportOpen, func() {
		ui.Column(c).Width(460).Gap(14).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "导出工作空间").FontSize(a.us(17)).Bold()
				ui.Spacer(c)
				a.closeIconButton(c, 24, "关闭导出", func() { a.exportOpen = false })
			})
			ui.Text(c, "把当前工作空间的项目导出为 JSON，可在另一台机器或另一个实例导入。").
				FontSize(11).TextColor(t.TextMuted).Wrap()
			ui.Checkbox(c, &a.keepProxy, "保留代理配置（含用户名与密码）")
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.exportOpen = false
				}
				if ui.PrimaryButton(c, "导出").Clicked() {
					if path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
						Parent: a.win, Title: "导出工作空间", DefaultPath: a.info.Name + ".json",
						Filters: []mygo.FileFilter{{Name: "工作空间", Extensions: []string{"json"}}},
					}); err == nil && path != "" {
						a.exportWorkspace(path)
						a.exportOpen = false
					}
				}
			})
		})
	})
}

func (a *app) proxyModal(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.proxyOpen, func() {
		ui.Column(c).Width(480).Gap(10).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "项目代理设置").FontSize(a.us(16)).Bold()
				ui.Spacer(c)
				a.closeIconButton(c, 24, "关闭代理设置", func() { a.proxyOpen = false })
			})
			ui.Text(c, "代理模式").FontSize(a.us(10.5)).TextColor(t.TextMuted)
			ui.Select(c, &a.proxyModeSel, []string{"系统代理", "直连", "自定义代理"})
			if a.proxyModeSel == "自定义代理" {
				ui.Row(c).Gap(8).Children(func() {
					ui.Column(c).Grow(1).Gap(4).Children(func() {
						ui.Text(c, "类型").FontSize(a.us(10.5)).TextColor(t.TextMuted)
						ui.Select(c, &a.proxyTypeSel, []string{"HTTP", "SOCKS5"})
					})
					ui.Column(c).Grow(1).Gap(4).Children(func() {
						ui.Text(c, "主机").FontSize(a.us(10.5)).TextColor(t.TextMuted)
						ui.TextInput(c, &a.proxyHost).Placeholder("127.0.0.1")
					})
					ui.Column(c).Width(100).Gap(4).Children(func() {
						ui.Text(c, "端口").FontSize(a.us(10.5)).TextColor(t.TextMuted)
						ui.TextInput(c, &a.proxyPort).Placeholder("8080")
					})
				})
				ui.Row(c).Gap(8).Children(func() {
					ui.Column(c).Grow(1).Gap(4).Children(func() {
						ui.Text(c, "用户名").FontSize(a.us(10.5)).TextColor(t.TextMuted)
						ui.TextInput(c, &a.proxyUser).Placeholder("可留空")
					})
					ui.Column(c).Grow(1).Gap(4).Children(func() {
						ui.Text(c, "密码").FontSize(a.us(10.5)).TextColor(t.TextMuted)
						ui.TextInput(c, &a.proxyPass).Placeholder("可留空")
					})
				})
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.proxyOpen = false
				}
				if ui.PrimaryButton(c, "保存").Clicked() {
					a.updateProxy()
				}
			})
		})
	})
}

func (a *app) newProjectModal(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.newProjectOpen, func() {
		ui.Column(c).Width(420).Gap(12).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "项目").FontSize(a.us(16)).Bold()
				ui.Spacer(c)
				a.closeIconButton(c, 24, "关闭新建项目", func() { a.newProjectOpen = false })
			})
			ui.Text(c, "项目名称").FontSize(a.us(10.5)).TextColor(t.TextMuted)
			ui.TextInput(c, &a.nameInput).Placeholder("新建项目").Grow(1)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.newProjectOpen = false
				}
				if ui.PrimaryButton(c, "创建").Clicked() {
					a.createProject(a.nameInput)
					a.newProjectOpen = false
				}
			})
		})
	})
}

func (a *app) workspaceModal(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.wsOpen, func() {
		title := "新建工作空间"
		if a.wsKind == 1 {
			title = "重命名工作空间"
		} else if a.wsKind == 2 {
			title = "导入工作空间"
		}
		ui.Column(c).Width(420).Gap(12).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, title).FontSize(a.us(16)).Bold()
				ui.Spacer(c)
				a.closeIconButton(c, 24, "关闭", func() { a.wsOpen = false })
			})
			ui.Text(c, "名称").FontSize(a.us(10.5)).TextColor(t.TextMuted)
			ui.TextInput(c, &a.nameInput).Placeholder("工作空间名称").Grow(1)
			commit := "创建"
			if a.wsKind == 1 {
				commit = "重命名"
			} else if a.wsKind == 2 {
				commit = "导入"
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.wsOpen = false
				}
				if ui.PrimaryButton(c, commit).Clicked() {
					switch a.wsKind {
					case 1:
						a.renameWorkspace(a.nameInput)
					case 2:
						a.importWorkspaceNamed(a.wsImportPath, a.nameInput)
					default:
						a.createWorkspace(a.nameInput)
					}
					a.wsOpen = false
				}
			})
		})
	})
}

// treeModal handles the small name prompts for services and operations.
func (a *app) treeModal(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.treeModalOpen, func() {
		title, placeholder := "新建服务", "服务名称"
		switch a.treeModalKind {
		case 2:
			title, placeholder = "新建操作", "操作名称"
		case 3:
			title, placeholder = "新建目录", "目录名称"
		}
		ui.Column(c).Width(420).Gap(12).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, title).FontSize(a.us(16)).Bold()
				ui.Spacer(c)
				a.closeIconButton(c, 24, "关闭", func() { a.treeModalOpen = false })
			})
			ui.Text(c, placeholder).FontSize(a.us(10.5)).TextColor(t.TextMuted)
			ui.TextInput(c, &a.nameInput).Placeholder(placeholder).Grow(1)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.treeModalOpen = false
				}
				if ui.PrimaryButton(c, "创建").Clicked() {
					switch a.treeModalKind {
					case 1:
						a.addService(a.nameInput)
					case 2:
						a.addOperation(a.nameInput)
					case 3:
						a.newGroup(a.newGroupParent, a.nameInput)
					}
					a.treeModalOpen = false
				}
			})
		})
	})
}

// confirmModal asks before a destructive action.
func (a *app) confirmModal(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.confirmOpen, func() {
		ui.Column(c).Width(440).Gap(14).Children(func() {
			ui.Text(c, "请确认").FontSize(a.us(16)).Bold()
			ui.Text(c, a.confirmText).FontSize(a.us(11.5)).TextColor(t.TextMuted).Wrap()
			label := a.confirmLabel
			if label == "" {
				label = "确定"
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.confirmOpen = false
					a.confirmAction = nil
				}
				if ui.PrimaryButton(c, label).Clicked() {
					a.confirmOpen = false
					fn := a.confirmAction
					a.confirmAction = nil
					if fn != nil {
						fn()
					}
				}
			})
		})
	})
}

// ---- chrome ----

// shortcutHelp lists the window-wide keyboard shortcuts shown in settings.
var shortcutHelp = []struct{ keys, desc string }{
	{"Ctrl/Cmd + Enter", "发送当前请求"},
	{"Ctrl/Cmd + S", "保存当前请求"},
	{"Ctrl/Cmd + Shift + F", "格式化请求报文"},
	{"Ctrl/Cmd + K", "聚焦左侧搜索框"},
	{"Esc", "关闭弹窗或清空搜索"},
	{"Tab / Shift+Tab", "在可聚焦控件间移动"},
	{"Enter / Space", "激活当前聚焦的按钮"},
}

// settingsModal is a two-pane settings window: a navigation list on the left
// and the chosen section's rows on the right, after a reference the user
// gave. Changes apply live, because the view reads them every frame, and are
// persisted with the rest of the UI state.
func (a *app) settingsModal(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.settingsOpen, func() {
		ui.Row(c).Width(760).Height(520).AlignItems(ui.Stretch).Clip().Children(func() {
			a.settingsNav(c)
			ui.Column(c).Width(1).FillHeight().Background(t.Border)
			a.settingsContent(c)
		})
	})
}

// settingsNav is the left list of the settings window: a search field and the
// sections, with the chosen one highlighted.
func (a *app) settingsNav(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Width(220).FillHeight().Padding(12).Gap(6).Children(func() {
		ui.TextInput(c, &a.settingsQuery).Placeholder("搜索设置").Margin(0, 0, 0, 6)
		items := []struct {
			idx   int
			label string
			icon  *ui.SVG
		}{
			{0, "外观", iconSliders},
			{1, "编辑器", iconCode},
			{2, "快捷键", iconKeyboard},
			{3, "关于", iconInfo},
		}
		q := strings.ToLower(strings.TrimSpace(a.settingsQuery))
		for _, it := range items {
			it := it
			if q != "" && !strings.Contains(strings.ToLower(it.label), q) {
				continue
			}
			active := a.settingsSection == it.idx
			row := ui.Row(c).Gap(10).AlignItems(ui.Center).Padding(7, 10).Radius(8).
				OnClick(func() { a.settingsSection = it.idx })
			switch {
			case active:
				row = row.Background(t.Surface).Border(1, t.Border)
			case row.Hovered():
				row = row.Background(t.SurfaceHover)
			}
			row.Children(func() {
				col := t.TextMuted
				if active {
					col = t.Text
				}
				ui.Icon(c, it.icon).FontSize(a.us(14)).TextColor(col)
				ui.Text(c, it.label).FontSize(a.us(12.5)).TextColor(col)
			})
		}
	})
}

// settingsContent is the right pane: a breadcrumb, the chosen section's cards,
// and the footer buttons.
func (a *app) settingsContent(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).FillHeight().Padding(18, 22).Gap(14).Children(func() {
		title := "外观"
		switch a.settingsSection {
		case 1:
			title = "编辑器"
		case 2:
			title = "快捷键"
		case 3:
			title = "关于"
		}
		ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(c, "设置").FontSize(a.us(12.5)).TextColor(t.TextMuted)
			ui.Text(c, "›").FontSize(a.us(12)).TextColor(t.Border)
			ui.Text(c, title).FontSize(a.us(15)).Bold()
		})

		switch a.settingsSection {
		case 1:
			a.editorSettings(c)
		case 2:
			a.shortcutsSettings(c)
		case 3:
			a.aboutSettings(c)
		default:
			a.appearanceSettings(c)
		}

		ui.Spacer(c)
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if a.settingsSection <= 1 && ui.Button(c, "恢复默认").Clicked() {
				a.resetFontSettings()
				if a.settingsSection == 0 {
					a.setAccent("")
				}
			}
			if ui.PrimaryButton(c, "完成").Clicked() {
				a.settingsOpen = false
			}
		})
	})
}

// appearanceSettings is the "外观" section: the color mode and the interface
// font.
func (a *app) appearanceSettings(c *ui.Context) {
	t := c.Theme()
	a.settingsCard(c, func() {
		a.settingsRow(c, "颜色模式", func() {
			ui.Row(c).Padding(2).Gap(2).Radius(8).Background(t.Background).Border(1, t.Border).Children(func() {
				a.segment(c, "浅色", !a.dark, func() { a.setDark(false) })
				a.segment(c, "深色", a.dark, func() { a.setDark(true) })
			})
		})
		ui.Divider(c)
		a.settingsRow(c, "主题色", func() {
			a.accentSwatches(c)
		})
		ui.Divider(c)
		a.settingsRow(c, "界面字体", func() {
			ui.Select(c, &a.uiFontChoice, uiFontOptions).Width(200)
		})
		ui.Divider(c)
		a.settingsRow(c, "界面字号", func() {
			a.sizeStepper(c, &a.uiFontSize, 11, 20, 1)
		})
	})
	ui.Text(c, "界面字号会等比缩放整个应用窗口里的文字。").
		FontSize(a.us(10.5)).TextColor(t.TextMuted)
}

// accentSwatches is the theme color picker: one round swatch per preset, with a
// check on the active one.
func (a *app) accentSwatches(c *ui.Context) {
	t := c.Theme()
	active := accentHex(a.accent)
	ui.Row(c).Gap(9).AlignItems(ui.Center).Children(func() {
		for _, p := range accentPresets {
			p := p
			selected := p.Color == active
			ring := ui.Row(c).Width(24).Height(24).Center().Justify(ui.Center).Radius(12).
				OnClick(func() { a.setAccent(p.Color) }).Tooltip(p.Name).Focusable().Label(p.Name).
				AccessSelected(selected)
			if selected {
				ring = ring.Border(2, t.Text)
			}
			ring.Children(func() {
				dot := ui.Row(c).Width(16).Height(16).Center().Justify(ui.Center).Radius(8).
					Background(ui.Hex(p.Color))
				dot.Children(func() {
					if selected {
						ui.Text(c, "✓").FontSize(a.us(9)).Bold().
							TextColor(contrastText(ui.Hex(p.Color)))
					}
				})
			})
		}
	})
}

// editorSettings is the "编辑器" section: the code editor's font.
func (a *app) editorSettings(c *ui.Context) {
	t := c.Theme()
	a.settingsCard(c, func() {
		a.settingsRow(c, "编辑器字体", func() {
			ui.Select(c, &a.editorFontChoice, editorFontOptions).Width(200)
		})
		ui.Divider(c)
		a.settingsRow(c, "编辑器字号", func() {
			a.sizeStepper(c, &a.editorFontSize, 10, 28, 1)
		})
	})
	ui.Text(c, "只影响请求与响应报文编辑区，不影响其它界面文字。").
		FontSize(a.us(10.5)).TextColor(t.TextMuted)
}

// aboutSettings is the "关于" section: the app's name, version and purpose.
func (a *app) aboutSettings(c *ui.Context) {
	t := c.Theme()
	a.settingsCard(c, func() {
		a.settingsRow(c, "名称", func() {
			ui.Text(c, "soap-go").FontSize(a.us(12.5)).TextColor(t.TextMuted)
		})
		ui.Divider(c)
		a.settingsRow(c, "版本", func() {
			ui.Text(c, appVersion).FontSize(a.us(12.5)).TextColor(t.Text)
		})
		ui.Divider(c)
		a.settingsRow(c, "说明", func() {
			ui.Text(c, "SOAP / WSDL 接口调试桌面工具").FontSize(a.us(12.5)).TextColor(t.TextMuted)
		})
	})
}

// shortcutsSettings is the "快捷键" section: the window-wide keyboard shortcuts.
func (a *app) shortcutsSettings(c *ui.Context) {
	t := c.Theme()
	a.settingsCard(c, func() {
		for i, s := range shortcutHelp {
			s := s
			if i > 0 {
				ui.Divider(c)
			}
			ui.Row(c).Padding(11, 14).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, s.keys).FontSize(a.us(11.5)).Bold().TextColor(t.Accent).Width(170)
				ui.Text(c, s.desc).FontSize(a.us(11.5)).TextColor(t.Text)
			})
		}
	})
	ui.Text(c, "以下快捷键在本窗口内生效。").
		FontSize(a.us(10.5)).TextColor(t.TextMuted)
}

// settingsCard is a rounded group of rows inside a section.
func (a *app) settingsCard(c *ui.Context, build func()) {
	t := c.Theme()
	ui.Column(c).Radius(10).Background(t.Surface).Border(1, t.Border).Clip().Children(build)
}

// settingsRow is one label-on-the-left, control-on-the-right row of a card.
func (a *app) settingsRow(c *ui.Context, label string, control func()) {
	ui.Row(c).Padding(11, 14).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(a.us(12.5))
		ui.Spacer(c)
		control()
	})
}

// sizeStepper is the horizontal font-size control of the settings dialog: a
// minus button, the value, and a plus button, after the reference.
func (a *app) sizeStepper(c *ui.Context, value *float64, lo, hi, step float64) {
	a.stepBtn(c, "−", *value > lo, func() {
		if *value-step < lo {
			*value = lo
		} else {
			*value -= step
		}
	})
	ui.Text(c, fmt.Sprintf("%.0f", *value)).FontSize(a.us(12.5)).Width(44).TextAlign(ui.Center)
	a.stepBtn(c, "+", *value < hi, func() {
		if *value+step > hi {
			*value = hi
		} else {
			*value += step
		}
	})
}

// stepBtn is one small square button of sizeStepper, dimmed when disabled.
func (a *app) stepBtn(c *ui.Context, label string, enabled bool, onClick func()) {
	t := c.Theme()
	btn := ui.Row(c).Width(24).Height(24).Center().Justify(ui.Center).Radius(6)
	col := t.TextMuted
	if enabled {
		col = t.Text
		btn = btn.OnClick(onClick).Focusable().Label(label)
		if btn.Hovered() {
			btn = btn.Background(t.SurfaceHover)
		}
	}
	btn.Children(func() {
		ui.Text(c, label).FontSize(a.us(14)).TextColor(col)
	})
}

// segment is one choice of a small segmented control.
func (a *app) segment(c *ui.Context, label string, active bool, onClick func()) {
	t := c.Theme()
	box := ui.Row(c).Padding(3, 12).Radius(6).OnClick(onClick).Focusable().Label(label)
	switch {
	case active:
		box = box.Background(t.SurfacePressed)
	case box.Hovered():
		box = box.Background(t.SurfaceHover)
	}
	box.Children(func() {
		col := t.TextMuted
		if active {
			col = t.Text
		}
		ui.Text(c, label).FontSize(a.us(11.5)).TextColor(col)
	})
}

func (a *app) splitSwitch(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Shrink(0).Padding(2).Gap(2).Background(t.Background).
		Border(1, t.Border).Radius(8).Children(func() {
		a.splitOption(c, 0, iconSplitV, "上下分屏")
		a.splitOption(c, 1, iconSplitH, "左右分屏")
	})
}

func (a *app) splitOption(c *ui.Context, mode int, icon *ui.SVG, tip string) {
	t := c.Theme()
	active := a.splitSide == mode
	btn := ui.Row(c).Width(28).Height(24).Center().Justify(ui.Center).Radius(6).
		OnClick(func() { a.splitSide = mode; a.saveUIState() }).Tooltip(tip).Focusable().Label(tip)
	if active {
		btn = btn.Background(t.SurfacePressed)
	} else if btn.Hovered() {
		btn = btn.Background(t.SurfaceHover)
	}
	btn.Children(func() {
		col := t.TextMuted
		if active {
			col = t.Accent
		}
		ui.Icon(c, icon).FontSize(a.us(14)).TextColor(col)
	})
}

func (a *app) iconBtn(c *ui.Context, icon *ui.SVG, tip string, onClick func()) {
	t := c.Theme()
	btn := ui.Row(c).Width(26).Height(24).Center().Justify(ui.Center).Radius(6).
		OnClick(onClick).Tooltip(tip).Focusable().Label(tip)
	if btn.Hovered() {
		btn = btn.Background(t.SurfaceHover)
	}
	btn.Children(func() {
		ui.Icon(c, icon).FontSize(a.us(13)).TextColor(t.TextMuted)
	})
}

// closeIconButton is a small circular × for dismissing a row or a dialog: a
// round hit area that turns red on hover, like the tab strip's close button.
func (a *app) closeIconButton(c *ui.Context, size float32, label string, onClick func()) {
	t := c.Theme()
	btn := ui.Row(c).Width(size).Height(size).Center().Justify(ui.Center).
		Radius(size / 2).Cursor(ui.CursorPointer).Label(label).Focusable().
		Transition(ui.ElementTransition{Duration: 100 * time.Millisecond, Colors: true}).
		OnClick(onClick)
	hovered := btn.Hovered()
	col := t.TextMuted
	if hovered {
		btn = btn.Background(t.Danger.Alpha(0.20))
		col = t.Danger
	}
	btn.Children(func() {
		ui.Icon(c, iconClose).FontSize(a.us(size * 0.5)).TextColor(col)
	})
}

// iconMenuBtn is an icon button that opens a popover menu on click, for the
// "more" actions of a toolbar or a panel header.
func (a *app) iconMenuBtn(c *ui.Context, icon *ui.SVG, tip string, build func(m *ui.Menu)) {
	t := c.Theme()
	btn := ui.ButtonBase(c).Width(26).Height(24).Center().Justify(ui.Center).Radius(6).
		BorderWidth(0).Background(ui.Transparent).Label(tip).Tooltip(tip)
	if btn.Hovered() {
		btn = btn.Background(t.SurfaceHover)
	}
	btn.Children(func() {
		ui.Icon(c, icon).FontSize(a.us(13)).TextColor(t.TextMuted)
	})
	btn.PopoverMenuButton(build)
}

// flatButton is a compact, borderless text button for dense toolbars: small
// type, tight padding and a hover face instead of a border.
func (a *app) flatButton(c *ui.Context, label string, onClick func()) {
	t := c.Theme()
	btn := ui.ButtonBase(c).Padding(2, 7).Radius(6)
	if btn.Hovered() {
		btn = btn.Background(t.SurfaceHover)
	}
	btn.Children(func() {
		ui.Text(c, label).FontSize(a.us(11)).TextColor(t.Text)
	})
	if btn.Clicked() {
		onClick()
	}
}

// plainButton is a borderless button for panel headers: normal text size with a
// hover face and no outline. It reports whether it was clicked.
func (a *app) plainButton(c *ui.Context, label string) bool {
	t := c.Theme()
	btn := ui.ButtonBase(c).Padding(4, 10).Radius(6).BorderWidth(0)
	if btn.Hovered() {
		btn = btn.Background(t.SurfaceHover)
	}
	btn.Children(func() {
		ui.Text(c, label).FontSize(a.us(11.5)).TextColor(t.Text)
	})
	return btn.Clicked()
}

// copyIcon is a small copy button overlaid on an editor's top-right corner,
// after the copy control of a Markdown code block: it writes the editor's
// text to the clipboard and reports it in the status bar.
func (a *app) copyIcon(c *ui.Context, copied string, text func() string) ui.Element {
	t := c.Theme()
	btn := ui.Row(c).Width(26).Height(24).Center().Justify(ui.Center).Radius(6).
		Background(t.Surface).Border(1, t.Border).Tooltip("复制").Focusable().Label("复制").
		OnClick(func() {
			mygo.Clipboard.WriteText(text())
			a.statusMsg = copied
		})
	if btn.Hovered() {
		btn = btn.Background(t.SurfaceHover)
	}
	return btn.Children(func() {
		ui.Icon(c, iconCopy).FontSize(a.us(13)).TextColor(t.TextMuted)
	})
}

// editorWithCopy puts an editor in a container whose copy button sits in its
// top-right corner and shows only while the pointer is over the region, as a
// Markdown code block's copy control does. The button stays built and
// reachable for assistive technology; at rest it is transparent and lets the
// press fall through to the editor.
func (a *app) editorWithCopy(c *ui.Context, ed *editor.State, copied string, text func() string) {
	box := ui.Column(c).Grow(1).FillWidth()
	hovered := box.Hovered()
	box.Children(func() {
		editor.View(c, ed).Grow(1).FillWidth()
		btn := a.copyIcon(c, copied, text).Absolute().Right(18).Top(8).IgnoreInput(!hovered)
		if !hovered {
			btn.Opacity(0)
		}
	})
}

func (a *app) statusPill(c *ui.Context, s string, col ui.Color) {
	ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(2, 8).Radius(6).Background(col.Alpha(0.15)).Children(func() {
		ui.Text(c, "●").FontSize(a.us(8)).TextColor(col)
		ui.Text(c, s).FontSize(a.us(10.5)).TextColor(col).Bold()
	})
}

// isSensitiveName reports whether a header carries credentials and is masked
// by default, as the browser app does.
func isSensitiveName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return false
	}
	switch lower {
	case "authorization", "proxy-authorization", "cookie", "set-cookie":
		return true
	}
	for _, frag := range []string{"token", "secret", "password", "apikey", "api-key", "x-auth"} {
		if strings.Contains(lower, frag) {
			return true
		}
	}
	return false
}

// ---- look helpers ----

// kindIcon returns the folder icon a tree row draws instead of a glyph, open
// when expanded and closed when not: projects and native folders read as
// directories, the way a file explorer shows them.
func kindIcon(kind string, expanded bool) *ui.SVG {
	switch kind {
	case "project", "group":
		if expanded {
			return iconFolderOpen
		}
		return iconFolder
	}
	return nil
}

func kindGlyph(kind string) string {
	switch kind {
	case "project":
		return "▣"
	case "group":
		return "▤"
	case "service":
		return "◈"
	case "operation":
		return "ƒ"
	default:
		return "→"
	}
}

func kindColor(t *ui.Theme, kind string) ui.Color {
	switch kind {
	case "project":
		return t.Accent
	case "group":
		return t.TextMuted
	case "service":
		return t.TextMuted
	case "operation":
		return t.Success
	default:
		return t.TextMuted
	}
}
