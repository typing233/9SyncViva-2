(function () {
    "use strict";

    const $ = (s) => document.querySelector(s);
    const lobby = $("#lobby");
    const room = $("#room");
    const video = $("#videoPlayer");
    const biliFrame = $("#biliPlayer");
    const placeholder = $("#playerPlaceholder");

    let ws = null;
    let nickname = "";
    let currentSource = ""; // "alist" | "bilibili"
    let hls = null;
    let ignoreEvents = false; // prevent echo loops

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

    function loadFromInput() {
        const raw = $("#videoUrl").value.trim();
        if (!raw) return;

        let url = raw;
        let source = "alist";

        const bvMatch = raw.match(/BV[a-zA-Z0-9]+/);
        if (bvMatch || raw.includes("bilibili.com")) {
            source = "bilibili";
            url = bvMatch ? bvMatch[0] : raw;
        }

        send("set_video", { url, source });
        loadVideo(url, source);
    }

    function loadVideo(url, source) {
        currentSource = source;
        if (hls) { hls.destroy(); hls = null; }

        if (source === "bilibili") {
            video.classList.add("hidden");
            placeholder.classList.add("hidden");
            biliFrame.classList.remove("hidden");
            const bvid = url.match(/BV[a-zA-Z0-9]+/);
            if (bvid) {
                biliFrame.src = `https://player.bilibili.com/player.html?bvid=${bvid[0]}&autoplay=0&high_quality=1`;
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
        send("sync", { action: "seek", time: video.currentTime });
    });

    function applySync(payload) {
        if (currentSource !== "alist") return;

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
        $("#chatMessages").innerHTML = "";
    });

    // --- Util ---
    function esc(s) {
        const d = document.createElement("div");
        d.textContent = s;
        return d.innerHTML;
    }
})();
