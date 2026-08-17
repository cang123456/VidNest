// Package router 注册所有 URL 路由。
// 把路由从 main.go 抽出来，main.go 只做初始化和启动，更清晰。
package router

import (
	"github.com/gin-gonic/gin"

	"video-nas/internal/handler"
)

// Setup 绑定所有 URL 到 handler 的方法。
// r 是外面已经 New 好的 gin Engine；h 是已经准备好依赖的 Handler 聚合实例。
func Setup(r *gin.Engine, h *handler.Handler) {
	// 静态资源：/static/xxx 指向项目下的 static/ 目录
	r.Static("/static", "./static")

	// --- 页面渲染路由 ---
	r.GET("/", h.RenderHome)
	r.GET("/folder/*path", h.RenderFolder) // *path 匹配 /folder/电影/动作片 这种多级路径
	r.GET("/play/:id", h.RenderPlayer)
	r.GET("/upload", h.RenderUpload)
	r.GET("/scan", h.RenderHome) // 兼容历史链接，直接回首页

	// --- 封面图：单独一个路由，避免和 HTML 路由冲突 ---
	r.GET("/thumb/:name", h.ServeThumbnail)

	// --- /api/ 开头的接口，全部返回 JSON 或数据流 ---
	api := r.Group("/api")
	{
		// 扫描相关
		api.POST("/scan", h.APIScan)
		api.GET("/scan/status", h.APIScanStatus)

		// 上传
		api.POST("/upload", h.APIUpload)

		// 视频：播放流 + 下载 + 删除
		api.GET("/stream/:id", h.APIStream)
		api.GET("/download/:id", h.APIDownload)
		api.POST("/delete/:id", h.APIDelete)
	}
}
