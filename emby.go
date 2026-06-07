package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

type EmbyClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func NewEmbyClient() *EmbyClient {
	u := os.Getenv("EMBY_URL")
	key := os.Getenv("EMBY_API_KEY")
	if u == "" || key == "" {
		return nil
	}
	return &EmbyClient{
		baseURL: u,
		apiKey:  key,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (e *EmbyClient) request(path string, params url.Values) ([]byte, error) {
	if params == nil {
		params = url.Values{}
	}
	params.Set("api_key", e.apiKey)
	reqURL := fmt.Sprintf("%s%s?%s", e.baseURL, path, params.Encode())
	resp, err := e.http.Get(reqURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("emby returned %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (e *EmbyClient) HandleLibraries(w http.ResponseWriter, r *http.Request) {
	data, err := e.request("/emby/Library/VirtualFolders", nil)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (e *EmbyClient) HandleItems(w http.ResponseWriter, r *http.Request) {
	parentID := r.URL.Query().Get("parentId")
	if parentID == "" {
		http.Error(w, "missing parentId", 400)
		return
	}
	params := url.Values{}
	params.Set("ParentId", parentID)
	params.Set("Fields", "Overview,Path,MediaSources")
	params.Set("SortBy", "SortName")
	params.Set("SortOrder", "Ascending")
	data, err := e.request("/emby/Items", params)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (e *EmbyClient) HandleStream(w http.ResponseWriter, r *http.Request) {
	itemID := r.URL.Query().Get("itemId")
	if itemID == "" {
		http.Error(w, "missing itemId", 400)
		return
	}
	streamURL := fmt.Sprintf("%s/emby/Items/%s/Download?api_key=%s", e.baseURL, itemID, e.apiKey)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": streamURL})
}
