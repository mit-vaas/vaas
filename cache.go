package main

import (
	"github.com/google/uuid"

	"net/http"
	"sync"
)

type CacheItem struct {
	Item interface{}
}

type Cache struct {
	items map[string]*CacheItem
	mu sync.Mutex
}

var cache = &Cache{
	items: make(map[string]*CacheItem),
}

func (cache *Cache) Add(item interface{}) string {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	id := uuid.New().String()
	cache.items[id] = &CacheItem{
		Item: item,
	}
	return id
}

func (cache *Cache) Get(id string) interface{} {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	item := cache.items[id]
	if item == nil {
		return nil
	}
	return item.Item
}

func (cache *Cache) Remove(id string) interface{} {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	item := cache.items[id]
	if item == nil {
		return nil
	}
	delete(cache.items, id)
	return item.Item
}

func init() {
	http.HandleFunc("/cache/preview", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		id := r.Form.Get("id")
		//contentType := r.Form.Get("type")
		item := cache.Get(id)
		if item == nil {
			w.WriteHeader(404)
			return
		}
		switch v := item.(type) {
		case []Image:
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write(v[0].AsJPG())
		case *PreviewClip:
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write(v.Images[0].AsJPG())
		}
	})

	http.HandleFunc("/cache/view", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		id := r.Form.Get("id")
		contentType := r.Form.Get("type")
		item := cache.Get(id)
		if item == nil {
			w.WriteHeader(404)
			return
		}
		switch v := item.(type) {
		case []Image:
			if contentType == "jpeg" {
				w.Header().Set("Content-Type", "image/jpeg")
				w.Write(v[0].AsJPG())
			} else if contentType == "mp4" {
				w.Header().Set("Content-Type", "video/mp4")
				w.Write(MakeVideo(v))
			}
		case *PreviewClip:
			w.Header().Set("Content-Type", "video/mp4")
			w.Write(v.GetVideo())
		}
	})
}
