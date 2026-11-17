package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// Icons drawn as SVG, so the chrome does not fall back to the look of a
// platform button. They take the text color of their element.
var (
	iconSliders  = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><line x1="4" y1="21" x2="4" y2="14"/><line x1="4" y1="10" x2="4" y2="3"/><line x1="12" y1="21" x2="12" y2="12"/><line x1="12" y1="8" x2="12" y2="3"/><line x1="20" y1="21" x2="20" y2="16"/><line x1="20" y1="12" x2="20" y2="3"/><line x1="1" y1="14" x2="7" y2="14"/><line x1="9" y1="8" x2="15" y2="8"/><line x1="17" y1="16" x2="23" y2="16"/></svg>`))
	iconCode     = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>`))
	iconInfo     = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><line x1="12" y1="11" x2="12" y2="16.5"/><line x1="12" y1="7.5" x2="12" y2="7.6"/></svg>`))
	iconMore     = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor"><circle cx="5" cy="12" r="1.7"/><circle cx="12" cy="12" r="1.7"/><circle cx="19" cy="12" r="1.7"/></svg>`))
	iconSplitV   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="3.5" y="4.5" width="17" height="15" rx="3"/><path d="M3.5 12h17"/></svg>`))
	iconSplitH   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="3.5" y="4.5" width="17" height="15" rx="3"/><path d="M12 4.5v15"/></svg>`))
	iconPlus     = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M12 5v14M5 12h14"/></svg>`))
	iconCopy     = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="12" height="12" rx="2.5"/><path d="M6 15H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v1"/></svg>`))
	iconSun      = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><circle cx="12" cy="12" r="4.2"/><path d="M12 2.6v2M12 19.4v2M2.6 12h2M19.4 12h2M5.1 5.1l1.4 1.4M17.5 17.5l1.4 1.4M18.9 5.1l-1.4 1.4M6.5 17.5l-1.4 1.4"/></svg>`))
	iconMoon     = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M20 14.5A8.5 8.5 0 1 1 9.5 4a7 7 0 0 0 10.5 10.5Z"/></svg>`))
	iconSettings = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3.2"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>`))
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
		ui.Row(c).Grow(1).Fill().AlignItems(ui.Stretch).Children(func() {
			a.sidebar(c)
			a.main(c)
		})
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

// topBar is a slim, draggable breadcrumb bar with the workspace menu, after
// Yaak.
func (a *app) topBar(c *ui.Context) {
	t := c.Theme()
	bar := c.TitleBar()
	h := bar.Height
	if h < 44 {
		h = 44
	}
	ui.Row(c).Shrink(0).Height(h).
		Padding(0, bar.Right+10, 0, 14).Gap(8).AlignItems(ui.Center).
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

func (a *app) sidebar(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Width(a.sideW).Shrink(0).FillHeight().Background(t.Surface).Children(func() {
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
				if m.Item("导出 SoapUI XML…").Chosen() {
					a.saveProjectXML()
				}
			})
		})
		ui.Row(c).Padding(0, 12, 8, 12).Children(func() {
			ui.TextInput(c, &a.search).Placeholder("搜索接口").Grow(1).Ref(&a.searchRef)
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
			for _, it := range items {
				if a.search != "" && !strings.Contains(strings.ToLower(it.label), strings.ToLower(a.search)) {
					continue
				}
				a.treeRow(c, it)
			}
		})
	})
}

// treeItem is one rendered row of the project tree.
type treeItem struct {
	kind          string // project | service | operation | call
	pid, sid, oid string
	cid           string
	label, badge  string
	depth         int
	hasChildren   bool
	expanded      bool
	status        int
}

func (a *app) treeItems() []treeItem {
	var out []treeItem
	for _, p := range a.projects {
		pe := a.expandedAt(p.ID, true)
		out = append(out, treeItem{kind: "project", pid: p.ID, label: p.Name, badge: p.Source, hasChildren: len(p.Services) > 0, expanded: pe})
		if !pe {
			continue
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
					st := 0
					if cl.Response != nil {
						st = cl.Response.Status
					}
					out = append(out, treeItem{
						kind: "call", pid: p.ID, sid: s.ID, oid: o.ID, cid: cl.ID,
						label: cl.Title, depth: 3, status: st,
					})
				}
			}
		}
	}
	return out
}

