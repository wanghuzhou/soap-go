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
	iconSplitV = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="3.5" y="4.5" width="17" height="15" rx="3"/><path d="M3.5 12h17"/></svg>`))
	iconSplitH = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="3.5" y="4.5" width="17" height="15" rx="3"/><path d="M12 4.5v15"/></svg>`))
	iconPlus   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M12 5v14M5 12h14"/></svg>`))
	iconCopy   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="12" height="12" rx="2.5"/><path d="M6 15H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v1"/></svg>`))
	iconSun    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><circle cx="12" cy="12" r="4.2"/><path d="M12 2.6v2M12 19.4v2M2.6 12h2M19.4 12h2M5.1 5.1l1.4 1.4M17.5 17.5l1.4 1.4M18.9 5.1l-1.4 1.4M6.5 17.5l-1.4 1.4"/></svg>`))
	iconMoon   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M20 14.5A8.5 8.5 0 1 1 9.5 4a7 7 0 0 0 10.5 10.5Z"/></svg>`))
)

// ---- view ----

func (a *app) view(c *ui.Context) {
	c.SetTheme(a.theme())
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
	})
	a.maybeSaveUIState()
}

// topBar is a slim, draggable breadcrumb bar with the file and workspace
// menus, after Yaak.
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
			ui.Text(c, "S").FontSize(12).Bold().TextColor(t.AccentText)
		})
		ui.Text(c, "soap-go").FontSize(13).Bold()
		ui.Text(c, "/").FontSize(12).TextColor(t.Border)
		ui.Text(c, a.info.Name).FontSize(12).TextColor(t.TextMuted)
		if p := a.selectedProject(); p != nil {
			ui.Text(c, "›").FontSize(12).TextColor(t.Border)
			ui.Text(c, p.Name).FontSize(12).TextColor(t.TextMuted).SingleLine()
		}

		// File menu.
		ui.PopoverMenuButton(c, "文件", func(m *ui.Menu) {
			if m.Item("新建项目…").Chosen() {
				a.newProjectOpen, a.projectModalKind, a.nameInput = true, 0, ""
			}
			if m.Item("重命名项目…").Chosen() {
				if p := a.selectedProject(); p != nil {
					a.newProjectOpen, a.projectModalKind, a.nameInput = true, 1, p.Name
				}
			}
			if m.Item("删除项目").Danger().Chosen() {
				a.deleteProject()
			}
			m.Separator()
			if m.Item("导出 SoapUI XML…").Chosen() {
				a.saveProjectXML()
			}
			if m.Item("代理设置…").Chosen() {
				a.openProxy()
			}
			m.Separator()
			if m.Item("导入…").Chosen() {
				a.importOpen, a.importKind = true, 0
			}
			if m.Item("导出…").Chosen() {
				a.exportOpen = true
			}
		}).BorderWidth(0)

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
			if m.Item("删除当前工作空间").Danger().Chosen() {
				a.deleteWorkspace()
			}
		}).BorderWidth(0)
		ui.Spacer(c)

		toggleTheme := func() {
			a.dark = !a.dark
			a.applyWindowButtons()
			a.saveUIState()
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
			ui.Text(c, "项目").FontSize(11.5).Bold()
			ui.Badge(c, fmt.Sprint(len(a.projects)))
			ui.Spacer(c)
			a.iconBtn(c, iconPlus, "新建项目", func() {
				a.newProjectOpen, a.projectModalKind, a.nameInput = true, 0, ""
			})
		})
		ui.Row(c).Padding(0, 12, 8, 12).Children(func() {
			ui.TextInput(c, &a.search).Placeholder("搜索接口").Grow(1).Ref(&a.searchRef)
		})
		ui.Scroll(c).Grow(1).Padding(4, 6, 6).Children(func() {
			items := a.treeItems()
			if len(items) == 0 {
				ui.Column(c).Padding(16).Gap(6).Children(func() {
					ui.Text(c, "还没有项目").FontSize(12.5).Bold()
					ui.Text(c, "用「文件 › 导入」导入 SoapUI 项目或 WSDL。").FontSize(10.5).TextColor(t.TextMuted).Wrap()
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

	row := ui.Row(c).Gap(6).AlignItems(ui.Center).
		Padding(4, 6, 4, float32(4+it.depth*14)).
		OnClick(func() { a.selectTree(it) }).
		ContextMenu(func(m *ui.Menu) { a.treeContextMenu(m, it) })
	switch {
	case selected:
		row = row.Background(t.Accent.Alpha(0.14)).Radius(6)
	case row.Hovered():
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
			ui.Text(c, caret).FontSize(9.5).TextColor(t.TextMuted).Width(11).
				OnClick(func() {
					switch it.kind {
					case "project", "service", "operation":
						a.toggleExpanded(nodeKey(it), true)
					}
				})
		} else {
			ui.Text(c, "").Width(11)
		}
		if it.kind == "call" {
			ui.Text(c, "POST").FontSize(9).Bold().TextColor(t.Accent).Width(30)
		} else {
			ui.Text(c, kindGlyph(it.kind)).FontSize(11).TextColor(kindColor(t, it.kind)).Width(14)
		}
		lc := t.Text
		if it.kind == "call" {
			lc = t.TextMuted
		}
		lbl := ui.Text(c, it.label).FontSize(12.5).TextColor(lc).Grow(1).MinWidth(0).SingleLine()
		if selected || it.kind == "project" || it.kind == "operation" {
			lbl = lbl.Bold()
		}
		_ = lbl
		switch {
		case it.kind == "call" && it.status > 0:
			col := t.Success
			if it.status >= 400 {
				col = t.Danger
			}
			ui.Text(c, fmt.Sprint(it.status)).FontSize(10).TextColor(col).Shrink(0)
		case it.badge != "":
			ui.Text(c, it.badge).FontSize(9.5).TextColor(t.TextMuted).Shrink(0)
		}
	})
}

// treeContextMenu is the right-click menu of a tree row: the create/rename/
// delete actions of that node, where the node is.
func (a *app) treeContextMenu(m *ui.Menu, it treeItem) {
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
			a.newProjectOpen, a.projectModalKind, a.nameInput = true, 1, name
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
			a.treeModalOpen, a.treeModalKind, a.nameInput = true, 3, it.label
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
				ui.Text(c, "POST").FontSize(8.5).Bold().TextColor(t.Accent)
				lc := t.TextMuted
				if active {
					lc = t.Text
				}
				ui.Text(c, a.callTitle(key)).FontSize(12).TextColor(lc).SingleLine().MaxWidth(180)
				ui.Text(c, "×").FontSize(12).TextColor(t.TextMuted).
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
		ui.Text(c, "POST").FontSize(10).Bold().TextColor(t.AccentText).
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
			ui.Spacer(c)
			ui.Text(c, a.contentType).FontSize(10).TextColor(t.TextMuted)
		})
		switch a.reqSection {
		case 1:
			a.headersPanel(c)
		case 2:
			ui.Column(c).Grow(1).Center().Justify(ui.Center).Children(func() {
				ui.Text(c, "SOAP 使用 HTTP Basic / WS-Security，认证配置后续接入").FontSize(11).TextColor(t.TextMuted)
			})
		case 3:
			a.settingsPanel(c)
		default:
			ui.Column(c).Grow(1).Padding(10, 12, 10).Gap(8).Children(func() {
				ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "XML 报文").FontSize(11).TextColor(t.TextMuted).Bold()
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
			ui.Text(c, "请求头").FontSize(11.5).Bold()
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
			ui.Text(c, "SOAPAction").FontSize(10.5).TextColor(t.TextMuted).Width(110)
			ui.TextInput(c, &a.action).Placeholder("http://tempuri.org/Op").Grow(1)
			ui.Text(c, "默认").FontSize(9).TextColor(t.Accent).Shrink(0)
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Content-Type").FontSize(10.5).TextColor(t.TextMuted).Width(110)
			ui.Select(c, &a.contentType, []string{
				"text/xml; charset=utf-8",
				"application/soap+xml; charset=utf-8",
			}).Grow(1)
			ui.Text(c, "默认").FontSize(9).TextColor(t.Accent).Shrink(0)
		})

		if len(a.projHeaders) > 0 {
			ui.Text(c, "项目默认头").FontSize(10.5).TextColor(t.TextMuted).Padding(4, 0, 0, 0)
			for _, h := range a.projHeaders {
				h := h
				value := h.Value
				if a.maskSecrets && isSensitiveName(h.Name) {
					value = "••••••（已遮蔽）"
				}
				ui.Row(c).Gap(8).Children(func() {
					ui.Text(c, h.Name+": "+value).FontSize(11).TextColor(t.TextMuted)
				})
			}
		}

		ui.Text(c, "样例头").FontSize(10.5).TextColor(t.TextMuted).Padding(4, 0, 0, 0)
		for i := range a.headers {
			i := i
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &a.headers[i].Name).Placeholder("Header-Name").Width(190)
				if a.maskSecrets && isSensitiveName(a.headers[i].Name) {
					ui.Text(c, "••••••（已遮蔽）").FontSize(11).TextColor(t.TextMuted).Grow(1)
				} else {
					ui.TextInput(c, &a.headers[i].Value).Placeholder("值").Grow(1)
				}
				ui.Text(c, "×").FontSize(13).TextColor(t.TextMuted).Width(18).
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
		ui.Text(c, "请求设置").FontSize(11.5).Bold()
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "超时").FontSize(11).TextColor(t.TextMuted).Width(72)
			ui.TextInput(c, &a.timeout).Placeholder("60000").Width(110)
			ui.Text(c, "ms").FontSize(11).TextColor(t.TextMuted)
			ui.Spacer(c)
			ui.Text(c, "范围 1000 – 600000").FontSize(10.5).TextColor(t.TextMuted)
		})
		ui.Divider(c)
		ui.Text(c, "请求方式").FontSize(11.5).Bold()
		ui.Text(c, "SOAP 请求固定为 POST。").FontSize(10.5).TextColor(t.TextMuted)
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
				ui.Text(c, "发送失败："+a.resp.Error).FontSize(10.5).TextColor(t.Danger).SingleLine()
			} else if a.hasResp && a.resp.Error == "" {
				col := t.Success
				if a.resp.Status >= 400 {
					col = t.Danger
				}
				a.statusPill(c, fmt.Sprintf("%d %s", a.resp.Status, a.resp.StatusText), col)
				ui.Text(c, fmt.Sprintf("%d ms", a.resp.DurationMS)).FontSize(10.5).TextColor(t.TextMuted)
				if !side {
					ui.Text(c, humanSize(a.resp.Size)).FontSize(10.5).TextColor(t.TextMuted)
				}
			} else if a.sending {
				ui.Text(c, "发送中…").FontSize(10.5).TextColor(t.TextMuted)
			} else {
				ui.Text(c, "尚未发送请求").FontSize(10.5).TextColor(t.TextMuted)
			}
		})
		ui.Row(c).Shrink(0).Padding(0, 14, 0).Gap(8).AlignItems(ui.Center).Children(func() {
			if ui.Tabs(c, &a.respTab, "格式化", "原始", "请求", "Headers").Shrink(0).Changed() {
				a.syncResponse()
			}
			ui.Spacer(c)
			if a.resp.Truncated {
				ui.Text(c, "内容已截断").FontSize(10).TextColor(t.Warning)
			}
		})
		if !a.hasResp {
			ui.Column(c).Grow(1).Center().Justify(ui.Center).Gap(6).Children(func() {
				ui.Text(c, "发送请求后，响应会显示在这里").FontSize(12).TextColor(t.TextMuted)
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
		ui.Text(c, "◈").FontSize(9).TextColor(t.Accent)
		ui.Text(c, a.info.Name).FontSize(10.5).TextColor(t.Text)
		ui.Text(c, "·").FontSize(10.5).TextColor(t.Border)
		ui.Text(c, "UTF-8").FontSize(10.5).TextColor(t.TextMuted)
		ui.Spacer(c)
		if a.errMsg != "" {
			ui.Text(c, a.errMsg).FontSize(10.5).TextColor(t.Danger).SingleLine()
		} else if a.statusMsg != "" {
			ui.Text(c, a.statusMsg).FontSize(10.5).TextColor(t.Success).SingleLine()
		}
		ui.Text(c, fmt.Sprintf("%d 个项目", len(a.projects))).FontSize(10.5).TextColor(t.TextMuted)
	})
}

