package main

import _ "embed"

// appIconPNG is the application icon, shown in the window's title bar and the
// taskbar. It is generated from the same gradient "S" mark the top bar draws.
//
//go:embed resources/icon.png
var appIconPNG []byte
