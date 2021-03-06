package app

import (
	"../vaas"
	"github.com/googollee/go-socket.io"
	"github.com/google/uuid"

	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
)

func init() {
	http.HandleFunc("/exec/job", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		r.ParseForm()
		queryID := vaas.ParseInt(r.PostForm.Get("query_id"))
		vector := ParseVector(r.PostForm.Get("vector"))
		query := GetQuery(queryID)
		if query == nil {
			w.WriteHeader(404)
			return
		}
		job := NewExecJob(query, vector, 30*vaas.FPS)
		go func() {
			err := RunJob(job)
			if err != nil {
				log.Printf("[exec-job %v] error: %v", query.Name, err)
			}
		}()
	})

	SetupFuncs = append(SetupFuncs, func(server *socketio.Server) {
		var mu sync.Mutex
		streams := make(map[string]*ExecStream)

		type ExecRequest struct {
			Vector string
			QueryID int
			Mode string // "random" or "sequential"
			StartSlice vaas.Slice
			Count int
			Unit int
			Continue bool
		}

		server.OnConnect("/exec", func(s socketio.Conn) error {
			return nil
		})

		server.OnError("/exec", func (s socketio.Conn, err error) {
			log.Printf("[socket.io] error on client %v: %v", s.ID(), err)
		})

		server.OnEvent("/exec", "exec", func(s socketio.Conn, request ExecRequest) {
			defer func() {
				if r := recover(); r != nil {
					fmt.Println(r)
					debug.PrintStack()
				}
			}()

			query := GetQuery(request.QueryID)
			if query == nil {
				s.Emit("error", "no such query")
				return
			}
			vector := ParseVector(request.Vector)

			// try to reuse existing task context
			mu.Lock()
			defer mu.Unlock()
			ok := func() bool {
				if !request.Continue {
					return false
				}
				stream := streams[s.ID()]
				if stream == nil {
					return false
				}
				if stream.query.ID != query.ID || Vector(stream.vector).String() != vector.String() {
					return false
				}
				log.Printf("[exec (%s) %v] reuse existing stream with %d new outputs", query.Name, vector, request.Count)
				stream.Get(request.Count)
				return true
			}()
			if ok {
				return
			}

			// build sampler
			var sampler func() *vaas.Slice
			if request.Mode == "random" {
				// find slices where all series in vector are available
				sets := make([][]vaas.Slice, len(vector))
				for i, series := range vector {
					for _, item := range series.ListItems() {
						sets[i] = append(sets[i], item.Slice)
					}
				}
				slices := SliceIntersection(sets)
				sliceSampler := SliceSampler(slices)
				sampler = func() *vaas.Slice {
					slice := sliceSampler.Uniform(request.Unit)
					return &slice
				}
			} else if request.Mode == "sequential" {
				segment := GetSegment(request.StartSlice.Segment.ID)
				if segment == nil || segment.Timeline.ID != vector[0].Timeline.ID {
					s.Emit("error", "invalid segment for sequential request")
					return
				}
				curIdx := request.StartSlice.Start
				sampler = func() *vaas.Slice {
					frameRange := [2]int{curIdx, curIdx+request.Unit}
					if frameRange[1] > segment.Frames {
						return nil
					}
					curIdx += request.Unit
					return &vaas.Slice{
						Segment: segment.Segment,
						Start: frameRange[0],
						End: frameRange[1],
					}
				}
			}

			log.Printf("[exec (%s) %v] beginning test for client %v", query.Name, vector, s.ID())
			renderVectors := query.GetOutputVectors(vector)
			stream := NewExecStream(query, vector, sampler, request.Count, vaas.ExecOptions{}, func(slice vaas.Slice, outputs [][]vaas.DataReader, err error) {
				if err != nil {
					s.Emit("exec-reject")
					if !strings.Contains(err.Error(), "selector reject") {
						s.Emit("exec-error", err.Error())
					}
					return
				}

				cacheID := uuid.New().String()
				r := RenderVideo(slice, outputs, RenderOpts{ProgressCallback: func(percent int) {
					type ProgressResponse struct {
						UUID string
						Percent int
					}
					s.Emit("exec-progress", ProgressResponse{cacheID, percent})
				}})
				cache.Put(cacheID, r)
				log.Printf("[exec (%s) %v] test: cached renderer with %d frames, uuid=%s", query.Name, slice, slice.Length(), cacheID)
				var t vaas.DataType = vaas.VideoType
				if len(outputs[0]) >= 2 {
					t = outputs[0][1].Type()
				}
				s.Emit("exec-result", VisualizeResponse{
					PreviewURL: fmt.Sprintf("/cache/preview?id=%s&type=jpeg", cacheID),
					URL: fmt.Sprintf("/cache/view?id=%s", cacheID),
					UUID: cacheID,
					Slice: slice,
					Type: t,
					Vectors: renderVectors,
				})
				go func() {
					err := r.Wait()
					if err != nil {
						s.Emit("exec-error", err.Error())
					}
				}()
			})
			stream.Get(request.Count)
			if streams[s.ID()] != nil {
				streams[s.ID()].Close()
			}
			streams[s.ID()] = stream
		})

		server.OnDisconnect("/exec", func(s socketio.Conn, e string) {
			mu.Lock()
			if streams[s.ID()] != nil {
				streams[s.ID()].Close()
				streams[s.ID()] = nil
			}
			mu.Unlock()
		})
	})
}
