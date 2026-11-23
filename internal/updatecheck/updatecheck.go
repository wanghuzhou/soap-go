// Package updatecheck checks whether a newer release of soap-go exists.
//
// This is the "check only" half of an updater: it reads a release description
// (a self-hosted latest.json, or GitHub's releases API) and compares the release
// version with the running one. It never downloads or replaces files, so it is
// safe to call from a background goroutine.
package updatecheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"soapgo/internal/fileutil"
	"soapgo/internal/model"
	"soapgo/internal/transport"
)

const (
	// DefaultRepo is the GitHub repository used when no custom source is set.
	DefaultRepo = "wanghuzhou/soap-go"
	// MaxBytes caps the update description response.
	MaxBytes = 1 << 20 // 1 MB
	// Timeout bounds a whole check, including redirects.
	Timeout = 15 * time.Second
	// UserAgent identifies the request; GitHub rejects one without a UA.
	UserAgent = "soap-go (update check)"
)

// Asset is one downloadable file of a release. It is unused by the check-only
// mode but kept so a self-hosted manifest with assets still parses.
type Asset struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256,omitempty"`
}

// Manifest is the update description this package understands. A self-hosted
// latest.json is decoded straight into it; a GitHub release is mapped onto it.
type Manifest struct {
	Version    string  `json:"version"`
	Notes      string  `json:"notes"`
	Page       string  `json:"page"`
	PubDate    string  `json:"pubDate,omitempty"`
	MinVersion string  `json:"minVersion,omitempty"`
	Assets     []Asset `json:"assets,omitempty"`
}

// Result is the outcome of one check.
type Result struct {
	Current   string
	Latest    string
	HasUpdate bool
	Notes     string
	Page      string
}

// githubRelease is the subset of GitHub's release JSON this package reads.
type githubRelease struct {
	TagName     string `json:"tag_name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
}

// FetchManifest downloads and parses a self-hosted latest.json.
func FetchManifest(ctx context.Context, rawURL string, proxyCfg *model.ProxySettings) (*Manifest, error) {
	var m Manifest
	if err := fetchJSON(ctx, rawURL, proxyCfg, "application/json", &m); err != nil {
		return nil, err
	}
	if strings.TrimSpace(m.Version) == "" {
		return nil, errors.New("更新信息里没有版本号")
	}
	return &m, nil
}

// FetchGitHub reads the latest release of repo through GitHub's API. An empty
// repo falls back to DefaultRepo.
func FetchGitHub(ctx context.Context, repo string, proxyCfg *model.ProxySettings) (*Manifest, error) {
	repo = strings.Trim(strings.TrimSpace(repo), "/")
	if repo == "" {
		repo = DefaultRepo
	}
	api := "https://api.github.com/repos/" + repo + "/releases/latest"
	var rel githubRelease
	if err := fetchJSON(ctx, api, proxyCfg, "application/vnd.github+json", &rel); err != nil {
		return nil, err
	}
	if strings.TrimSpace(rel.TagName) == "" {
		return nil, errors.New("更新信息里没有版本号")
	}
	return &Manifest{
		Version: rel.TagName,
		Notes:   rel.Body,
		Page:    rel.HTMLURL,
		PubDate: rel.PublishedAt,
	}, nil
}

// fetchJSON GETs rawURL and decodes the JSON body into dst.
func fetchJSON(ctx context.Context, rawURL string, proxyCfg *model.ProxySettings, accept string, dst any) error {
	u, err := parseURL(rawURL)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("构造请求失败：%w", err)
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", accept)
	if accept == "application/vnd.github+json" {
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}

	client, err := transport.NewClient(proxyCfg, Timeout)
	if err != nil {
		return fmt.Errorf("代理配置错误：%w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败：%w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return errors.New("未找到更新信息（404）")
	case http.StatusForbidden:
		return fmt.Errorf("更新源拒绝访问（%d），可能触发了 GitHub 频率限制", resp.StatusCode)
	default:
		return fmt.Errorf("更新源返回 %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	if resp.ContentLength > MaxBytes {
		return fmt.Errorf("更新信息超过 %d KB 上限", MaxBytes>>10)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBytes+1))
	if err != nil {
		return fmt.Errorf("读取更新信息失败：%w", err)
	}
	if len(body) > MaxBytes {
		return fmt.Errorf("更新信息超过 %d KB 上限", MaxBytes>>10)
	}
	if err := json.Unmarshal(bytes.TrimSpace(fileutil.StripBOM(body)), dst); err != nil {
		return fmt.Errorf("解析更新信息失败：%w", err)
	}
	return nil
}

// parseURL accepts only an absolute http/https URL.
func parseURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("更新源地址为空")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("更新源地址无效：%w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, fmt.Errorf("更新源仅支持 http/https，当前为 %q", u.Scheme)
	}
	if u.Host == "" || u.Hostname() == "" {
		return nil, errors.New("更新源地址缺少主机名")
	}
	u.Scheme = scheme
	return u, nil
}

// Evaluate compares the running version with the manifest's version.
func Evaluate(current string, m *Manifest) Result {
	latest := strings.TrimSpace(m.Version)
	r := Result{
		Current: current,
		Latest:  latest,
		Notes:   strings.TrimSpace(m.Notes),
		Page:    strings.TrimSpace(m.Page),
	}
	if latest == "" {
		return r
	}
	r.HasUpdate = Compare(current, latest) < 0
	return r
}

// Compare orders two version strings: -1 when a < b, 0 when equal, 1 when a > b.
// A leading "v" and a "+build" suffix are ignored; a pre-release ("-rc1") sorts
// before the matching release.
func Compare(a, b string) int {
	va, vb := parseVersion(a), parseVersion(b)
	for i := 0; i < 3; i++ {
		if va.nums[i] != vb.nums[i] {
			if va.nums[i] < vb.nums[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case va.pre == "" && vb.pre != "":
		return 1 // release > pre-release
	case va.pre != "" && vb.pre == "":
		return -1
	case va.pre != vb.pre:
		if va.pre < vb.pre {
			return -1
		}
		return 1
	}
	return 0
}

// version is a parsed version: three numeric parts plus an optional pre-release.
type version struct {
	nums [3]int
	pre  string
}

// parseVersion reads the numeric core and pre-release of a version string.
func parseVersion(s string) version {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	pre := ""
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre = s[i+1:]
		s = s[:i]
	}
	return version{nums: parseCore(s), pre: pre}
}

// parseCore parses up to three dot-separated integers; non-numeric parts are 0.
func parseCore(core string) [3]int {
	var out [3]int
	parts := strings.Split(core, ".")
	for i := 0; i < len(parts) && i < 3; i++ {
		n, _ := strconv.Atoi(strings.TrimSpace(parts[i]))
		out[i] = n
	}
	return out
}
