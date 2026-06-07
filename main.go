package main

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed static
var staticFiles embed.FS

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func main() {
	hub := NewHub()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	staticFS, _ := fs.Sub(staticFiles, "static")
	http.Handle("/", http.FileServer(http.FS(staticFS)))

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("upgrade error:", err)
			return
		}
		client := NewClient(hub, conn)
		go client.WritePump()
		go client.ReadPump()
	})

	http.HandleFunc("/api/resolve", handleResolve)

	emby := NewEmbyClient()
	if emby != nil {
		http.HandleFunc("/api/emby/libraries", emby.HandleLibraries)
		http.HandleFunc("/api/emby/items", emby.HandleItems)
		http.HandleFunc("/api/emby/stream", emby.HandleStream)
		log.Printf("Emby integration enabled: %s", os.Getenv("EMBY_URL"))
	}

	log.Printf("SyncViva server listening on :%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func handleResolve(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Query().Get("url")
	if url == "" {
		http.Error(w, "missing url param", 400)
		return
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	finalURL := url
	for i := 0; i < 5; i++ {
		resp, err := client.Get(finalURL)
		if err != nil {
			break
		}
		resp.Body.Close()
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			loc := resp.Header.Get("Location")
			if loc == "" {
				break
			}
			finalURL = loc
		} else {
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": finalURL})
}
