# SyncViva 部署文档

## 概述

SyncViva 是一个实时同步观影应用，支持多人同步播放视频、弹幕互动、Emby 媒体库浏览。单二进制部署，无外部依赖。

## 功能列表

- 房间创建与加入（支持密码保护）
- 视频同步播放/暂停/进度跳转
- 实时滚动弹幕系统
- 聊天室
- Emby 媒体库集成（可选）
- 直播流支持（m3u8/flv）
- Bilibili 视频嵌入播放
- 房间权限管理（房主/成员）
- 自由/房主控制两种同步模式
- NTP 式时间同步校正

---

## 方式一：二进制部署

### 构建

```bash
# 需要 Go 1.22+
make build
```

产出文件：`syncviva`（Linux amd64，约 8MB）

### 运行

```bash
# 最简运行
./syncviva

# 指定端口
PORT=9090 ./syncviva

# 启用 Emby 集成
PORT=8080 EMBY_URL=http://emby.local:8096 EMBY_API_KEY=your-key ./syncviva
```

### systemd 服务（推荐）

```ini
[Unit]
Description=SyncViva
After=network.target

[Service]
Type=simple
ExecStart=/opt/syncviva/syncviva
EnvironmentFile=/opt/syncviva/.env
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

---

## 方式二：Docker 部署

### 构建镜像

```bash
make docker
# 或
docker build -t syncviva .
```

### 运行

```bash
# 创建 .env 文件（参考 config.example.env）
cp config.example.env .env
# 编辑 .env 填入实际配置

# 使用 docker compose
docker compose up -d

# 或直接 docker run
docker run -d --name syncviva -p 8080:8080 --env-file .env syncviva
```

---

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PORT` | `8080` | 服务监听端口 |
| `EMBY_URL` | 空 | Emby 服务器地址（如 `http://192.168.1.100:8096`） |
| `EMBY_API_KEY` | 空 | Emby API Key（在 Emby 后台 → 高级 → API 密钥 中生成） |

当 `EMBY_URL` 和 `EMBY_API_KEY` 都设置时，Emby 媒体库功能自动启用。

---

## Emby 配置

1. 登录 Emby 管理面板
2. 进入 **设置 → 高级 → API 密钥**
3. 点击 **新建 API 密钥**，名称填 `SyncViva`
4. 复制生成的 API Key，填入 `EMBY_API_KEY`
5. 将 Emby 服务器地址填入 `EMBY_URL`

---

## 使用说明

### 基本流程

1. 打开浏览器访问 `http://your-server:8080`
2. 输入昵称，可选填房间号（留空自动生成）和密码
3. 第一个创建房间的人自动成为房主
4. 房主可加载视频、控制播放、管理成员

### 视频源

- **直链**：粘贴 mp4/m3u8 文件直链
- **直播**：粘贴 m3u8/flv 直播流地址
- **Bilibili**：粘贴 BV 号、AV 链接、番剧链接、b23.tv 短链
- **Emby**：房主点击顶部 "Emby" 按钮浏览媒体库选片

### 同步模式

- **自由模式**：房间内任何人都可以控制播放
- **房主控制**：只有房主可以控制播放进度，其他人自动同步

### 弹幕

在视频下方的弹幕栏输入文字，选择颜色和位置，点击发射即可。

---

## 反向代理（Nginx 示例）

```nginx
server {
    listen 443 ssl;
    server_name syncviva.example.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_read_timeout 86400;
    }
}
```

注意：WebSocket 需要 `Upgrade` 和 `Connection` 头以及较长的 `proxy_read_timeout`。

---

## 架构

```
┌────────────┐         WebSocket          ┌─────────────────┐
│  Browser   │ ◄──────────────────────── │  SyncViva       │
│  (Vanilla  │                            │  (Go binary)    │
│   JS)      │ ──────────────────────── │                 │
└────────────┘    JSON messages           │  ┌───────────┐  │
                                          │  │  Hub      │  │
                                          │  │  ┌─────┐  │  │
                                          │  │  │Room │  │  │
                                          │  │  │  *  │  │  │
                                          │  │  └─────┘  │  │
                                          │  └───────────┘  │
                                          │                 │
                                          │  /api/emby/* ──►│──► Emby Server
                                          └─────────────────┘
```

单进程、无数据库、全内存状态。重启后房间数据清空。
