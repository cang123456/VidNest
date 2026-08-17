# 本地视频 NAS 系统 实现方案

> 适用范围：家里多人局域网使用 | 技术栈：Go + Gin + SQLite + 本地文件存储 + ffmpeg

---

## 0. 已确认的需求清单

| 模块 | 内容 |
|---|---|
| 使用范围 | 家里多人用（局域网访问，比如手机、平板、电视浏览器打开 `http://你电脑IP:端口` 就能用） |
| 视频来源 | ① 扫描本地已有文件夹（自动导入）② 网页端拖拽/点击上传 |
| 核心功能 | ① 文件夹目录浏览 ② 在线播放（网页播放器，可拖动进度条、倍速）③ 视频信息展示（封面、时长、大小、分辨率）④ 搜索/排序/筛选 |
| 界面 | Gin 模板渲染页面（单项目，一个可执行文件跑起来，前后端不分离，对新手最友好） |
| 数据库 | SQLite（零部署，一个 `.db` 文件就是数据库，复制就能备份） |
| 存储位置 | 本地文件夹存储（比如 `D:/Media/Videos`，文件直接放磁盘，省掉 MinIO 的复杂度） |
| 视频封面 | 需要（用 ffmpeg 从视频第 5 秒截一张图当缩略图） |
| 访问安全 | 不需要登录（局域网内直接访问） |
| 代码要求 | 可读性高（命名直白、注释详细、分层清晰） |

---

## 1. 项目目录结构

```
v-go001/
├── main.go                  # 程序入口，初始化+启动服务器
│
├── config/
│   └── config.go            # 配置：端口、视频目录、封面目录、FFmpeg路径等（集中改这里）
│
├── internal/
│   ├── db/
│   │   └── db.go            # 数据库初始化 + 建表（SQLite）
│   │
│   ├── models/
│   │   └── models.go        # 数据结构：Video（视频）、Folder（文件夹）
│   │
│   ├── store/
│   │   ├── scanner.go       # 【核心1】扫描本地文件夹 → 写入数据库 + 截封面
│   │   ├── uploader.go      # 【核心2】网页上传 → 保存文件 + 写入DB + 截封面
│   │   └── thumbnail.go     # 调用 ffmpeg 截缩略图的封装
│   │
│   ├── handler/
│   │   ├── page.go          # 页面渲染：首页、文件夹页、播放页、上传页
│   │   ├── video.go         # 数据接口：搜索、播放流、删除、下载
│   │   └── folder.go        # 文件夹接口：列表、创建、删除
│   │
│   └── router/
│       └── router.go        # 路由注册（哪个URL走哪个函数）
│
├── static/                  # 静态资源
│   ├── css/
│   │   └── style.css        # 页面样式（简洁干净，间距充足）
│   └── js/
│       └── player.js        # 视频播放器（video.js，支持倍速、进度条）
│
├── templates/               # HTML 页面（Gin 模板）
│   ├── layout.html          # 公共头/脚/侧边栏
│   ├── home.html            # 首页：所有视频缩略图网格
│   ├── folder.html          # 文件夹浏览页
│   ├── player.html          # 视频播放页（大播放器 + 信息）
│   └── upload.html          # 拖拽上传页
│
├── data/                    # 运行时数据（程序自动创建）
│   ├── videos.db            # SQLite 数据库文件（自动生成）
│   └── thumbnails/          # 视频封面缩略图（自动生成）
│
├── go.mod
├── go.sum
└── README.txt               # 给你自己看的启动说明 + ffmpeg 安装步骤
```

---

## 2. 数据库表设计

### 表：`videos`（视频表）

