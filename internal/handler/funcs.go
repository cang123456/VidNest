package handler

import (
	"html/template"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// 这段代码专门做一件事：给 Gin 模板注册一些自定义函数。
// Gin 默认模板函数很少，我们常用的「字符串切分」「数字加减」「时间格式化」都得自己加。
// 调用时机是在 r.LoadHTMLGlob 之前，用 r.SetFuncMap。

// FuncMap 返回所有需要注入模板的函数。
// 关键：Gin 必须在 LoadHTMLGlob 之前 SetFuncMap，顺序不能反。
func FuncMap() template.FuncMap {
	return template.FuncMap{
		// ---- 简单的四则运算，给分页、加减用 ----
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"mul": func(a, b int) int { return a * b },
		"div": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a / b
		},

		// ---- 时间格式化 ----
		"formatTime": func(t time.Time) string {
			if t.IsZero() {
				return "-"
			}
			return t.Format("2006-01-02 15:04")
		},

		// ---- 文件夹面包屑：把 /电影/动作片 拆成 [/电影, /电影/动作片] ----
		"splitFolder": splitFolderForTemplate,

		// ---- 面包屑第 i 层的累积路径 ----
		"accumulate": accumulateForTemplate,

		// ---- 给完整 folder_path，取最后一节显示名 ----
		"folderName": func(p string) string {
			if p == "/" || p == "" {
				return "根目录"
			}
			return filepath.Base(filepath.ToSlash(p))
		},

		// ---- 根据后缀猜 Content-Type（供 <source type=...>） ----
		"guessMime": guessVideoMime,

		// ---- 分页：生成「1 2 ... 8 9 10 ... 20」这样的序列 ----
		"pageRange": pageRangeForTemplate,

		// ---- 分页：拼回当前 URL（保留 q、sort，去掉 page） ----
		"pagerBaseURL": pagerBaseURLForTemplate,

		// ---- 避免 nil interface 渲染时报错 ----
		"or": func(a, b interface{}) interface{} {
			switch v := a.(type) {
			case string:
				if v == "" {
					return b
				}
			case nil:
				return b
			}
			return a
		},
	}
}

// breadcrumbPart 只是为了模板里拿 Name + Path。
// 因为 Go 模板对 map key 的类型要求严格，用结构体更稳。
type breadcrumbPart struct {
	Name string
	Path string
}

func splitFolderForTemplate(p string) []breadcrumbPart {
	// p = "/电影/动作片" → 先去首位空段，再逐段累加
	cleaned := filepath.ToSlash(strings.TrimPrefix(p, "/"))
	cleaned = strings.Trim(cleaned, "/")
	if cleaned == "" {
		return []breadcrumbPart{{Name: "根目录", Path: "/"}}
	}
	parts := strings.Split(cleaned, "/")
	out := make([]breadcrumbPart, 0, len(parts))
	prefix := ""
	for _, s := range parts {
		if s == "" {
			continue
		}
		prefix += "/" + s
		out = append(out, breadcrumbPart{Name: s, Path: prefix})
	}
	return out
}

// accumulateForTemplate 取出 splitFolder 返回切片里 [0:i] 的累积路径
// 比如 parts = [{Name:电影,Path:/电影}, {Name:动作,Path:/电影/动作}]
// i=0 → /电影；i=1 → /电影/动作
func accumulateForTemplate(parts []breadcrumbPart, i int) string {
	if i < 0 || i >= len(parts) {
		return ""
	}
	return parts[i].Path
}

func guessVideoMime(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".ogg", ".ogv":
		return "video/ogg"
	case ".mov":
		return "video/quicktime"
	case ".avi":
		return "video/x-msvideo"
	case ".wmv":
		return "video/x-ms-wmv"
	case ".flv":
		return "video/x-flv"
	case ".mkv":
		// 大多数现代浏览器都能通过 ffmpeg 解码，但 <source type> 没有标准 MIME
		// 给一个通用的，让浏览器自己去试
		return "video/x-matroska"
	}
	return "video/mp4"
}

