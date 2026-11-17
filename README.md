# soap-go

SOAP / WSDL 接口调试桌面工具，用 **Go + MyGo 原生 UI**（纯 Go 绘制，无 WebView）

![主界面](docs/screenshots/example1.png)

## 功能

- **项目树**：项目 → 服务 → 操作 → 请求样例，展开折叠、搜索过滤、显示上次响应状态码
- **请求编辑**：Endpoint、SOAPAction、Content-Type、超时、请求头（项目默认 + 样例覆盖 + 敏感头遮蔽）、XML 代码编辑器（行号/语法高亮）
- **发送请求**：真实 SOAP POST，支持代理（系统 / 直连 / 自定义 HTTP·SOCKS5）、超时、响应截断
- **响应查看**：格式化 / 原始 / 请求 / Headers 四个视图，状态码、耗时、大小；悬停显示复制图标，发送成功后自动保存最近响应
- **导入**：SoapUI 项目（.xml）、WSDL 文件、WSDL URL（可选代理）
- **导出**：工作空间 JSON（可选保留代理）、单个项目导出为 SoapUI XML
- **管理**：工作空间切换/新建/重命名/删除/导入/导出；项目、服务、操作、请求的增删改（树节点右键菜单）
- **其它**：上下/左右分屏、深色/浅色主题、界面与编辑器字体设置（字体和字号分别可调）、主题与分栏布局持久化、请求可取消


## 数据兼容

数据格式与 soap-web **完全一致**（`settings.json`、`projects.json`）。

把 soap-web 的 `bin/data` 下的 `settings.json` 与 `projects.json` 复制到 soap-go 的数据目录，即可看到原有项目。主题、分栏布局和字体设置保存在数据目录的 `ui-state.json`。

数据目录默认为可执行文件旁 `data/`，也可用 `-data <目录>` 指定。

## 运行

### 预编译版

```powershell
bin\soap-go.exe
```

首次运行会自动创建数据目录与默认工作空间。

### 从源码构建

```powershell
go build -trimpath -ldflags "-H windowsgui" -o bin/soap-go.exe .
```

`-H windowsgui` 让 Windows 不额外弹出控制台窗口；日志写入数据目录旁的 `logs/soap.log`。

需要 **Go 1.27.3+**（MyGo 的版本要求）。

### 测试

```powershell
go vet ./...
go test ./...
```

## 快捷键

| 快捷键 | 作用 |
|---|---|
| `Ctrl+Enter` | 发送请求 |
| `Ctrl+S` | 保存请求样例 |
| `Ctrl+Shift+F` | 格式化 XML |
| `Ctrl+K` | 聚焦项目树搜索 |

## 目录结构

```
main.go        入口：数据目录、日志、窗口、退出时保存布局
app.go         应用状态与操作（加载/保存/发送/导入/CRUD/工作空间/持久化）
view.go        原生 UI：工作台、请求 Tab、分区、弹窗、右键菜单
theme.go       深色 / 浅色主题
internal/      从 soap-web 移植的业务包
  model        数据模型
  store        项目持久化与 CRUD
  workspace    多工作空间注册表
  wsdl         WSDL 解析
  wsdlfetch    远程 WSDL 抓取
  soapuiproj   SoapUI 项目导入 / 导出
  transport    HTTP/SOAP 发送（支持 context 取消、代理、字符集）
  charset      字符集解码
  fileutil     原子写文件
  applog       日志
  appsetup     数据目录与注册表初始化
```

## 发布

推送 `v*.*.*` 形式的 tag 会触发 `.github/workflows/release.yml`：

- 先跑 `go vet` / `go test`，通过后构建
- 产物：`windows/amd64`、`linux/amd64`、`darwin/arm64`、`darwin/amd64`
- 自动创建 GitHub Release，附 `SHA256SUMS.txt`
## 说明

- 界面由 MyGo 的原生 UI 工具包绘制，不依赖 WebView
- 项目树节点的「新建服务 / 新建操作 / 新建请求 / 重命名 / 复制 / 删除」在**右键菜单**里。
- 已知差异：原 web 版的树键盘导航未实现；提示采用底部状态栏而非浮层 Toast。