| 字段名 | 类型 | 说明 |
|---|---|---|
| id | INTEGER 主键自增 | 视频唯一编号 |
| folder_path | TEXT | 所属文件夹路径（虚拟路径，如 `/电影/动作片`） |
| file_name | TEXT | 文件名，如 `盗梦空间.mp4` |
| file_path | TEXT | 实际磁盘完整路径，如 `D:/Media/Videos/盗梦空间.mp4` |
| file_size | INTEGER | 文件大小（字节），显示时换算成 MB/GB |
| duration | INTEGER | 时长（秒），显示时换算成分:秒 |
| resolution | TEXT | 分辨率，如 `1920x1080`，从 ffmpeg 读 |
| thumbnail | TEXT | 封面图路径（相对于data/thumbnails） |
| created_at | DATETIME | 入库时间（扫描或上传时记录） |
| updated_at | DATETIME | 修改时间 |

> 文件夹本身**不单独建表**，直接用 `videos.folder_path` 字段分组即可。

---

## 3. 核心流程说明

### 3.1 扫描本地文件夹（启动时 + 手动触发）

```
触发：① 程序启动时自动扫描 ② 网页点「重新扫描」按钮
      │
      ▼
1. 读取配置里的「视频根目录」，如 D:/Media/Videos
2. 递归遍历里面所有 .mp4 / .mkv / .avi / .mov / .wmv 文件
3. 逐个文件：
   ├─ 用文件路径查 DB → 已存在则跳过（按 file_path 去重）
   ├─ 不存在则：
   │   ├─ 调 ffmpeg 取「时长 + 分辨率」
   │   ├─ 调 ffmpeg 从第 5 秒截封面图 → 保存到 data/thumbnails/
   │   └─ 把所有信息 INSERT 进 videos 表
4. 扫描完成，返回：新增 X 个、跳过 Y 个
```

### 3.2 网页上传视频

```
用户在 /upload 页拖拽文件 → 浏览器 POST /api/upload
      │
      ▼
1. Gin 接收 multipart 文件
2. 保存文件到「视频根目录/uploads/原文件名」
3. 调用 ffmpeg 取时长/分辨率 + 截封面
4. INSERT 进 videos 表
5. 返回成功 + 跳转播放页
```

### 3.3 在线播放视频

```
用户点缩略图 → /play/123 打开播放页
      │
      ▼
1. 从 DB 查 id=123 的视频 → 拿到 file_path
2. Gin 用 c.File(file_path) 返回文件
3. MinIO ？不，直接读本地文件，支持 HTTP Range（进度条拖动）
4. 前端 <video> 标签 + video.js 实现倍速/全屏
```

---

## 4. 页面设计（4 个页面）

### ① 首页 `/`
- 左侧：文件夹列表（从 DB 中 `folder_path` 去重得到）
- 右侧：视频缩略图网格（卡片：封面 + 文件名 + 时长角标 + 大小）
- 顶部：搜索框（按文件名模糊搜索）、排序按钮（按时间/大小/名称）、筛选后缀
- 右上角：「上传视频」「重新扫描」两个按钮

### ② 文件夹浏览页 `/folder/*path`
- 内容与首页几乎一样，只是右侧只显示该 folder_path 下的视频
- 支持面包屑导航：`根目录 > 电影 > 动作片`

### ③ 播放页 `/play/:id`
- 左/上：大播放器（占页面 60%，支持全屏、倍速 0.5x~2x）
- 右/下：视频信息卡片（文件名、大小、时长、分辨率、入库时间、所属文件夹）
- 底部：「返回列表」「下载原文件」按钮

### ④ 上传页 `/upload`
- 中间一个大虚线框：「拖拽视频文件到这里，或点击选择」
- 支持多文件同时上传
- 显示上传进度条
- 完成后显示「完成！去播放」跳转链接

---

## 5. 实现步骤（按顺序做，每步跑通再下一步）

1. **骨架搭建**：创建目录结构、go.mod、写 `main.go` + `config` + `router`，跑通一个「Hello 页面」，确认 Gin + 模板正常
2. **数据库层**：写 `db.go` 连接 SQLite、建表；写 `models.go` 定义结构体；写一条测试插入/查询
3. **FFmpeg 封装**：写 `thumbnail.go` + 命令行调用封装，拿一个 mp4 测试「取时长 + 截封面」能否跑通（最容易踩坑的一步）
4. **扫描功能**：写 `scanner.go` 递归遍历 + 去重 + 批量入库；手动造几个文件夹测试扫描结果
5. **上传功能**：写 `uploader.go` 接收文件保存 + 截封面 + 入库；写上传页 HTML + 拖拽 JS
6. **页面开发**：
   - layout.html（公共样式+侧边栏+顶部）
   - home.html（缩略图网格 + 搜索排序）
   - folder.html（同 home + 面包屑）
   - player.html（播放器 + 信息）
