// Package config 集中管理所有配置项。
// 想要改端口、视频目录、ffmpeg 路径等，只需要修改 Default 函数里的值。
package config

import "path/filepath"

// Config 保存整个应用的运行参数。
type Config struct {
	Port        int    // HTTP 服务监听端口
	VideoRoot   string // 视频文件的根目录（扫描会递归遍历此目录，上传也存在这里）
	ThumbDir    string // 视频封面缩略图的保存目录
	DBPath      string // SQLite 数据库文件路径
	FFmpegPath  string // ffmpeg 可执行文件路径；写 "ffmpeg" 表示从系统 PATH 找，也可写绝对路径如 C:/ffmpeg/bin/ffmpeg.exe
	FFprobePath string // ffprobe 可执行文件路径（和 ffmpeg 在同一个目录下）
	ScanOnStart bool   // 程序启动时是否自动扫描 VideoRoot
}

// Default 返回默认配置。部署时按实际情况修改即可。
func Default() *Config {
	// 统一用相对路径 ./data 作为运行时数据目录
	// 这样程序在哪里启动，data 就在哪里，方便备份和迁移。
	dataDir := "./data"

	return &Config{
		Port: 8080,
		//VideoRoot:   filepath.Join(dataDir, "library"),
		VideoRoot:   filepath.Join("E:\\", "B站视频"),
		ThumbDir:    filepath.Join(dataDir, "thumbnails"),
		DBPath:      filepath.Join(dataDir, "videos.db"),
		FFmpegPath:  "ffmpeg",
		FFprobePath: "ffprobe",
		ScanOnStart: true,
	}
}
