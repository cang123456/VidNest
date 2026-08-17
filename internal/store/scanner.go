package store

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"video-nas/internal/models"
)

// 常见的视频文件扩展名，扫描时只看这些。
var videoExts = map[string]bool{
	".mp4": true, ".mkv": true, ".avi": true,
	".mov": true, ".wmv": true, ".flv": true,
	".webm": true, ".m4v": true, ".rmvb": true, ".rm": true,
}

// Scanner 负责扫描本地视频文件夹并写入数据库。
type Scanner struct {
	DB        *sql.DB
	VideoRoot string
	Thumb     *Thumbnailer

	// 扫描过程统计（并发安全）
	mu      sync.Mutex
	added   int
	skipped int
	failed  int
	running bool // 是否正在扫描，避免重复点击导致多个扫描同时跑
}

// NewScanner 构造一个 Scanner。
func NewScanner(db *sql.DB, videoRoot string, thumb *Thumbnailer) *Scanner {
	return &Scanner{DB: db, VideoRoot: videoRoot, Thumb: thumb}
}

// Result 扫描完成后返回的统计信息。
type Result struct {
	Added   int
	Skipped int
	Failed  int
}

// IsRunning 判断是否正在扫描（前端用来禁用按钮或显示进度）。
func (s *Scanner) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Run 启动一次完整扫描。已经在扫描则直接返回 nil 且不重复执行。
func (s *Scanner) Run() (*Result, error) {
	// 防止并发跑多个扫描
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil, nil
	}
	s.running = true
	s.added = 0
	s.skipped = 0
	s.failed = 0
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	// 确保根目录存在
	if err := os.MkdirAll(s.VideoRoot, 0o755); err != nil {
		return nil, err
	}

	// 递归遍历，遇到视频文件就处理
	err := filepath.Walk(s.VideoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 某个子目录出错就跳过，不要整个扫描崩
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(info.Name()))
		if !videoExts[ext] {
			return nil
		}
		s.processOne(path)
		return nil
	})

	s.mu.Lock()
	res := &Result{Added: s.added, Skipped: s.skipped, Failed: s.failed}
	s.mu.Unlock()
	return res, err
}

// processOne 处理单个视频文件：查库去重 → 取元数据 → 截封面 → 入库。
func (s *Scanner) processOne(fullPath string) {
	// 1. 先看数据库里有没有（用完整路径做唯一键去重）
	var existsID int64
	err := s.DB.QueryRow(`SELECT id FROM videos WHERE file_path = ?`, fullPath).Scan(&existsID)
	if err == nil {
		// 已存在，检查文件大小有没有变；变了可能是文件被替换了，更新一下
		s.mu.Lock()
		s.skipped++
		s.mu.Unlock()
		return
	}
	if err != sql.ErrNoRows {
		log.Println("[scanner] 查询数据库失败:", fullPath, err)
		s.mu.Lock()
		s.failed++
		s.mu.Unlock()
		return
	}

	// 2. 读文件大小
	info, err := os.Stat(fullPath)
	if err != nil {
		log.Println("[scanner] 读取文件失败:", fullPath, err)
		s.mu.Lock()
		s.failed++
		s.mu.Unlock()
		return
	}

	// 3. 计算 folder_path（相对 VideoRoot 的目录，前面统一加 /，根目录就是 /）
	relDir := filepath.Dir(fullPath)
	relDir, err = filepath.Rel(s.VideoRoot, relDir)
	if err != nil || relDir == "." {
		relDir = ""
	}
	// 统一用 / 做分隔符，Windows 的 \ 也转成 /
	folderPath := "/" + filepath.ToSlash(relDir)
	folderPath = strings.TrimSuffix(folderPath, "/")
	if folderPath == "" {
		folderPath = "/"
	}

	// 4. 调 ffprobe 取时长+分辨率（失败了也继续跑，只是没这些信息）
	meta := &VideoMeta{}
	if s.Thumb != nil {
		if m, err := s.Thumb.Probe(fullPath); err == nil {
			meta = m
		} else {
			log.Println("[scanner] ffprobe 失败:", filepath.Base(fullPath), err)
		}
	}

	// 5. 截封面（失败不影响入库，只是没封面）
	thumbName := ""
	if s.Thumb != nil {
		if n, err := s.Thumb.GenerateThumbnail(fullPath); err == nil {
			thumbName = n
		} else {
			log.Println("[scanner] 截封面失败:", filepath.Base(fullPath), err)
		}
	}

	// 6. INSERT 入库
	now := time.Now()
	v := models.Video{
		FolderPath: folderPath,
		FileName:   filepath.Base(fullPath),
		FilePath:   fullPath,
		FileSize:   info.Size(),
		Duration:   meta.Duration,
		Resolution: meta.Resolution,
		Thumbnail:  thumbName,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	_, err = s.DB.Exec(`
		INSERT INTO videos(folder_path, file_name, file_path, file_size, duration, resolution, thumbnail, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?,?)
	`, v.FolderPath, v.FileName, v.FilePath, v.FileSize, v.Duration, v.Resolution, v.Thumbnail, v.CreatedAt, v.UpdatedAt)
	if err != nil {
		log.Println("[scanner] 入库失败:", fullPath, err)
		s.mu.Lock()
		s.failed++
		s.mu.Unlock()
		return
	}

	log.Printf("[scanner] +新增 %s  (%d MB)", filepath.Base(fullPath), v.FileSize/1024/1024)
	s.mu.Lock()
	s.added++
	s.mu.Unlock()
}
