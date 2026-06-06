package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"

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

	log.Printf("SyncViva server listening on :%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
