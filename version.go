package main

import (
	_ "embed"
	"encoding/json"
	"strings"
)

// mygoMeta is mygo.json, embedded so the running app reports the same version
// that MyGo uses to package the binary. The macOS .app/.dmg take their version
// from mygo.json, and `go tool mygo build` has no way to pass -ldflags, so
// without this the macOS build would fall back to a hardcoded source version and
// keep telling users to "update" to the version they already run.
//
//go:embed mygo.json
var mygoMeta []byte

// appVersion is the version shown in the About panel and compared by the update
// check. It is empty by default so a release build can set it from the git tag
// with `-ldflags "-X main.appVersion=${TAG}"`; when no tag is injected (notably
// the macOS `go tool mygo build`), init fills it from mygo.json.
var appVersion = ""

func init() {
	if appVersion == "" {
		appVersion = versionFromMygo()
	}
}

// versionFromMygo reads the version out of the embedded mygo.json, normalising
// it to a leading "v" so it matches tag-style versions.
func versionFromMygo() string {
	var meta struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(mygoMeta, &meta); err == nil {
		if v := strings.TrimSpace(meta.Version); v != "" {
			if !strings.HasPrefix(v, "v") && !strings.HasPrefix(v, "V") {
				v = "v" + v
			}
			return v
		}
	}
	return "v0.0.0"
}
