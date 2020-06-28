package main

import (
	"./skyhook"
	gouuid "github.com/google/uuid"

	"log"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"
)

func main() {
	type Cmd struct {
		cmd *exec.Cmd
		stdin io.WriteCloser
	}
	containers := make(map[string]Cmd)
	var mu sync.Mutex

	http.HandleFunc("/allocate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}

		var request skyhook.Environment
		if err := skyhook.ParseJsonRequest(w, r, &request); err != nil {
			return
		}

		cmd := exec.Command("./container")
		stdin, err := cmd.StdinPipe()
		if err != nil {
			panic(err)
		}
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			panic(err)
		}
		uuid := gouuid.New().String()
		log.Printf("[machine] container %s started", uuid)
		mu.Lock()
		containers[uuid] = Cmd{cmd, stdin}
		mu.Unlock()

		time.Sleep(time.Second)
		skyhook.JsonResponse(w, skyhook.Container{
			UUID: uuid,
			BaseURL: "http://localhost:8082",
		})
	})

	http.HandleFunc("/deallocate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		r.ParseForm()
		uuid := r.Form.Get("uuid")
		mu.Lock()
		containers[uuid].stdin.Close()
		containers[uuid].cmd.Wait()
		mu.Unlock()
		log.Printf("[machine] container %s stopped", uuid)
	})

	log.Printf("starting on :8081")
	if err := http.ListenAndServe(":8081", nil); err != nil {
		panic(err)
	}
}
