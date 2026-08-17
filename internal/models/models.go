// Package models 定义数据库表对应的 Go 结构体。
// 所有数据模型都集中在这里，避免散落在各文件里。
package models

import "time"

// Video 对应 videos 表，一条记录 = 一个视频文件。
type Video struct {
	ID         int64     `db:"id"`          // 主键
	FolderPath string    `db:"folder_path"` // 虚拟文件夹路径，如 /电影/动作片（相对 VideoRoot 的相对路径）
	FileName   string    `db:"file_name"`   // 文件名（含扩展名），如 盗梦空间.mp4
	FilePath   string    `db:"file_path"`   // 磁盘绝对完整路径
	FileSize   int64     `db:"file_size"`   // 文件字节数
	Duration   int       `db:"duration"`    // 视频时长（秒）
	Resolution string    `db:"resolution"`  // 宽x高，如 1920x1080；取不到就留空
	Thumbnail  string    `db:"thumbnail"`   // 封面文件保存在 ThumbDir 下的相对文件名
	CreatedAt  time.Time `db:"created_at"`  // 首次入库时间
	UpdatedAt  time.Time `db:"updated_at"`  // 最近修改时间
}

// Folder 虚拟文件夹（只用于页面展示，不对应数据库表）。
type Folder struct {
	Path  string // 完整路径，如 /电影/动作片
	Name  string // 文件夹名称，如 动作片
	Count int    // 该文件夹下的视频数量
}

// ---- 下面是展示用的辅助方法，不存数据库 ----

// SizeHuman 把字节数转成易读的字符串，比如 1.2 GB。
func (v *Video) SizeHuman() string {
	s := float64(v.FileSize)
	switch {
	case s >= 1<<30:
		return human(s, 1<<30, "GB")
	case s >= 1<<20:
		return human(s, 1<<20, "MB")
	case s >= 1<<10:
		return human(s, 1<<10, "KB")
	default:
		return human(s, 1, "B")
	}
}

// DurationHuman 把秒转成分:秒或时:分:秒，比如 01:23:45。
func (v *Video) DurationHuman() string {
	d := v.Duration
	if d <= 0 {
		return "--:--"
	}
	h := d / 3600
	m := (d % 3600) / 60
	s := d % 60
	if h > 0 {
		return format2(h) + ":" + format2(m) + ":" + format2(s)
	}
	return format2(m) + ":" + format2(s)
}

func human(v float64, unit float64, name string) string {
	return trimFloat(v/unit) + " " + name
}

// trimFloat 把浮点数格式化成最多 2 位小数，去掉多余的 0。
func trimFloat(v float64) string {
	s := trimFloatHelper(v)
	return s
}

func format2(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func trimFloatHelper(v float64) string {
	// 四舍五入到 2 位小数
	scaled := int64(v*100 + 0.5)
	whole := scaled / 100
	frac := scaled % 100
	if frac == 0 {
		return itoa(int(whole))
	}
	if frac%10 == 0 {
		return itoa(int(whole)) + "." + itoa(int(frac/10))
	}
	return itoa(int(whole)) + "." + format2(int(frac))
}
