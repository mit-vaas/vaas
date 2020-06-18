package skyhook

import (
	"github.com/google/uuid"

	"io"
	"log"
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

func (cache *Cache) Put(id string, item interface{}) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.items[id] = &CacheItem{
		Item: item,
	}
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
		case *VideoRenderer:
			im, err := v.GetPreview()
			if err != nil {
				log.Printf("[cache] preview: GetPreview: %v", err)
				w.WriteHeader(400)
				return
			}
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write(im.AsJPG())
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
				imReader := &sliceReader{v, 0}
				rd, cmd := MakeVideo(imReader, v[0].Width, v[0].Height)
				w.Header().Set("Content-Type", "video/mp4")
				_, err := io.Copy(w, rd)
				if err != nil {
					log.Printf("[cache] view: read from MakeVideo: %v", err)
				}
				cmd.Wait()
			}
		case *VideoRenderer:
			if contentType == "labels" {
				labels, err := v.GetLabels()
				if err != nil {
					http.Error(w, err.Error(), 400)
					return
				}
				JsonResponse(w, labels[0][1])
			} else if contentType == "meta" {
				type MetaResponse struct {
					Width int
					Height int
				}
				im, err := v.GetPreview()
				if err != nil {
					http.Error(w, err.Error(), 400)
					return
				}
				JsonResponse(w, MetaResponse{im.Width, im.Height})
			} else {
				rd, err := v.GetVideo()
				if err != nil {
					log.Printf("[cache] view: GetVideo: %v", err)
					w.WriteHeader(400)
					return
				}
				w.Header().Set("Content-Type", "video/mp4")
				_, err = io.Copy(w, rd)
				if err != nil {
					log.Printf("[cache] view: read from GetVideo: %v", err)
				}
			}
		}
	})
}