// ---- dialogs ----

func (a *app) importModal(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.importOpen, func() {
		ui.Column(c).Width(520).Gap(14).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "导入").FontSize(17).Bold()
				ui.Spacer(c)
				ui.Text(c, "×").FontSize(15).TextColor(t.TextMuted).
					OnClick(func() { a.importOpen = false })
			})
			ui.Text(c, "选择一种方式把接口定义带进来。").FontSize(11).TextColor(t.TextMuted)

			if a.importKind == 3 {
				ui.Column(c).Gap(8).Children(func() {
					ui.Text(c, "WSDL URL").FontSize(10.5).TextColor(t.TextMuted)
					ui.TextInput(c, &a.importURL).Placeholder("http://127.0.0.1:8080/services/ws/api?wsdl").Grow(1)
					ui.Text(c, "代理").FontSize(10.5).TextColor(t.TextMuted)
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
			ui.Text(c, icon).FontSize(15).TextColor(t.Accent)
		})
		ui.Column(c).Gap(2).Grow(1).Children(func() {
			ui.Text(c, title).FontSize(12.5).Bold()
			ui.Text(c, desc).FontSize(10.5).TextColor(t.TextMuted).Wrap()
		})
		ui.Text(c, "›").FontSize(15).TextColor(t.TextMuted)
	})
}

