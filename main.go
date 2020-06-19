package main

import (
	"./skyhook"
	_ "./builtins"

	"github.com/googollee/go-socket.io"

	"log"
	"net/http"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	server, err := socketio.NewServer(nil)
	if err != nil {
		log.Fatal(err)
	}
	server.OnConnect("/", func(s socketio.Conn) error {
		return nil
	})
	for _, f := range skyhook.SetupFuncs {
		f(server)
	}
	go server.Serve()
	defer server.Close()
	http.Handle("/socket.io/", server)
	fileServer := http.FileServer(http.Dir("static/"))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	})
	log.Printf("starting on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}