func (a *app) treeRow(c *ui.Context, it treeItem) {
	t := c.Theme()
	selected := (it.kind == "call" && a.cid == it.cid && it.cid != "") ||
		(it.kind == "project" && it.pid == a.pid && a.cid == "" && a.sid == "") ||
		(it.kind == "service" && it.pid == a.pid && it.sid == a.sid && a.oid == "") ||
		(it.kind == "operation" && it.oid == a.oid && a.cid == "")

	editing := a.renamingID != "" && a.renamingID == renameTargetID(it)
	row := ui.Row(c).Gap(6).AlignItems(ui.Center).
		Padding(4, 6, 4, float32(4+it.depth*14))
	if !editing {
		row = row.OnClick(func() { a.selectTree(it) }).
			ContextMenu(func(m *ui.Menu) { a.treeContextMenu(m, it) })
	}
	switch {
	case selected:
		row = row.Background(t.Accent.Alpha(0.14)).Radius(6)
	case !editing && row.Hovered():
		row = row.Background(t.SurfaceHover).Radius(6)
	}
	row.Children(func() {
		if selected {
			ui.Column(c).Width(2).Height(15).Background(t.Accent).Radius(1)
		} else {
			ui.Text(c, "").Width(2)
		}
		if it.hasChildren {
			caret := "▸"
			if it.expanded {
				caret = "▾"
			}
			ct := ui.Text(c, caret).FontSize(a.us(9.5)).TextColor(t.TextMuted).Width(11)
			if !editing {
				ct.OnClick(func() {
					switch it.kind {
					case "project", "service", "operation":
						a.toggleExpanded(nodeKey(it), true)
					}
				})
			}
		} else {
			ui.Text(c, "").Width(11)
		}
		if it.kind == "call" {
			ui.Text(c, "POST").FontSize(a.us(9)).Bold().TextColor(t.Accent).Width(30)
		} else {
			ui.Text(c, kindGlyph(it.kind)).FontSize(a.us(11)).TextColor(kindColor(t, it.kind)).Width(14)
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
}

// treeContextMenu is the right-click menu of a tree row: the create/rename/
// delete actions of that node, where the node is.
func (a *app) treeContextMenu(m *ui.Menu, it treeItem) {
	if a.renamingID != "" {
		a.cancelInlineRename()
	}
	switch it.kind {
	case "project":
		a.pid, a.sid, a.oid, a.cid = it.pid, "", "", ""
		if m.Item("新建服务…").Chosen() {
			a.treeModalOpen, a.treeModalKind, a.nameInput = true, 1, ""
		}
		if m.Item("重命名项目…").Chosen() {
			name := ""
			if p := a.findProject(it.pid); p != nil {
				name = p.Name
			}
			a.startInlineRename(it.pid, name)
		}
		if m.Item("代理设置…").Chosen() {
			a.openProxy()
		}
		if m.Item("导出 SoapUI XML…").Chosen() {
			a.saveProjectXML()
		}
		m.Separator()
		if m.Item("删除项目").Danger().Chosen() {
			a.deleteProject()
		}
	case "service":
		a.pid, a.sid, a.oid, a.cid = it.pid, it.sid, "", ""
		if m.Item("新建操作…").Chosen() {
			a.treeModalOpen, a.treeModalKind, a.nameInput = true, 2, ""
		}
		m.Separator()
		if m.Item("删除服务").Danger().Chosen() {
			a.deleteService()
		}
	case "operation":
		a.pid, a.sid, a.oid, a.cid = it.pid, it.sid, it.oid, ""
		if m.Item("新建请求样例").Chosen() {
			a.newCall()
		}
		m.Separator()
		if m.Item("删除操作").Danger().Chosen() {
			a.deleteOperation()
		}
	case "call":
		a.pid, a.sid, a.oid, a.cid = it.pid, it.sid, it.oid, it.cid
		if m.Item("打开").Chosen() {
			a.openCall(it.pid, it.sid, it.oid, it.cid)
		}
		if m.Item("重命名请求…").Chosen() {
			a.startInlineRename(it.cid, it.label)
		}
		if m.Item("复制请求样例").Chosen() {
			a.duplicateCall()
		}
		m.Separator()
		if m.Item("删除请求样例").Danger().Chosen() {
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
	switch it.kind {
	case "project":
		a.pid, a.sid, a.oid, a.cid = it.pid, "", "", ""
		a.toggleExpanded(it.pid, true)
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
		a.urlBar(c)
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
		Background(t.Background).Children(func() {
		for i, key := range a.tabs {
			i, key := i, key
			active := i == a.active
			tab := ui.Row(c).Shrink(0).Height(30).Padding(0, 10).Gap(8).AlignItems(ui.Center).
				OnClick(func() {
					pid, sid, oid, cid := splitKey(key)
					a.openCall(pid, sid, oid, cid)
				})
			if active {
				tab = tab.Background(t.Surface).Radius(6)
			} else if tab.Hovered() {
				tab = tab.Background(t.SurfaceHover).Radius(6)
			}
			tab.Children(func() {
				ui.Text(c, "POST").FontSize(a.us(8.5)).Bold().TextColor(t.Accent)
				lc := t.TextMuted
				if active {
					lc = t.Text
				}
				ui.Text(c, a.callTitle(key)).FontSize(a.us(12)).TextColor(lc).SingleLine().MaxWidth(180)
				if active && a.isDirty() {
					ui.Text(c, "●").FontSize(a.us(8)).TextColor(t.Accent).Shrink(0).
						Tooltip("有未保存的修改")
				}
				ui.Text(c, "×").FontSize(a.us(12)).TextColor(t.TextMuted).
					OnClick(func() { a.closeTab(i) })
			})
		}
		ui.Spacer(c)
		a.splitSwitch(c)
		ui.Text(c, "").Width(4)
	})
	ui.Divider(c)
}

func (a *app) urlBar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Shrink(0).Padding(10, 14).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "POST").FontSize(a.us(10)).Bold().TextColor(t.AccentText).
			Padding(6, 10).Background(t.Accent).Radius(7)
		ui.TextInput(c, &a.endpoint).Placeholder("http://host/service.asmx").Grow(1).OnSubmit(func() { a.sendRequest() })
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
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Row(c).Shrink(0).Padding(6, 14, 0).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Tabs(c, &a.reqSection,
				"报文",
				fmt.Sprintf("请求头 %d", len(a.headers)+2),
				"认证",
				"设置",
			).Shrink(0)
		})
		switch a.reqSection {
		case 1:
			a.headersPanel(c)
		case 2:
			ui.Column(c).Grow(1).Center().Justify(ui.Center).Children(func() {
				ui.Text(c, "SOAP 使用 HTTP Basic / WS-Security，认证配置后续接入").FontSize(a.us(11)).TextColor(t.TextMuted)
			})
		case 3:
			a.settingsPanel(c)
		default:
			ui.Column(c).Grow(1).Padding(10, 12, 10).Gap(8).Children(func() {
				ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "XML 报文").FontSize(a.us(11)).TextColor(t.TextMuted).Bold()
					ui.Spacer(c)
					a.flatButton(c, "格式化", a.formatBody)
					a.flatButton(c, "压缩", a.minifyBody)
					a.flatButton(c, "复制为 curl", a.copyCurl)
					a.flatButton(c, "保存请求", a.saveCall)
				})
				a.editorWithCopy(c, a.xmlEd, "请求报文已复制", func() string { return a.xmlEd.Buffer.String() })
			})
		}
	})
}