func (a *app) exportModal(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.exportOpen, func() {
		ui.Column(c).Width(460).Gap(14).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "导出工作空间").FontSize(17).Bold()
				ui.Spacer(c)
				ui.Text(c, "×").FontSize(15).TextColor(t.TextMuted).
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
				ui.Text(c, "项目代理设置").FontSize(16).Bold()
				ui.Spacer(c)
				ui.Text(c, "×").FontSize(15).TextColor(t.TextMuted).
					OnClick(func() { a.proxyOpen = false })
			})
			ui.Text(c, "代理模式").FontSize(10.5).TextColor(t.TextMuted)
			ui.Select(c, &a.proxyModeSel, []string{"系统代理", "直连", "自定义代理"})
			if a.proxyModeSel == "自定义代理" {
				ui.Row(c).Gap(8).Children(func() {
					ui.Column(c).Grow(1).Gap(4).Children(func() {
						ui.Text(c, "类型").FontSize(10.5).TextColor(t.TextMuted)
						ui.Select(c, &a.proxyTypeSel, []string{"HTTP", "SOCKS5"})
					})
					ui.Column(c).Grow(1).Gap(4).Children(func() {
						ui.Text(c, "主机").FontSize(10.5).TextColor(t.TextMuted)
						ui.TextInput(c, &a.proxyHost).Placeholder("127.0.0.1")
					})
					ui.Column(c).Width(100).Gap(4).Children(func() {
						ui.Text(c, "端口").FontSize(10.5).TextColor(t.TextMuted)
						ui.TextInput(c, &a.proxyPort).Placeholder("8080")
					})
				})
				ui.Row(c).Gap(8).Children(func() {
					ui.Column(c).Grow(1).Gap(4).Children(func() {
						ui.Text(c, "用户名").FontSize(10.5).TextColor(t.TextMuted)
						ui.TextInput(c, &a.proxyUser).Placeholder("可留空")
					})
					ui.Column(c).Grow(1).Gap(4).Children(func() {
						ui.Text(c, "密码").FontSize(10.5).TextColor(t.TextMuted)
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
				ui.Text(c, "项目").FontSize(16).Bold()
				ui.Spacer(c)
				ui.Text(c, "×").FontSize(15).TextColor(t.TextMuted).
					OnClick(func() { a.newProjectOpen = false })
			})
			ui.Text(c, "项目名称").FontSize(10.5).TextColor(t.TextMuted)
			ui.TextInput(c, &a.nameInput).Placeholder("新建项目").Grow(1)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.newProjectOpen = false
				}
				if ui.PrimaryButton(c, "确定").Clicked() {
					if a.projectModalKind == 1 {
						a.renameProject(a.nameInput)
					} else {
						a.createProject(a.nameInput)
					}
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
				ui.Text(c, title).FontSize(16).Bold()
				ui.Spacer(c)
				ui.Text(c, "×").FontSize(15).TextColor(t.TextMuted).
					OnClick(func() { a.wsOpen = false })
			})
			ui.Text(c, "名称").FontSize(10.5).TextColor(t.TextMuted)
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

// treeModal handles the small name prompts for services, operations and
// request titles.
func (a *app) treeModal(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.treeModalOpen, func() {
		title, placeholder := "新建服务", "服务名称"
		switch a.treeModalKind {
		case 2:
			title, placeholder = "新建操作", "操作名称"
		case 3:
			title, placeholder = "重命名请求", "请求名称"
		}
		ui.Column(c).Width(420).Gap(12).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, title).FontSize(16).Bold()
				ui.Spacer(c)
				ui.Text(c, "×").FontSize(15).TextColor(t.TextMuted).
					OnClick(func() { a.treeModalOpen = false })
			})
			ui.Text(c, placeholder).FontSize(10.5).TextColor(t.TextMuted)
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
					case 3:
						a.renameCall(a.nameInput)
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
			ui.Text(c, "请确认").FontSize(16).Bold()
			ui.Text(c, a.confirmText).FontSize(11.5).TextColor(t.TextMuted).Wrap()
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
		ui.Icon(c, icon).FontSize(14).TextColor(col)
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
		ui.Icon(c, icon).FontSize(13).TextColor(t.TextMuted)
	})
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
		ui.Text(c, label).FontSize(11).TextColor(t.Text)
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
		ui.Icon(c, iconCopy).FontSize(13).TextColor(t.TextMuted)
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
		ui.Text(c, "●").FontSize(8).TextColor(col)
		ui.Text(c, s).FontSize(10.5).TextColor(col).Bold()
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
