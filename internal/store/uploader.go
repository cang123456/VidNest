package store

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"video-nas/internal/models"
)

// Uploader 负责把用户从网页上传的文件落盘 + 入库。
type Uploader struct {
	DB        *sql.DB
	VideoRoot string
	Thumb     *Thumbnailer
}

func NewUploader(db *sql.DB, videoRoot string, thumb *Thumbnailer) *Uploader {
	return &Uploader{DB: db, VideoRoot: videoRoot, Thumb: thumb}
}

// Save 把上传的数据流保存到 VideoRoot/uploads/xxx.ext，然后入库。
// 返回新插入视频的 ID。
// src：上传的数据流；origName：用户端的原始文件名。
func (u *Uploader) Save(src io.Reader, origName string) (int64, error) {
	// 1. 确保 uploads 目录存在
	uploadDir := filepath.Join(u.VideoRoot, "uploads")
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return 0, fmt.Errorf("创建 uploads 目录失败: %w", err)
	}

	// 2. 整理一下文件名：去掉可能的路径分隔符，避免 ../ 穿越；避免同名覆盖加时间戳
	base := filepath.Base(origName)
	ext := strings.ToLower(filepath.Ext(base))
	nameNoExt := strings.TrimSuffix(base, ext)
	if !videoExts[ext] {
		return 0, fmt.Errorf("不支持的文件类型：%s（只支持常见视频后缀）", ext)
	}

	stamp := time.Now().Format("20060102_150405")
	safeName := sanitizeName(nameNoExt) + "_" + stamp + ext
	savePath := filepath.Join(uploadDir, safeName)

	// 3. 先写到临时文件，写完再改名；防止上传到一半程序崩了留下半截文件
	tmpPath := savePath + ".part"
	dst, err := os.Create(tmpPath)
	if err != nil {
		return 0, fmt.Errorf("创建目标文件失败: %w", err)
	}
	size, err := io.Copy(dst, src)
	dst.Close()
	if err != nil {
		os.Remove(tmpPath)
		return 0, fmt.Errorf("写入文件失败: %w", err)
	}
	if err := os.Rename(tmpPath, savePath); err != nil {
		os.Remove(tmpPath)
		return 0, fmt.Errorf("重命名文件失败: %w", err)
	}

	// 4. folder_path 固定是 /uploads
	folderPath := "/uploads"

	// 5. 取元数据 + 截封面
	meta := &VideoMeta{}
	if u.Thumb != nil {
		if m, err := u.Thumb.Probe(savePath); err == nil {
			meta = m
		}
	}
	thumbName := ""
	if u.Thumb != nil {
		if n, err := u.Thumb.GenerateThumbnail(savePath); err == nil {
			thumbName = n
		}
	}

	// 6. 入库
	now := time.Now()
	v := models.Video{
		FolderPath: folderPath,
		FileName:   safeName,
		FilePath:   savePath,
		FileSize:   size,
		Duration:   meta.Duration,
		Resolution: meta.Resolution,
		Thumbnail:  thumbName,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	res, err := u.DB.Exec(`
		INSERT INTO videos(folder_path, file_name, file_path, file_size, duration, resolution, thumbnail, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?,?)
	`, v.FolderPath, v.FileName, v.FilePath, v.FileSize, v.Duration, v.Resolution, v.Thumbnail, v.CreatedAt, v.UpdatedAt)
	if err != nil {
		// 入库失败把刚才存的文件也清掉，避免垃圾文件
		os.Remove(savePath)
		return 0, fmt.Errorf("写入数据库失败: %w", err)
	}
	return res.LastInsertId()
}

// sanitizeName 去掉文件名里 Windows/Linux 都不允许的字符，防止路径穿越或创建失败。
func sanitizeName(name string) string {
	replacer := strings.NewReplacer(
		"/", "_", "\\", "_",
		":", "_", "*", "_",
		"?", "_", "\"", "_",
		"<", "_", ">", "_",
		"|", "_", " ", "_",
	)
	out := replacer.Replace(name)
	if out == "" {
		out = "video"
	}
	if len(out) > 100 {
		out = out[:100]
	}
	return out
}
