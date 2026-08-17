package store

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// VideoMeta 保存从 ffprobe 读取到的视频元数据。
type VideoMeta struct {
	Duration   int    // 时长（秒，向下取整）
	Width      int    // 宽
	Height     int    // 高
	Resolution string // 宽x高 拼接好的字符串
}

// Thumbnailer 封装所有跟 ffmpeg/ffprobe 有关的调用。
type Thumbnailer struct {
	FFmpegPath  string // ffmpeg 可执行文件路径
	FFprobePath string // ffprobe 可执行文件路径
	ThumbDir    string // 封面图输出目录
}

// NewThumbnailer 构造一个 Thumbnailer，并确保封面目录存在。
func NewThumbnailer(ffmpegPath, ffprobePath, thumbDir string) (*Thumbnailer, error) {
	if err := os.MkdirAll(thumbDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建封面目录失败: %w", err)
	}
	return &Thumbnailer{
		FFmpegPath:  ffmpegPath,
		FFprobePath: ffprobePath,
		ThumbDir:    thumbDir,
	}, nil
}

// CheckTools 启动时调用一下，确保 ffmpeg 和 ffprobe 能用，
// 不然后面扫描到一半才发现会很坑。
func (t *Thumbnailer) CheckTools() error {
	if _, err := exec.LookPath(t.FFmpegPath); err != nil {
		return fmt.Errorf("找不到 ffmpeg (%s)：请先安装 ffmpeg 并加入 PATH，或在 config 里写绝对路径。%w", t.FFmpegPath, err)
	}
	if _, err := exec.LookPath(t.FFprobePath); err != nil {
		return fmt.Errorf("找不到 ffprobe (%s)：通常和 ffmpeg 在同一个目录，安装 ffmpeg 会自带。%w", t.FFprobePath, err)
	}
	return nil
}

// Probe 读取视频元数据（时长 + 分辨率）。
func (t *Thumbnailer) Probe(videoPath string) (*VideoMeta, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// -v quiet：不打日志；-print_format json：输出 json；-show_streams：取流信息
	cmd := exec.CommandContext(ctx, t.FFprobePath,
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		videoPath,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffprobe 执行失败: %w; stderr: %s", err, stderr.String())
	}

	return parseProbeJSON(stdout.Bytes())
}

// ffprobe 输出的 json 结构子集，我们只取需要的字段。
type probeOutput struct {
	Streams []struct {
		CodecType string `json:"codec_type"` // "video" 或 "audio"
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		Duration  string `json:"duration"` // 注意是字符串，比如 "123.45"
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"` // format 里也有 duration，作为兜底
	} `json:"format"`
}

func parseProbeJSON(raw []byte) (*VideoMeta, error) {
	var p probeOutput
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("解析 ffprobe 输出失败: %w", err)
	}
	meta := &VideoMeta{}
	// 找第一条 video 流
	for _, s := range p.Streams {
		if s.CodecType == "video" {
			meta.Width = s.Width
			meta.Height = s.Height
			if s.Duration != "" {
				meta.Duration = secondsStringToInt(s.Duration)
			}
			break
		}
	}
	// 视频流里没读到 duration，就用 format 里的
	if meta.Duration == 0 && p.Format.Duration != "" {
		meta.Duration = secondsStringToInt(p.Format.Duration)
	}
	if meta.Width > 0 && meta.Height > 0 {
		meta.Resolution = strconv.Itoa(meta.Width) + "x" + strconv.Itoa(meta.Height)
	}
	return meta, nil
}

// secondsStringToInt 把 "123.45" 这种秒数字符串转成整数秒（向下取整）。
func secondsStringToInt(s string) int {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int(f)
}

// GenerateThumbnail 从视频第 5 秒截一张 jpg 封面图，保存到 ThumbDir。
// 返回的是生成的封面文件名（相对 ThumbDir 的文件名）。
func (t *Thumbnailer) GenerateThumbnail(videoPath string) (string, error) {
	// 用 videoPath 的 md5 当封面文件名，避免中文/特殊字符搞事情，也天然去重。
	sum := md5.Sum([]byte(videoPath))
	outName := hex.EncodeToString(sum[:]) + ".jpg"
	outPath := filepath.Join(t.ThumbDir, outName)

	// 已存在就不重复截了，节省时间
	if _, err := os.Stat(outPath); err == nil {
		return outName, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// -ss 5：跳到第 5 秒；-i：输入文件；-frames:v 1：只取 1 帧；-q:v 3：JPEG 质量
	cmd := exec.CommandContext(ctx, t.FFmpegPath,
		"-ss", "5",
		"-i", videoPath,
		"-frames:v", "1",
		"-q:v", "3",
		"-vf", "scale=640:-2", // 宽度固定 640，高度按比例，保持清晰度且文件不大
		"-y", // 覆盖同名文件
		outPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// 截失败不至于整个视频都废掉，返回空串让上层处理，程序继续跑
		// 但如果是 5 秒还没到（短视频），就退回到第 0 秒再试一次
		cmd2 := exec.CommandContext(ctx, t.FFmpegPath,
			"-ss", "0",
			"-i", videoPath,
			"-frames:v", "1",
			"-q:v", "3",
			"-vf", "scale=640:-2",
			"-y",
			outPath,
		)
		var stderr2 bytes.Buffer
		cmd2.Stderr = &stderr2
		if err2 := cmd2.Run(); err2 != nil {
			return "", fmt.Errorf("截封面失败（第5秒+第0秒都失败）: %w; stderr: %s", err2, stderr2.String())
		}
	}
	return outName, nil
}
