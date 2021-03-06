package main

import (
	"./vaas"
	_ "./builtins"

	"fmt"
	"io/ioutil"
	"log"
	"os"
	"net"
	"net/http"
	"sync"
)

func main() {
	myUUID := os.Args[1]
	coordinatorURL := os.Args[2]

	log.Println("new container", myUUID, os.Getpid())

	vaas.SeedRand()

	executors := make(map[int]vaas.Executor)
	buffers := make(map[string]map[int]vaas.DataBuffer)
	var mu sync.Mutex
	cond := sync.NewCond(&mu)

	http.HandleFunc("/query/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}

		var context vaas.ExecContext
		if err := vaas.ParseJsonRequest(w, r, &context); err != nil {
			return
		}

		r.ParseForm()
		nodeID := vaas.ParseInt(r.Form.Get("node_id"))
		node := context.Nodes[nodeID]

		buf := func() vaas.DataBuffer {
			// if we already have the buffer for it, just use that
			// synchronization is a bit complicated because e.Run call may recursively
			// request more buffers, and we can't hold the lock here on the recursive calls
			mu.Lock()
			if buffers[context.UUID] == nil {
				buffers[context.UUID] = make(map[int]vaas.DataBuffer)
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
				log.Printf("container %s starting node %s", myUUID, node.Name)
				executors[node.ID] = vaas.Executors[node.Type].New(*node)
			}
			e := executors[node.ID]

			// placeholder buffer
			buffers[context.UUID][node.ID] = nil
			mu.Unlock()

			buf = e.Run(context)

			// asynchronously persist the outputs
			addOutputItem := func(format string, freq int, dims [2]int) vaas.Item {
					request := vaas.AddOutputItemRequest{
						Node: *node,
						Vector: context.Vector,
						Slice: context.Slice,
						Format: format,
						Freq: freq,
						Dims: dims,
					}
					var item vaas.Item
					vaas.JsonPost(coordinatorURL, "/series/add-output-item", request, &item)
					return item
			}
			if !context.Opts.NoPersist && node.DataType != vaas.VideoType {
				go func() {
					rd := buf.Reader()
					data, err := rd.Read(context.Slice.Length())
					if err != nil {
						return
					}
					item := addOutputItem("json", rd.Freq(), [2]int{0, 0})
					item.UpdateData(data)
				}()
			} else if context.Opts.PersistVideo {
				go func() {
					rd := buf.Reader()
					if context.Slice.Length() == 1 {
						data, err := rd.Read(1)
						if err != nil {
							return
						}
						im := data.(vaas.VideoData)[0]
						item := addOutputItem("jpeg", rd.Freq(), [2]int{im.Width, im.Height})
						item.Mkdir()
						err = ioutil.WriteFile(item.Fname(0), im.AsJPG(), 0644)
						if err != nil {
							panic(err)
						}
					} else {
						videoRd := rd.(*vaas.VideoBufferReader)
						item := addOutputItem("mp4", rd.Freq(), videoRd.GetDims())
						file, err := os.Create(item.Fname(0))
						if err != nil {
							panic(err)
						}
						err = videoRd.ReadMP4(file)
						if err != nil {
							panic(err)
						}
						file.Close()
					}
				}()
			}

			mu.Lock()
			buffers[context.UUID][node.ID] = buf
			cond.Broadcast()
			mu.Unlock()

			return buf
		}()


		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(200)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		err := buf.(vaas.DataBufferIOWriter).ToWriter(w)
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
		nodeID := vaas.ParseInt(r.Form.Get("node_id"))

		mu.Lock()
		e := executors[nodeID]
		mu.Unlock()
		if e == nil {
			http.Error(w, "no such node", 404)
		}
		statsProvider, ok := e.(vaas.StatsProvider)
		if !ok {
			vaas.JsonResponse(w, vaas.StatsSample{})
			return
		}
		sample := statsProvider.Stats()
		vaas.JsonResponse(w, sample)
	})

	http.HandleFunc("/allstats", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		m := make(map[int]vaas.StatsSample)
		mu.Lock()
		for nodeID, e := range executors {
			statsProvider, ok := e.(vaas.StatsProvider)
			if !ok {
				continue
			}
			m[nodeID] = statsProvider.Stats()
		}
		mu.Unlock()
		vaas.JsonResponse(w, m)
	})

	http.HandleFunc("/query/finish", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		uuid := r.Form.Get("uuid")
		mu.Lock()
		delete(buffers, uuid)
		mu.Unlock()
	})

	ln, err := net.Listen("tcp", "")
	if err != nil {
		panic(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	log.Printf("starting on port %d", port)
	os.Stdout.Write([]byte(fmt.Sprintf("%d\n", port)))

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
		ln.Close()
		os.Exit(0)
	}()

	if err := http.Serve(ln, nil); err != nil {
		log.Println("serve error:", err)
	}
}
