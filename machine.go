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
	if len(os.Args) < 4 {
		fmt.Println("usage: ./machine [external IP] [coordinator URL] [num gpus]")
		fmt.Println("example: ./machine localhost http://localhost:8080 2")
		return
	}
	myIP := os.Args[1]
	coordinatorURL := os.Args[2]
	gpus := vaas.ParseInt(os.Args[3])

	type Cmd struct {
		cmd *exec.Cmd
		stdin io.WriteCloser
		gpus []int
	}
	containers := make(map[string]Cmd)
	var mu sync.Mutex

	gpusInUse := make(map[int]bool)
	for i := 0; i < gpus; i++ {
		gpusInUse[i] = false
	}

	getGPUs := func(num int) ([]int, error) {
		var gpus []int
		for i := 0; i < num; i++ {
			var freeGPU int = -1
			for idx, used := range gpusInUse {
				if used {
					continue
				}
				freeGPU = idx
				break
			}
			if freeGPU == -1 {
				return nil, fmt.Errorf("insufficient idle GPUs")
			}
			gpus = append(gpus, freeGPU)
		}
		for _, idx := range gpus {
			gpusInUse[idx] = true
		}
		return gpus, nil
	}

	cudaStr := func(gpus []int) string {
		var parts []string
		for _, idx := range gpus {
			parts = append(parts, fmt.Sprintf("%d", idx))
		}
		return strings.Join(parts, ",")
	}

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

		// assign GPUs if needed
		var gpus []int
		if request.Requirements["gpu"] > 0 {
			var err error
			gpus, err = getGPUs(request.Requirements["gpu"])
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			for _, env := range os.Environ() {
				if strings.Contains(env, "CUDA_VISIBLE_DEVICES") {
					cmd.Env = append(cmd.Env, env)
				}
			}
			cmd.Env = append(cmd.Env, cudaStr(gpus))
		}

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
		containers[uuid] = Cmd{
			cmd: cmd,
			stdin: stdin,
			gpus: gpus,
		}
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
