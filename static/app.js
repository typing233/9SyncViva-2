(function () {
    "use strict";

    const $ = (s) => document.querySelector(s);
    const lobby = $("#lobby");
    const room = $("#room");
    const video = $("#videoPlayer");
    const biliFrame = $("#biliPlayer");
    const placeholder = $("#playerPlaceholder");
    const biliControls = $("#biliControls");

    let ws = null;
    let nickname = "";
    let currentSource = ""; // "alist" | "bilibili"
    let hls = null;
    let ignoreEvents = false;

    // --- Lobby ---
    $("#joinBtn").addEventListener("click", joinRoom);
    $("#nickname").addEventListener("keydown", (e) => { if (e.key === "Enter") joinRoom(); });
    $("#roomId").addEventListener("keydown", (e) => { if (e.key === "Enter") joinRoom(); });

    function joinRoom() {
        nickname = $("#nickname").value.trim();
        if (!nickname) { alert("请输入昵称"); return; }
        let roomId = $("#roomId").value.trim();
        if (!roomId) roomId = randomId();
        connectWS(roomId, nickname);
    }

    function randomId() {
        return Math.random().toString(36).substring(2, 8);
    }

    // --- WebSocket ---
    function connectWS(roomId, nick) {
        const proto = location.protocol === "https:" ? "wss:" : "ws:";
        ws = new WebSocket(`${proto}//${location.host}/ws`);

        ws.onopen = () => {
            send("join", { roomId, nickname: nick });
        };

        ws.onmessage = (evt) => {
            const msg = JSON.parse(evt.data);
            handleServerMsg(msg);
        };

        ws.onclose = () => {
            addSystemMsg("连接已断开");
        };
    }

    function send(type, payload) {
        if (ws && ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({ type, payload }));
        }
    }

    // --- Message Handling ---
    function handleServerMsg(msg) {
        const { type, payload } = msg;
        switch (type) {
            case "room_state":
                enterRoom(payload);
                break;
            case "set_video":
                loadVideo(payload.url, payload.source);
                break;
            case "sync":
                applySync(payload);
                break;
            case "chat":
                addChatMsg(payload.nickname, payload.message);
                break;
            case "member_update":
                updateMembers(payload.members);
                break;
            case "error":
                addSystemMsg("错误: " + payload.message);
                break;
        }
    }

    function enterRoom(state) {
        lobby.classList.add("hidden");
        room.classList.remove("hidden");
        $("#roomTitle").textContent = "房间: " + state.roomId;
        updateMembers(state.members);
        if (state.videoUrl) {
            loadVideo(state.videoUrl, state.videoSource);
            if (currentSource === "alist") {
                video.currentTime = state.currentTime;
                if (state.playing) video.play();
            }
        }
    }

    // --- Video Loading ---
    $("#loadVideoBtn").addEventListener("click", loadFromInput);
    $("#videoUrl").addEventListener("keydown", (e) => { if (e.key === "Enter") loadFromInput(); });

    function isShortLink(input) {
        return /b23\.tv|bili2233\.cn/i.test(input);
    }

    function isBilibiliSource(input) {
        if (/BV[a-zA-Z0-9]{10}/.test(input)) return true;
        if (/bilibili\.com|bilibili\.tv|b23\.tv|bili2233\.cn/i.test(input)) return true;
        return false;
    }

    async function loadFromInput() {
        let raw = $("#videoUrl").value.trim();
        if (!raw) return;

        if (isShortLink(raw)) {
            addSystemMsg("正在解析短链接...");
            const resolved = await resolveShortLink(raw);
            if (resolved && resolved !== raw) {
                raw = resolved;
                $("#videoUrl").value = raw;
                addSystemMsg("短链已解析: " + raw);
            } else {
                addSystemMsg("短链解析失败，请手动在浏览器打开后复制完整链接");
                return;
            }
        }

        let url = raw;
        let source = "alist";

        if (isBilibiliSource(raw)) {
            source = "bilibili";
            url = raw;
        }

        send("set_video", { url, source });
        loadVideo(url, source);
    }

    async function resolveShortLink(url) {
        try {
            const resp = await fetch(`/api/resolve?url=${encodeURIComponent(url)}`);
            if (!resp.ok) return null;
            const data = await resp.json();
            return data.url || null;
        } catch (e) {
            return null;
        }
    }

    function buildBiliEmbedUrl(url) {
        const bvMatch = url.match(/BV[a-zA-Z0-9]{10}/);
        if (bvMatch) {
            return `https://player.bilibili.com/player.html?bvid=${bvMatch[0]}&autoplay=0&high_quality=1`;
        }

        const avMatch = url.match(/av(\d+)/i);
        if (avMatch) {
            return `https://player.bilibili.com/player.html?aid=${avMatch[1]}&autoplay=0&high_quality=1`;
        }

        const epMatch = url.match(/ep(\d+)/i);
        if (epMatch) {
            return `https://player.bilibili.com/player.html?epid=${epMatch[1]}&autoplay=0&high_quality=1`;
        }

        const ssMatch = url.match(/ss(\d+)/i);
        if (ssMatch) {
            return `https://player.bilibili.com/player.html?season_id=${ssMatch[1]}&autoplay=0&high_quality=1`;
        }

        const mdMatch = url.match(/md(\d+)/i);
        if (mdMatch) {
            return `https://player.bilibili.com/player.html?media_id=${mdMatch[1]}&autoplay=0&high_quality=1`;
        }

        return null;
    }

    function loadVideo(url, source) {
        currentSource = source;
        if (hls) { hls.destroy(); hls = null; }
        biliControls.classList.add("hidden");

        if (source === "bilibili") {
            video.classList.add("hidden");
            placeholder.classList.add("hidden");
            biliFrame.classList.remove("hidden");
            biliControls.classList.remove("hidden");

            const embedUrl = buildBiliEmbedUrl(url);
            if (embedUrl) {
                biliFrame.src = embedUrl;
                addSystemMsg("Bilibili 视频已加载 — 请使用下方「同步播放/暂停」按钮控制，iframe 内点击不会同步");
            } else {
                biliFrame.classList.add("hidden");
                biliControls.classList.add("hidden");
                placeholder.classList.remove("hidden");
                placeholder.innerHTML = "<p>无法解析该 Bilibili 链接，请尝试其他格式</p>";
            }
        } else {
            biliFrame.classList.add("hidden");
            placeholder.classList.add("hidden");
            video.classList.remove("hidden");

            if (url.includes(".m3u8")) {
                if (Hls.isSupported()) {
                    hls = new Hls();
                    hls.loadSource(url);
                    hls.attachMedia(video);
                } else if (video.canPlayType("application/vnd.apple.mpegurl")) {
                    video.src = url;
                }
            } else {
                video.src = url;
            }
        }
    }

    // --- Bilibili Sync Buttons ---
    $("#biliPlayBtn").addEventListener("click", () => {
        const frame = biliFrame.contentWindow;
        if (frame) frame.postMessage({ type: "player:play" }, "*");
        send("sync", { action: "play", time: 0 });
    });

    $("#biliPauseBtn").addEventListener("click", () => {
        const frame = biliFrame.contentWindow;
        if (frame) frame.postMessage({ type: "player:pause" }, "*");
        send("sync", { action: "pause", time: 0 });
    });

    // --- Sync Controls (Alist) ---
    video.addEventListener("play", () => {
        if (ignoreEvents) return;
        send("sync", { action: "play", time: video.currentTime });
    });

    video.addEventListener("pause", () => {
        if (ignoreEvents) return;
        send("sync", { action: "pause", time: video.currentTime });
    });

    video.addEventListener("seeked", () => {
        if (ignoreEvents) return;
        send("sync", { action: "seek", time: video.currentTime });
    });

    function applySync(payload) {
        if (currentSource === "bilibili") {
            applyBiliSync(payload);
            return;
        }

        ignoreEvents = true;
        const diff = Math.abs(video.currentTime - payload.time);
        if (diff > 0.5) {
            video.currentTime = payload.time;
        }

        switch (payload.action) {
            case "play":
                video.play();
                break;
            case "pause":
                video.pause();
                break;
            case "seek":
                video.currentTime = payload.time;
                break;
        }

        setTimeout(() => { ignoreEvents = false; }, 300);
    }

    function applyBiliSync(payload) {
        const frame = biliFrame.contentWindow;
        if (!frame) return;
        switch (payload.action) {
            case "play":
                frame.postMessage({ type: "player:play" }, "*");
                break;
            case "pause":
                frame.postMessage({ type: "player:pause" }, "*");
                break;
            case "seek":
                addSystemMsg(`[同步] ${payload.from} 跳转到 ${fmtTime(payload.time)}（Bilibili 不支持精确跳转同步，请手动调整）`);
                break;
        }
    }

    function fmtTime(sec) {
        const m = Math.floor(sec / 60);
        const s = Math.floor(sec % 60);
        return `${m}:${s.toString().padStart(2, "0")}`;
    }

    // --- Chat ---
    $("#sendChatBtn").addEventListener("click", sendChat);
    $("#chatInput").addEventListener("keydown", (e) => { if (e.key === "Enter") sendChat(); });

    function sendChat() {
        const input = $("#chatInput");
        const msg = input.value.trim();
        if (!msg) return;
        send("chat", { message: msg });
        input.value = "";
    }

    function addChatMsg(nick, message) {
        const el = document.createElement("div");
        el.className = "chat-msg";
        el.innerHTML = `<span class="nick">${esc(nick)}</span>${esc(message)}`;
        $("#chatMessages").appendChild(el);
        $("#chatMessages").scrollTop = $("#chatMessages").scrollHeight;
    }

    function addSystemMsg(message) {
        const el = document.createElement("div");
        el.className = "chat-msg system";
        el.textContent = message;
        $("#chatMessages").appendChild(el);
        $("#chatMessages").scrollTop = $("#chatMessages").scrollHeight;
    }

    // --- Members ---
    function updateMembers(members) {
        $("#memberCount").textContent = members.length + " 人在线";
        $("#memberList").textContent = "在线: " + members.join(", ");
    }

    // --- Leave ---
    $("#leaveBtn").addEventListener("click", () => {
        if (ws) ws.close();
        room.classList.add("hidden");
        lobby.classList.remove("hidden");
        video.src = "";
        biliFrame.src = "";
        biliControls.classList.add("hidden");
        $("#chatMessages").innerHTML = "";
    });

    // --- Util ---
    function esc(s) {
        const d = document.createElement("div");
        d.textContent = s;
        return d.innerHTML;
    }
})();
