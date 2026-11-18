# 项目规则

## SoapUI XML 导出

- 导出 SoapUI XML 时，只写 SoapUI 自身支持、能识别的字段。
- 本应用新增而 SoapUI 不认识的字段（例如请求的 `auth` 认证配置、界面/主题偏好等）一律不写入导出文件，保证产物能被 SoapUI 正常导入。
- 相关代码在 `internal/soapuiproj/export.go`；给数据模型新增字段时，需确认是否属于 SoapUI 字段，否则不要导出。

> 工作空间 JSON 导出不受此限制，仍按本应用完整格式保存。