func (a *app) headersPanel(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Padding(14, 14, 10).Gap(8).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "请求头").FontSize(a.us(11.5)).Bold()
			ui.Spacer(c)
			ui.Checkbox(c, &a.maskSecrets, "遮蔽敏感头")
			if ui.Button(c, "＋ 添加请求头").Clicked() {
				a.headers = append(a.headers, headerRow{})
			}
		})
		ui.Text(c, "SOAPAction 与 Content-Type 默认存在；项目默认头会叠加，样例头可覆盖同名项。").
			FontSize(10.5).TextColor(t.TextMuted)
		ui.Divider(c)

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
		for i := range a.headers {
			i := i
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &a.headers[i].Name).Placeholder("Header-Name").Width(190)
				if a.maskSecrets && isSensitiveName(a.headers[i].Name) {
					ui.Text(c, "••••••（已遮蔽）").FontSize(a.us(11)).TextColor(t.TextMuted).Grow(1)
				} else {
					ui.TextInput(c, &a.headers[i].Value).Placeholder("值").Grow(1)
				}
				ui.Text(c, "×").FontSize(a.us(13)).TextColor(t.TextMuted).Width(18).
					OnClick(func() {
						if i >= 0 && i < len(a.headers) {
							a.headers = append(a.headers[:i], a.headers[i+1:]...)
						}
					})
			})
		}
		ui.Spacer(c)
	})
}

