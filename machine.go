package main

import (
	"./vaas"
	gouuid "github.com/google/uuid"

	"bufio"
	"fmt"
	"log"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

func main() {
	myIP := os.Args[1]
	coordinatorURL := os.Args[2]

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

		var request vaas.Environment
		if err := vaas.ParseJsonRequest(w, r, &request); err != nil {
			return
		}

		cmd := exec.Command("./container", coordinatorURL)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			panic(err)
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			panic(err)
		}
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			panic(err)
		}
		uuid := gouuid.New().String()
		log.Printf("[machine] container %s started", uuid)
		mu.Lock()
		containers[uuid] = Cmd{cmd, stdin}
		mu.Unlock()

		rd := bufio.NewReader(stdout)
		line, err := rd.ReadString('\n')
		if err != nil {
			panic(err)
		}
		port := vaas.ParseInt(strings.TrimSpace(line))

		time.Sleep(time.Second)
		vaas.JsonResponse(w, vaas.Container{
			UUID: uuid,
			BaseURL: fmt.Sprintf("http://%s:%d", myIP, port),
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
		cmd, ok := containers[uuid]
		if ok {
			cmd.stdin.Close()
			cmd.cmd.Wait()
			delete(containers, uuid)
		}
		mu.Unlock()
		log.Printf("[machine] container %s stopped", uuid)
	})

	log.Printf("starting on :8081")
	if err := http.ListenAndServe(":8081", nil); err != nil {
		panic(err)
	}
}