7. **播放接口**：`/api/stream/:id` 返回视频流，测试能否拖动进度条
8. **搜索/排序/筛选**：首页顶部功能联调
9. **细节打磨**：分页、空状态提示、删除功能、响应式适配手机
10. **README.txt**：写启动步骤 + ffmpeg 安装图文说明

---

## 6. 依赖清单

| 依赖 | 用途 | 安装方式 |
|---|---|---|
| Go 1.21+ | 编程语言 | 官网下载安装 |
| github.com/gin-gonic/gin | Web 框架 | `go get` |
| github.com/mattn/go-sqlite3 | SQLite 驱动 | `go get`（Windows 需要 gcc，用 MinGW 或 TDM-GCC） |
| FFmpeg 命令行工具 | 截图 + 取视频元数据 | https://ffmpeg.org/download.html 下载，加到 PATH |
| video.js (CDN) | 网页播放器 | 直接在 HTML 引 CDN 链接，不用下载 |

> ⚠️ **注意**：`go-sqlite3` 是 CGO 驱动，Windows 必须装 MinGW/gcc 才能编译。这一步我会给你详细安装步骤。

---

## 7. 验证清单（做完后自测）

- [ ] 访问 `http://localhost:8080/` 能打开首页
- [ ] 配置里指向一个有视频的文件夹，启动后能自动扫描入库
- [ ] 首页显示视频封面 + 时长角标，不报错
- [ ] 点击封面进入播放页，视频能正常播放、拖动进度条
- [ ] 点击侧边栏文件夹，能筛选该文件夹下的视频
- [ ] 搜索框输入关键词，能正确过滤结果
- [ ] 上传页拖一个 mp4 进去，能上传完成并在列表看到
- [ ] 同一文件夹下重复扫描不会重复入库
- [ ] 手机连同一 WiFi，打开 `http://电脑IP:8080` 能用

---

## 8. 风险与处理

| 风险 | 影响 | 处理方案 |
|---|---|---|
| FFmpeg 没装或 PATH 不对 | 扫描时报错、没封面 | 启动时检查 ffmpeg 命令是否可用，不可用则给出友好提示 |
| go-sqlite3 在 Windows 编译失败 | 整个项目跑不起来 | 提前给 MinGW 安装教程；实在不行换纯 Go 的 sqlite 驱动（现代c替代版） |
| 视频文件名含中文/空格/特殊字符 | 播放失败或截封面失败 | 所有路径用 `filepath.Join`，不用字符串拼接；Windows 路径用 `\\` 或 `/` |
| 视频太多（几千个）首屏加载慢 | 页面卡 | 加分页（每页 30 个）+ 封面懒加载 |
| 超大视频（>10GB）上传卡死 | 体验差 | 限制上传大小 + 流式写入，必要时分片上传（第一版可以先不做，够用即可） |

---

## 9. 配置文件示例（config/config.go 默认值）

```go
type Config struct {
    Port        int    // 默认 8080
    VideoRoot   string // 默认 ./data/library （建议你改成 D:/Videos 之类的真实路径）
    ThumbDir    string // 默认 ./data/thumbnails
    DBPath      string // 默认 ./data/videos.db
    FFmpegPath  string // 默认 "ffmpeg"（即从PATH找，也可写绝对路径 C:/ffmpeg/bin/ffmpeg.exe）
    ScanOnStart bool   // 默认 true：启动时自动扫描
}
```

---

**以上就是完整方案，请你过目，有任何想调整的地方随时说。觉得 OK 了告诉我一声，我就按步骤 1~10 开工写代码。**
