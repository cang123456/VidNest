package handler

import "net/url"

// urlEncode 用 RFC 3986 URL 编码字符串，用于下载时的文件名（避免中文变乱码）。
func urlEncode(s string) string {
	return url.PathEscape(s)
}
