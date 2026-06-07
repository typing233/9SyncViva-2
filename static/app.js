(function () {
    "use strict";

    const $ = (s) => document.querySelector(s);
    const lobby = $("#lobby");
    const room = $("#room");
    const video = $("#videoPlayer");
    const biliFrame = $("#biliPlayer");
    const placeholder = $("#playerPlaceholder");
    const biliControls = $("#biliControls");
    const danmakuOverlay = $("#danmakuOverlay");

    let ws = null;
    let nickname = "";
    let currentRoomId = "";
    let currentSource = "";
    let isLive = false;
    let isOwner = false;
    let hls = null;
    let flvPlayer = null;
    let ignoreEvents = false;
    let previousMembers = [];

    // Sync timing
    let clockOffset = 0;
    let rtt = 100;
    let pingInterval = null;
    let pingHistory = [];

    // Danmaku track cycling
    let danmakuTrack = 0;

    // --- Lobby ---
    $("#joinBtn").addEventListener("click", joinRoom);
    $("#nickname").addEventListener("keydown", (e) => { if (e.key === "Enter") joinRoom(); });
    $("#roomId").addEventListener("keydown", (e) => { if (e.key === "Enter") joinRoom(); });
    $("#roomPassword").addEventListener("keydown", (e) => { if (e.key === "Enter") joinRoom(); });

    function joinRoom() {
        nickname = $("#nickname").value.trim();
        if (!nickname) { alert("请输入昵称"); return; }
        let roomId = $("#roomId").value.trim();
        if (!roomId) roomId = randomId();
        const password = $("#roomPassword").value;
        connectWS(roomId, nickname, password);
    }

    function randomId() {
        return Math.random().toString(36).substring(2, 8);
    }

    // --- Password Modal ---
    let pendingJoin = null;

    $("#modalPasswordBtn").addEventListener("click", () => {
        if (!pendingJoin) return;
        const pw = $("#modalPassword").value;
        send("join", { roomId: pendingJoin.roomId, nickname: pendingJoin.nickname, password: pw });
        $("#passwordModal").classList.add("hidden");
        $("#modalPassword").value = "";
    });

    $("#modalCancelBtn").addEventListener("click", () => {
        $("#passwordModal").classList.add("hidden");
        pendingJoin = null;
    });

    // --- WebSocket ---
    function connectWS(roomId, nick, password) {
        const proto = location.protocol === "https:" ? "wss:" : "ws:";
        ws = new WebSocket(`${proto}//${location.host}/ws`);
        pendingJoin = { roomId, nickname: nick, password: password || "" };

        ws.onopen = () => {
            send("join", { roomId, nickname: nick, password: password || "" });
            startPingMeasurement();
        };

        ws.onmessage = (evt) => {
            const msg = JSON.parse(evt.data);
            handleServerMsg(msg);
        };

        ws.onclose = () => {
            stopPingMeasurement();
            addSystemMsg("连接已断开");
        };
    }

    function send(type, payload) {
        if (ws && ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({ type, payload }));
        }
    }

    // --- Ping/Pong RTT Measurement ---
    function startPingMeasurement() {
        pingInterval = setInterval(() => {
            send("ping", { clientTime: Date.now() });
        }, 10000);
        send("ping", { clientTime: Date.now() });
    }

    function stopPingMeasurement() {
        if (pingInterval) { clearInterval(pingInterval); pingInterval = null; }
    }

    function handlePong(payload) {
        const now = Date.now();
        const currentRtt = now - payload.clientTime;
        pingHistory.push({ rtt: currentRtt, offset: payload.serverTime - payload.clientTime - currentRtt / 2 });
        if (pingHistory.length > 5) pingHistory.shift();
        const sorted = [...pingHistory].sort((a, b) => a.rtt - b.rtt);
        const best = sorted.slice(0, 3);
        rtt = best.reduce((s, v) => s + v.rtt, 0) / best.length;
        clockOffset = best.reduce((s, v) => s + v.offset, 0) / best.length;
    }

    // --- Message Handling ---
    function handleServerMsg(msg) {
        const { type, payload } = msg;
        switch (type) {
            case "room_state":
                enterRoom(payload);
                break;
            case "set_video":
                loadVideo(payload.url, payload.source, payload.isLive);
                break;
            case "sync":
                applySync(payload);
                break;
            case "chat":
                addChatMsg(payload.nickname, payload.message);
                break;
            case "danmaku":
                renderDanmaku(payload);
                break;
            case "member_update":
                updateMembers(payload.members, payload.owner);
                break;
            case "heartbeat_sync":
                applyHeartbeat(payload);
                break;
            case "pong":
                handlePong(payload);
                break;
            case "room_config":
                applyRoomConfig(payload);
                break;
            case "error":
                handleError(payload.message);
                break;
        }
    }

    function handleError(message) {
        if (message === "wrong password") {
            $("#passwordModal").classList.remove("hidden");
            $("#modalPassword").focus();
        } else if (message === "you have been kicked from the room") {
            showToast("你已被房主踢出房间");
            leaveRoomUI();
        } else {
            addSystemMsg("错误: " + message);
        }
    }

    function enterRoom(state) {
        lobby.classList.add("hidden");
        room.classList.remove("hidden");
        currentRoomId = state.roomId;
        $("#roomTitle").textContent = "房间: " + state.roomId;
        isOwner = state.owner === nickname;
        updateOwnerUI(state.owner);
        updateMembers(state.members, state.owner);

        if (state.mode) {
            $("#roomMode").value = state.mode;
        }

        if (state.videoUrl) {
            loadVideo(state.videoUrl, state.videoSource, state.isLive);
            if (currentSource === "alist" && !state.isLive) {
                video.currentTime = state.currentTime;
                if (state.playing) video.play();
            }
        }

        checkEmbyAvailable();
    }

    function updateOwnerUI(ownerNick) {
        isOwner = ownerNick === nickname;
        if (isOwner) {
            $("#ownerBadge").classList.remove("hidden");
            $("#roomMode").classList.remove("hidden");
            $("#videoInputBar").classList.remove("hidden");
        } else {
            $("#ownerBadge").classList.add("hidden");
            $("#roomMode").classList.add("hidden");
            // In strict mode, hide video input for non-owners
        }
    }

    function applyRoomConfig(payload) {
        if (payload.mode) {
            $("#roomMode").value = payload.mode;
            showToast("房间模式已切换: " + (payload.mode === "free" ? "自由模式" : "房主控制"));
        }
    }

    // --- Room Mode ---
    $("#roomMode").addEventListener("change", (e) => {
        send("room_config", { mode: e.target.value });
    });

    // --- Copyable Room ID ---
    $("#roomTitle").addEventListener("click", () => {
        if (currentRoomId) {
            navigator.clipboard.writeText(currentRoomId).then(() => {
                showToast("房间号已复制: " + currentRoomId);
            });
        }
    });

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

    function detectIsLive(url) {
        if (/\.flv(\?|$)/i.test(url)) return true;
        if (/\/live\//i.test(url)) return true;
        if (/live.*\.m3u8/i.test(url)) return true;
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
                addSystemMsg("短链解析失败，请手动复制完整链接");
                return;
            }
        }

        let url = raw;
        let source = "alist";
        let live = detectIsLive(raw);

        if (isBilibiliSource(raw)) {
            source = "bilibili";
            live = false;
        }

        send("set_video", { url, source, isLive: live });
        loadVideo(url, source, live);
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
        if (bvMatch) return `https://player.bilibili.com/player.html?bvid=${bvMatch[0]}&autoplay=0&high_quality=1`;
        const avMatch = url.match(/av(\d+)/i);
        if (avMatch) return `https://player.bilibili.com/player.html?aid=${avMatch[1]}&autoplay=0&high_quality=1`;
        const epMatch = url.match(/ep(\d+)/i);
        if (epMatch) return `https://player.bilibili.com/player.html?epid=${epMatch[1]}&autoplay=0&high_quality=1`;
        const ssMatch = url.match(/ss(\d+)/i);
        if (ssMatch) return `https://player.bilibili.com/player.html?season_id=${ssMatch[1]}&autoplay=0&high_quality=1`;
        const mdMatch = url.match(/md(\d+)/i);
        if (mdMatch) return `https://player.bilibili.com/player.html?media_id=${mdMatch[1]}&autoplay=0&high_quality=1`;
        return null;
    }

    function loadVideo(url, source, live) {
        currentSource = source;
        isLive = !!live;
        if (hls) { hls.destroy(); hls = null; }
        if (flvPlayer) { flvPlayer.destroy(); flvPlayer = null; }
        biliControls.classList.add("hidden");

        if (source === "bilibili") {
            video.classList.add("hidden");
            placeholder.classList.add("hidden");
            biliFrame.classList.remove("hidden");
            biliControls.classList.remove("hidden");
            $("#liveBadge").classList.add("hidden");

            const embedUrl = buildBiliEmbedUrl(url);
            if (embedUrl) {
                biliFrame.src = embedUrl;
                addSystemMsg("Bilibili 视频已加载");
            } else {
                biliFrame.classList.add("hidden");
                biliControls.classList.add("hidden");
                placeholder.classList.remove("hidden");
                placeholder.innerHTML = "<p>无法解析该 Bilibili 链接</p>";
            }
        } else {
            biliFrame.classList.add("hidden");
            placeholder.classList.add("hidden");
            video.classList.remove("hidden");

            if (/\.m3u8(\?|$)/i.test(url)) {
                if (typeof Hls !== "undefined" && Hls.isSupported()) {
                    hls = new Hls();
                    hls.loadSource(url);
                    hls.attachMedia(video);
                } else if (video.canPlayType("application/vnd.apple.mpegurl")) {
                    video.src = url;
                }
            } else if (/\.flv(\?|$)/i.test(url)) {
                if (typeof flvjs !== "undefined" && flvjs.isSupported()) {
                    flvPlayer = flvjs.createPlayer({ type: "flv", url: url, isLive: true });
                    flvPlayer.attachMediaElement(video);
                    flvPlayer.load();
                } else {
                    addSystemMsg("浏览器不支持 FLV 播放");
                }
            } else {
                video.src = url;
            }

            if (isLive) {
                $("#liveBadge").classList.remove("hidden");
            } else {
                $("#liveBadge").classList.add("hidden");
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

    // --- Sync Controls ---
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
        if (isLive) return;
        send("sync", { action: "seek", time: video.currentTime });
    });

    function applySync(payload) {
        if (currentSource === "bilibili") {
            applyBiliSync(payload);
            return;
        }

        ignoreEvents = true;

        if (!isLive) {
            const compensatedTime = payload.time + (rtt / 2000);
            const diff = Math.abs(video.currentTime - compensatedTime);
            if (diff > 0.5) {
                video.currentTime = compensatedTime;
            }

            if (payload.action === "seek") {
                video.currentTime = compensatedTime;
            }
        }

        switch (payload.action) {
            case "play":
                video.play();
                break;
            case "pause":
                video.pause();
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
                addSystemMsg(`[同步] ${payload.from} 跳转到 ${fmtTime(payload.time)}`);
                break;
        }
    }

    function applyHeartbeat(payload) {
        if (currentSource !== "alist" || isLive) return;
        if (!payload.playing) return;

        const compensatedTime = payload.time + (rtt / 2000);
        const drift = Math.abs(video.currentTime - compensatedTime);
        if (drift > 1.5) {
            ignoreEvents = true;
            video.currentTime = compensatedTime;
            setTimeout(() => { ignoreEvents = false; }, 300);
        }
    }

    function fmtTime(sec) {
        const m = Math.floor(sec / 60);
        const s = Math.floor(sec % 60);
        return `${m}:${s.toString().padStart(2, "0")}`;
    }

    // --- Danmaku ---
    $("#sendDanmakuBtn").addEventListener("click", sendDanmaku);
    $("#danmakuText").addEventListener("keydown", (e) => { if (e.key === "Enter") sendDanmaku(); });

    function sendDanmaku() {
        const text = $("#danmakuText").value.trim();
        if (!text) return;
        const color = $("#danmakuColor").value;
        const position = $("#danmakuPosition").value;
        send("danmaku", { text, color, position });
        $("#danmakuText").value = "";
    }

    function renderDanmaku(payload) {
        const el = document.createElement("div");
        el.className = "danmaku-item " + (payload.position || "scroll");
        el.textContent = payload.text;
        el.style.color = payload.color || "#FFFFFF";

        if (payload.position === "scroll" || !payload.position) {
            const track = danmakuTrack++ % 15;
            el.style.top = (5 + track * 6) + "%";
        } else if (payload.position === "top") {
            const track = danmakuTrack++ % 5;
            el.style.top = (5 + track * 7) + "%";
        } else if (payload.position === "bottom") {
            const track = danmakuTrack++ % 5;
            el.style.bottom = (5 + track * 7) + "%";
            el.style.top = "auto";
        }

        danmakuOverlay.appendChild(el);
        el.addEventListener("animationend", () => el.remove());

        // Limit visible danmaku
        while (danmakuOverlay.childElementCount > 100) {
            danmakuOverlay.removeChild(danmakuOverlay.firstChild);
        }
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
    function updateMembers(members, owner) {
        $("#memberCount").textContent = members.length + " 人在线";

        // Detect join/leave for toast
        if (previousMembers.length > 0) {
            const joined = members.filter(m => !previousMembers.includes(m));
            const left = previousMembers.filter(m => !members.includes(m));
            joined.forEach(m => { if (m !== nickname) showToast(m + " 加入了房间"); });
            left.forEach(m => showToast(m + " 离开了房间"));
        }
        previousMembers = [...members];

        // Update owner state
        if (owner) updateOwnerUI(owner);

        // Render member list with kick buttons for owner
        const listEl = $("#memberList");
        listEl.innerHTML = "";
        members.forEach(m => {
            const tag = document.createElement("span");
            tag.className = "member-tag" + (m === owner ? " owner" : "");
            tag.textContent = m + (m === owner ? " (房主)" : "");
            if (isOwner && m !== nickname) {
                const kickBtn = document.createElement("span");
                kickBtn.className = "kick-btn";
                kickBtn.textContent = "×";
                kickBtn.title = "踢出";
                kickBtn.addEventListener("click", () => {
                    if (confirm("确定要踢出 " + m + " 吗？")) {
                        send("kick", { nickname: m });
                    }
                });
                tag.appendChild(kickBtn);
            }
            listEl.appendChild(tag);
        });
    }

    // --- Leave ---
    $("#leaveBtn").addEventListener("click", leaveRoomUI);

    function leaveRoomUI() {
        if (ws) ws.close();
        room.classList.add("hidden");
        lobby.classList.remove("hidden");
        video.src = "";
        biliFrame.src = "";
        biliControls.classList.add("hidden");
        $("#chatMessages").innerHTML = "";
        $("#liveBadge").classList.add("hidden");
        danmakuOverlay.innerHTML = "";
        isOwner = false;
        previousMembers = [];
        currentRoomId = "";
        currentSource = "";
        isLive = false;
    }

    // --- Emby ---
    let embyAvailable = false;
    let embyStack = [];

    async function checkEmbyAvailable() {
        try {
            const resp = await fetch("/api/emby/libraries");
            embyAvailable = resp.ok;
            if (embyAvailable && isOwner) {
                $("#embyBtn").classList.remove("hidden");
            }
        } catch { embyAvailable = false; }
    }

    $("#embyBtn").addEventListener("click", () => {
        embyStack = [];
        loadEmbyLibraries();
        $("#embyPanel").classList.remove("hidden");
    });

    $("#embyCloseBtn").addEventListener("click", () => {
        $("#embyPanel").classList.add("hidden");
    });

    async function loadEmbyLibraries() {
        try {
            const resp = await fetch("/api/emby/libraries");
            const data = await resp.json();
            embyStack = [{ name: "媒体库", id: null }];
            renderEmbyItems(data, true);
            updateEmbyBreadcrumb();
        } catch (e) {
            addSystemMsg("Emby 加载失败: " + e.message);
        }
    }

    async function loadEmbyFolder(parentId, name) {
        try {
            const resp = await fetch(`/api/emby/items?parentId=${parentId}`);
            const data = await resp.json();
            embyStack.push({ name, id: parentId });
            renderEmbyItems(data.Items || data, false);
            updateEmbyBreadcrumb();
        } catch (e) {
            addSystemMsg("Emby 加载失败: " + e.message);
        }
    }

    function renderEmbyItems(items, isLibrary) {
        const list = $("#embyItems");
        list.innerHTML = "";
        if (!items || items.length === 0) {
            list.innerHTML = '<div style="padding:1rem;color:#888;">空</div>';
            return;
        }
        items.forEach(item => {
            const el = document.createElement("div");
            el.className = "emby-item";

            const id = item.Id || item.ItemId;
            const name = item.Name || item.Path || "Unknown";
            const type = item.Type || item.CollectionType || "";
            const isFolder = type === "Folder" || type === "Series" || type === "Season" ||
                             type === "CollectionFolder" || type === "UserView" || isLibrary;

            el.innerHTML = `<span class="icon">${isFolder ? "📁" : "🎬"}</span><span class="name">${esc(name)}</span><span class="meta">${type}</span>`;

            if (isFolder || isLibrary) {
                const folderId = isLibrary ? (item.ItemId || item.Id) : id;
                el.addEventListener("click", () => loadEmbyFolder(folderId, name));
            } else {
                el.addEventListener("click", () => selectEmbyItem(id, name));
            }
            list.appendChild(el);
        });
    }

    function updateEmbyBreadcrumb() {
        const bc = $("#embyBreadcrumb");
        bc.innerHTML = embyStack.map((item, i) => {
            if (i === embyStack.length - 1) return `<span style="color:#eee">${esc(item.name)}</span>`;
            return `<span data-idx="${i}">${esc(item.name)}</span>`;
        }).join(" / ");
        bc.querySelectorAll("span[data-idx]").forEach(el => {
            el.addEventListener("click", () => {
                const idx = parseInt(el.dataset.idx);
                embyStack = embyStack.slice(0, idx + 1);
                const target = embyStack[embyStack.length - 1];
                if (target.id === null) {
                    loadEmbyLibraries();
                } else {
                    embyStack.pop();
                    loadEmbyFolder(target.id, target.name);
                }
            });
        });
    }

    async function selectEmbyItem(itemId, name) {
        try {
            const resp = await fetch(`/api/emby/stream?itemId=${itemId}`);
            const data = await resp.json();
            if (data.url) {
                send("set_video", { url: data.url, source: "alist", isLive: false });
                loadVideo(data.url, "alist", false);
                $("#embyPanel").classList.add("hidden");
                showToast("正在播放: " + name);
            }
        } catch (e) {
            addSystemMsg("获取播放地址失败");
        }
    }

    // --- Toast ---
    function showToast(message, duration) {
        duration = duration || 3000;
        const container = $("#toastContainer");
        const toast = document.createElement("div");
        toast.className = "toast";
        toast.textContent = message;
        container.appendChild(toast);
        setTimeout(() => toast.classList.add("fade-out"), duration - 400);
        setTimeout(() => toast.remove(), duration);
    }

    // --- Util ---
    function esc(s) {
        const d = document.createElement("div");
        d.textContent = s;
        return d.innerHTML;
    }
})();
