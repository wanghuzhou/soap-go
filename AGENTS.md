# 项目规则

## SoapUI XML 导出

- 导出 SoapUI XML 时，只写 SoapUI 自身支持、能识别的字段。
- 本应用新增而 SoapUI 不认识的字段（例如请求的 `auth` 认证配置、界面/主题偏好等）一律不写入导出文件，保证产物能被 SoapUI 正常导入。
- 相关代码在 `internal/soapuiproj/export.go`；给数据模型新增字段时，需确认是否属于 SoapUI 字段，否则不要导出。

> 工作空间 JSON 导出不受此限制，仍按本应用完整格式保存。

## 普通 HTTP 项目禁止导出 SoapUI XML

- 新建的项目（`Project.Source == "native"`，即普通 HTTP 项目）不提供「导出 SoapUI XML」功能，菜单项隐藏，调用的导出入口在代码层直接拒绝并提示。
- 只有来自 SoapUI / WSDL 导入的项目（`source` 为 `soapui` / `wsdl` / `wsdl-url`）才允许导出 SoapUI XML。
- 判断依据统一走模型的来源判定函数，不要在 UI 和导出逻辑里各写一份。

## 新增按钮默认无边框

- 树/面板里的「新增 / 添加」类操作按钮（如「＋ 添加请求头」「＋ 添加参数」「＋ 添加字段」）默认使用无边框样式：纯文字 + 悬停底色，不要用带描边的普通按钮。
- 统一用 `view.go` 的 `plainButton`（或等价的无边框按钮），保证各处新增按钮外观一致。
