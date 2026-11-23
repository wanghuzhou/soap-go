// Command soap-go is a SOAP / WSDL debugging desktop app written in Go and
// drawn by MyGo's native UI toolkit (no web page). It reuses soap-web's
// business packages (model, store, workspace, wsdl, transport) and presents
// them through a native workbench.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/egoist/mygo"
	// Register MyGo's own strings, such as the text fields' edit menu, in
	// Chinese, as the rest of this app's UI is written in Chinese.
	_ "github.com/egoist/mygo/locales/zh-Hans"
	"github.com/egoist/mygo/ui"

	"soapgo/internal/applog"
	"soapgo/internal/appsetup"
)

func main() {
	dataPath := flag.String("data", "", "数据目录（默认：可执行文件旁 data）")
	flag.Parse()

	dir, legacy := appsetup.ResolveDataDir(*dataPath)
	if path, err := applog.Init(appsetup.LogDir(dir)); err != nil {
		log.Printf("日志文件不可用，仅输出到控制台：%v", err)
	} else {
		log.Printf("日志文件：%s", path)
	}
	defer applog.Close()

	reg, err := appsetup.OpenRegistry(dir, legacy, *dataPath == "")
	if err != nil {
		log.Printf("加载数据目录失败 %s：%v", dir, err)
		os.Exit(1)
	}
	log.Printf("数据目录：%s", reg.DataDir())
	log.Printf("当前工作空间：%s", reg.CurrentInfo().Name)

	a := newApp(reg)
	// Persist the theme and split layout when the app quits, as a backstop for
	// changes the running view has not written yet. When a request has unsaved
	// edits, ask first instead of losing them.
	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
		if !a.quitting && a.hasUnsavedTabs() {
			e.PreventDefault()
			a.askQuit()
			if a.win != nil {
				a.win.Invalidate()
			}
			return
		}
		a.saveUIState()
	})
	mygo.App.OnQuitCanceled(func() { a.quitting = false })
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:                "soap-go",
			StateKey:             "main",
			Width:                1440,
			Height:               920,
			MinWidth:             1040,
			MinHeight:            680,
			BackgroundColor:      "#0b0e14",
			TitleBarStyle:        mygo.TitleBarHidden,
			TitleBarHeight:       topBarHeight,
			TrafficLightPosition: trafficLightPosition(),
			Content:              ui.View(a.view),
		})
		// Closing the window is the other way out; guard it the same way.
		a.win.OnClose(func(e *mygo.CloseEvent) {
			if !a.quitting && a.hasUnsavedTabs() {
				e.PreventDefault()
				a.askQuit()
				a.win.Invalidate()
			}
		})

		// The executable carries no icon resource, so set the window's icon
		// from the embedded PNG; without this the title bar and taskbar show a
		// blank default.
		if err := a.win.SetIcon(appIconPNG); err != nil {
			log.Printf("设置窗口图标失败：%v", err)
		}
		a.applyWindowButtons()
		// Check for a newer release once the window is up, without blocking it.
		a.maybeAutoCheckUpdates()
	})
	err = mygo.App.Run()
	a.saveUIState()
	if err != nil {
		log.Printf("soap-go 退出：%v", err)
		os.Exit(1)
	}
}