// pageRangeForTemplate 生成「1 ... 5 6 7 ... 30」这样的分页数字序列
// 规则：始终显示 1、last；当前页前后各显示 2 个；用 0 代表 ...
func pageRangeForTemplate(cur, total int) []int {
	if total <= 1 {
		if total == 0 {
			return nil
		}
		return []int{1}
	}
	pages := map[int]bool{}
	add := func(n int) {
		if n >= 1 && n <= total {
			pages[n] = true
		}
	}
	add(1)
	add(total)
	for i := -2; i <= 2; i++ {
		add(cur + i)
	}
	// 排序
	sorted := make([]int, 0, len(pages))
	for n := 1; n <= total; n++ {
		if pages[n] {
			sorted = append(sorted, n)
		}
	}
	// 插入 0 作为省略号占位（和 last 连续就不插）
	out := make([]int, 0, len(sorted)+2)
	for i, v := range sorted {
		if i > 0 && v-sorted[i-1] > 1 {
			out = append(out, 0)
		}
		out = append(out, v)
	}
	return out
}

// pagerBaseURLForTemplate 把当前页的 q 和 sort 拼回 URL，让分页链接不丢参数
// 注：这个函数需要拿到 Gin 的 context 才能拿 Query；但模板函数没有 context 参数。
// 解决思路：我们直接在 PageData 里找不到这些，所以用一个全局存储（借助 context 链做不到）。
// 替代方案：在渲染每个页面的时候，让 handler 把 URL 已经准备好塞进 PageData。
// 这里为了避免在 handler 里每个地方都拼，我们在模板里用 request URL。
// 但由于 Go template 没有 request，我们用另一个办法——由 handler 在 PageData 上提供。
// 所以这里需要我们在 PageData 里补充一个 PagerBaseURL 字段。
//
// 但是我们已经发布了 PageData 没有该字段，需要一个兼容方案：
// 读当前模板的上下文拿不到 Request，所以我们用 "全局 hack"——用一个 gin 中间件把
// 当前 request 存在一个 sync.Map 里，这里再取出来。对菜鸟来说太复杂，
// 所以更简单的方式：把 pagerBaseURL 函数签名改为直接接收 *PageData，
// PageData 结构体加上一个 RequestURL 字段，在渲染之前由 handler 赋值。

func pagerBaseURLForTemplate(pd interface{}) string {
	switch x := pd.(type) {
	case interface{ GetPagerBase() string }:
		return x.GetPagerBase()
	case PageData:
		return x.PagerBaseURL
	case *PageData:
		return x.PagerBaseURL
	}
	return "?"
}

// ---- 下面是「给每个请求把 pagerBase 注入 PageData」的实现 ----
// 为了不改动 PageData 公共字段太多，我们在 PageData 里加一个非导出字段 pagerBase，
// 然后提供一个 SetPagerBase 方法。不过非导出字段在包外（模板是字符串匹配）不影响，
// 但结构体字面量里需要赋值，所以我们改为导出字段 PagerBaseURL，统一。

// 说明：上面 pagerBaseURLForTemplate 实际需要访问到 page 级别保存的 URL，
// 我们在 RenderHome / RenderFolder 里调用 SetPagerBase 之前会先构造它。
// 所以这里真正有用的实现，放在下面 BuildPagerBaseURL 里。

// BuildPagerBaseURL 从 Gin 的 Query 里保留 q + sort，拼成 ?q=xxx&sort=yyy 形式
// （最后一个 & 没关系，我们写成 &page=N，浏览器都能解析）
func BuildPagerBaseURL(c *gin.Context) string {
	q := c.Query("q")
	sort := c.Query("sort")
	vals := url.Values{}
	if q != "" {
		vals.Set("q", q)
	}
	if sort != "" {
		vals.Set("sort", sort)
	}
	// 无论有没有值都加个 ?，后面模板拼 &page= 就不会漏 ?
	return c.Request.URL.Path + "?" + vals.Encode()
}

// ---- 给 PageData 注入分页基础 URL（handler 里调用）----
func (p *PageData) SetPagerBase(u string) {
	p.PagerBaseURL = trimEnd(u)
}

// trimEnd 去掉末尾多余的 & 或 ?，让拼接出来的 URL 好看点
func trimEnd(u string) string {
	u = strings.TrimRight(u, "&")
	if strings.HasSuffix(u, "?") {
		// "path?" 的话删掉 ?，后面模板直接加 &page= 会变成 path?&page= 也能用，但不美观
		// 这里保留 ?& 也不影响，所以简单处理
	}
	// 加个结尾锚点：当没有任何参数时 "path?" + "&page=2" = "path?&page=2"，浏览器正常解析
	return u
}

// GetPagerBase 让模板能通过 interface{} 拿到（兜底）
func (p *PageData) GetPagerBase() string {
	if p == nil {
		return "?"
	}
	return p.PagerBaseURL
}

// ---- 额外：字符串转 int 的安全函数（和 handler.go 里 parseQueryInt 保持一致，调试用）----
func atoiSafe(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
