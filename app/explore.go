package app

import (
	"../vaas"
	"github.com/googollee/go-socket.io"
	"github.com/google/uuid"

	"fmt"
	"log"
	"sync"
)

func init() {
	SetupFuncs = append(SetupFuncs, func(server *socketio.Server) {
		var mu sync.Mutex
		streams := make(map[string]*ExecStream)

		type ExecRequest struct {
			Vector string
			QueryID int
			Mode string // "random" or "sequential"
			StartSlice vaas.Slice
			Count int
			Continue bool
		}

		server.OnConnect("/exec", func(s socketio.Conn) error {
			return nil
		})

		server.OnError("/exec", func (s socketio.Conn, err error) {
			log.Printf("[socket.io] error on client %v: %v", s.ID(), err)
		})

		server.OnEvent("/exec", "exec", func(s socketio.Conn, request ExecRequest) {
			query := GetQuery(request.QueryID)
			if query == nil {
				s.Emit("error", "no such query")
				return
			}
			vector := ParseVector(request.Vector)
			var sampler func() *vaas.Slice
			if request.Mode == "random" {
				sampler = func() *vaas.Slice {
					slice := DBTimeline{Timeline: vector[0].Timeline}.Uniform(VisualizeMaxFrames)
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
					frameRange := [2]int{curIdx, curIdx+VisualizeMaxFrames}
					if frameRange[1] > segment.Frames {
						return nil
					}
					curIdx += VisualizeMaxFrames
					return &vaas.Slice{
						Segment: segment.Segment,
						Start: frameRange[0],
						End: frameRange[1],
					}
				}
			}

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

			log.Printf("[exec (%s) %v] beginning test for client %v", query.Name, vector, s.ID())
			renderVectors := query.GetOutputVectors(vector)
			stream := NewExecStream(query, vector, sampler, request.Count, func(slice vaas.Slice, outputs [][]vaas.DataReader, err error) {
				if err != nil {
					s.Emit("exec-reject")
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