func (a *app) settingsPanel(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Padding(14, 14, 10).Gap(14).Children(func() {
		ui.Text(c, "请求设置").FontSize(a.us(11.5)).Bold()
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "超时").FontSize(a.us(11)).TextColor(t.TextMuted).Width(72)
			ui.TextInput(c, &a.timeout).Placeholder("60000").Width(110)
			ui.Text(c, "ms").FontSize(a.us(11)).TextColor(t.TextMuted)
			ui.Spacer(c)
			ui.Text(c, "范围 1000 – 600000").FontSize(a.us(10.5)).TextColor(t.TextMuted)
		})
		ui.Divider(c)
		ui.Text(c, "请求方式").FontSize(a.us(11.5)).Bold()
		ui.Text(c, "SOAP 请求固定为 POST。").FontSize(a.us(10.5)).TextColor(t.TextMuted)
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
				ui.Text(c, "发送失败："+a.resp.Error).FontSize(a.us(10.5)).TextColor(t.Danger).SingleLine()
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
			if ui.Tabs(c, &a.respTab, "格式化", "原始", "请求", "Headers").Shrink(0).Changed() {
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
		if a.errMsg != "" {
			ui.Text(c, a.errMsg).FontSize(a.us(10.5)).TextColor(t.Danger).SingleLine()
		} else if a.statusMsg != "" {
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
				ui.Text(c, "×").FontSize(a.us(15)).TextColor(t.TextMuted).
					OnClick(func() { a.importOpen = false })
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
		Border(1, t.Border).OnClick(onClick)
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
				ui.Text(c, "×").FontSize(a.us(15)).TextColor(t.TextMuted).
					OnClick(func() { a.exportOpen = false })
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
				ui.Text(c, "×").FontSize(a.us(15)).TextColor(t.TextMuted).
					OnClick(func() { a.proxyOpen = false })
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
				ui.Text(c, "×").FontSize(a.us(15)).TextColor(t.TextMuted).
					OnClick(func() { a.newProjectOpen = false })
			})
			ui.Text(c, "项目名称").FontSize(a.us(10.5)).TextColor(t.TextMuted)
			ui.TextInput(c, &a.nameInput).Placeholder("新建项目").Grow(1)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.newProjectOpen = false
				}
				if ui.PrimaryButton(c, "确定").Clicked() {
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
				ui.Text(c, "×").FontSize(a.us(15)).TextColor(t.TextMuted).
					OnClick(func() { a.wsOpen = false })
			})
			ui.Text(c, "名称").FontSize(a.us(10.5)).TextColor(t.TextMuted)
			ui.TextInput(c, &a.nameInput).Placeholder("工作空间名称").Grow(1)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.wsOpen = false
				}
				if ui.PrimaryButton(c, "确定").Clicked() {
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
		}
		ui.Column(c).Width(420).Gap(12).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, title).FontSize(a.us(16)).Bold()
				ui.Spacer(c)
				ui.Text(c, "×").FontSize(a.us(15)).TextColor(t.TextMuted).
					OnClick(func() { a.treeModalOpen = false })
			})
			ui.Text(c, placeholder).FontSize(a.us(10.5)).TextColor(t.TextMuted)
			ui.TextInput(c, &a.nameInput).Placeholder(placeholder).Grow(1)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.treeModalOpen = false
				}
				if ui.PrimaryButton(c, "确定").Clicked() {
					switch a.treeModalKind {
					case 1:
						a.addService(a.nameInput)
					case 2:
						a.addOperation(a.nameInput)
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
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.confirmOpen = false
					a.confirmAction = nil
				}
				if ui.PrimaryButton(c, "确定").Clicked() {
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
			{2, "关于", iconInfo},
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
			title = "关于"
		}
		ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(c, "设置").FontSize(a.us(12.5)).TextColor(t.TextMuted)
			ui.Text(c, "›").FontSize(a.us(12)).TextColor(t.Border)
			ui.Text(c, title).FontSize(a.us(15)).Bold()
			ui.Spacer(c)
			ui.Text(c, "×").FontSize(a.us(15)).TextColor(t.TextMuted).
				OnClick(func() { a.settingsOpen = false })
		})

		switch a.settingsSection {
		case 1:
			a.editorSettings(c)
		case 2:
			a.aboutSettings(c)
		default:
			a.appearanceSettings(c)
		}

		ui.Spacer(c)
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if a.settingsSection != 2 && ui.Button(c, "恢复默认").Clicked() {
				a.resetFontSettings()
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
		btn = btn.OnClick(onClick)
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
	box := ui.Row(c).Padding(3, 12).Radius(6).OnClick(onClick)
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
		OnClick(func() { a.splitSide = mode; a.saveUIState() }).Tooltip(tip)
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
		OnClick(onClick).Tooltip(tip)
	if btn.Hovered() {
		btn = btn.Background(t.SurfaceHover)
	}
	btn.Children(func() {
		ui.Icon(c, icon).FontSize(a.us(13)).TextColor(t.TextMuted)
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

// copyIcon is a small copy button overlaid on an editor's top-right corner,
// after the copy control of a Markdown code block: it writes the editor's
// text to the clipboard and reports it in the status bar.
func (a *app) copyIcon(c *ui.Context, copied string, text func() string) ui.Element {
	t := c.Theme()
	btn := ui.Row(c).Width(26).Height(24).Center().Justify(ui.Center).Radius(6).
		Background(t.Surface).Border(1, t.Border).Tooltip("复制").
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

func kindGlyph(kind string) string {
	switch kind {
	case "project":
		return "▣"
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
	case "service":
		return t.TextMuted
	case "operation":
		return t.Success
	default:
		return t.TextMuted
	}
}
