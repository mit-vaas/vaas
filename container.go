package main

import (
	"./skyhook"
	_ "./builtins"

	"io/ioutil"
	"log"
	"os"
	"net/http"
	"sync"
	"time"
)

/*
TODO:
- DataBuffer: expose BinaryReader function
	for SimpleBuffer it just reads Data, encodes it, and returns a []byte packet
	for VideoBuffer it should read the video bytes instead of decoding
- exec: add convenience function to get the buffers of the parents
	because now we are doing bottom-up approach where parents aren't provided already, node has to request them
	so the convenience function loads buffers
*/

func main() {
	executors := make(map[int]skyhook.Executor)
	buffers := make(map[string]map[int]skyhook.DataBuffer)
	var mu sync.Mutex
	cond := sync.NewCond(&mu)

	http.HandleFunc("/query/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}

		var context skyhook.ExecContext
		if err := skyhook.ParseJsonRequest(w, r, &context); err != nil {
			return
		}

		r.ParseForm()
		nodeID := skyhook.ParseInt(r.Form.Get("node_id"))
		node := context.Nodes[nodeID]

		buf := func() skyhook.DataBuffer {
			// if we already have the buffer for it, just use that
			// synchronization is a bit complicated because e.Run call may recursively
			// request more buffers, and we can't hold the lock here on the recursive calls
			mu.Lock()
			if buffers[context.UUID] == nil {
				buffers[context.UUID] = make(map[int]skyhook.DataBuffer)
			}
			buf, ok := buffers[context.UUID][node.ID]
			if buf != nil {
				mu.Unlock()
				return buf
			} else if ok {
				for buffers[context.UUID][node.ID] == nil {
					cond.Wait()
				}
				mu.Unlock()
				return buffers[context.UUID][node.ID]
			}

			// init the executor if it's not already present
			if executors[node.ID] == nil {
				executors[node.ID] = skyhook.Executors[node.Type](node)
			}
			e := executors[node.ID]

			// placeholder buffer
			buffers[context.UUID][node.ID] = nil
			mu.Unlock()

			buf = e.Run(context)
			time.Sleep(time.Second)

			mu.Lock()
			buffers[context.UUID][node.ID] = buf
			cond.Broadcast()
			mu.Unlock()

			return buf
		}()

		err := buf.(skyhook.DataBufferIOWriter).ToWriter(w)
		if err != nil {
			log.Printf("[node %s %v] error writing buffer: %v", node.Name, context.Slice, err)
		}
	})

	http.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}

		r.ParseForm()
		nodeID := skyhook.ParseInt(r.Form.Get("node_id"))

		mu.Lock()
		e := executors[nodeID]
		mu.Unlock()
		if e == nil {
			http.Error(w, "no such node", 404)
		}
		statsProvider, ok := e.(skyhook.StatsProvider)
		if !ok {
			skyhook.JsonResponse(w, skyhook.StatsSample{})
			return
		}
		sample := statsProvider.Stats()
		skyhook.JsonResponse(w, sample)
	})

	http.HandleFunc("/allstats", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		m := make(map[int]skyhook.StatsSample)
		mu.Lock()
		for nodeID, e := range executors {
			statsProvider, ok := e.(skyhook.StatsProvider)
			if !ok {
				continue
			}
			m[nodeID] = statsProvider.Stats()
		}
		mu.Unlock()
		skyhook.JsonResponse(w, m)
	})

	http.HandleFunc("/query/finish", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		uuid := r.Form.Get("uuid")
		mu.Lock()
		delete(buffers, uuid)
		mu.Unlock()
	})

	// kill when stdin is closed
	go func() {
		_, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			panic(err)
		}
		mu.Lock()
		for _, e := range executors {
			e.Close()
		}
		os.Exit(0)
	}()

	log.Printf("starting on :8082")
	if err := http.ListenAndServe(":8082", nil); err != nil {
		panic(err)
	}
}
