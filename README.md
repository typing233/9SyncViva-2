# SyncViva — 异地同步观影

多人实时同步观影工具，支持 Alist 直链（mp4/m3u8）和 Bilibili 视频。

## 功能

- 房间机制：创建/加入房间，支持 20+ 人同时在线
- 播放同步：play/pause/seek 通过 WebSocket 实时广播（<500ms 延迟）
- 视频源：mp4 直链、m3u8（HLS）、Bilibili（BV号/链接）
- 文字聊天：房间内实时聊天

## 快速启动

### 方式一：直接运行二进制

```bash
# 编译
go build -o syncviva .

# 运行（默认端口 8080）
./syncviva

# 自定义端口
PORT=3000 ./syncviva
```

### 方式二：Docker

```bash
docker compose up --build
```

启动后浏览器打开 `http://localhost:8080`。

## 使用方法

1. 打开页面，输入昵称和房间号（留空自动生成）
2. 进入房间后，在顶部输入框粘贴视频链接：
   - **Alist 直链**：直接粘贴 mp4 或 m3u8 URL
   - **Bilibili**：粘贴 BV 号（如 `BV1xx411c7mD`）或完整链接
3. 房间内任何人操作播放/暂停/跳转，所有人同步
4. 右侧面板可发送文字聊天

## 远程访问

部署在公网服务器后，其他人通过 `http://<服务器IP>:8080` 即可加入。

如需 HTTPS，建议前置 nginx/caddy 反代并配置 SSL 证书。

## 技术架构

- **后端**：Go + gorilla/websocket，单二进制，零依赖
- **前端**：纯 HTML/CSS/JS，hls.js 处理 m3u8
- **同步**：WebSocket JSON 协议，服务端广播，客户端本地执行

## 限制

- Bilibili 使用 embed iframe，仅支持播放/暂停同步，精确 seek 受限
- 不存储聊天历史，刷新后消失
- 房间在所有人离开后自动销毁
