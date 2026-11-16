// Command soap-go is a SOAP / WSDL debugging desktop app written in Go and
// drawn by MyGo's native UI toolkit (no web page). It reuses soap-web's
// business packages (model, store, workspace, wsdl, transport) and presents
// them through a native workbench.
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
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

	// Keep WebView2's profile under the data directory, as the Wails build
	// does: a stale profile in the default location makes controller creation
	// fail with 8000ffff.
	webviewData := filepath.Join(dir, "webview2")
	_ = os.MkdirAll(webviewData, 0o755)
	_ = os.Setenv("WEBVIEW2_USER_DATA_FOLDER", webviewData)

	a := newApp(reg)
	// Persist the theme and split layout when the app quits, as a backstop for
	// changes the running view has not written yet.
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.saveUIState() })
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:           "soap-go",
			Width:           1440,
			Height:          920,
			MinWidth:        1040,
			MinHeight:       680,
			BackgroundColor: "#0b0e14",
			TitleBarStyle:  mygo.TitleBarHidden,
			TitleBarHeight: 44,
			Content:        ui.View(a.view),
		})
		a.applyWindowButtons()
	})
	err = mygo.App.Run()
	a.saveUIState()
	if err != nil {
		log.Printf("soap-go 退出：%v", err)
		os.Exit(1)
	}
}
