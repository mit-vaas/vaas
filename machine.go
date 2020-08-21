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
	"runtime"
	"strings"
	"sync"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("usage: ./machine [external IP] [port] [coordinator URL] [CUDA GPU list]")
		fmt.Println("example: ./machine localhost 8081 http://localhost:8080 0,1")
		return
	}
	myIP := os.Args[1]
	port := vaas.ParseInt(os.Args[2])
	coordinatorURL := os.Args[3]
	gpulist := strings.Split(os.Args[4], ",")

	// set gpulist correctly if it's empty
	if len(gpulist) == 1 && gpulist[0] == "" {
		gpulist = nil
	}

	type Cmd struct {
		cmd *exec.Cmd
		stdin io.WriteCloser
		gpuIndexes []int
	}
	containers := make(map[string]Cmd)
	var mu sync.Mutex

	// indexes in gpulist that are in use
	gpusInUse := make(map[int]bool)
	for i := range gpulist {
		gpusInUse[i] = false
	}

	getGPUs := func(num int) ([]int, string, error) {
		var gpuIndexes []int
		for i := 0; i < num; i++ {
			var freeGPUIdx int = -1
			for idx, used := range gpusInUse {
				if used {
					continue
				}
				freeGPUIdx = idx
				break
			}
			if freeGPUIdx == -1 {
				return nil, "", fmt.Errorf("insufficient idle GPUs")
			}
			gpuIndexes = append(gpuIndexes, freeGPUIdx)
		}
		var gpus []string
		for _, idx := range gpuIndexes {
			gpusInUse[idx] = true
			gpus = append(gpus, gpulist[idx])
		}
		cudaStr := "CUDA_VISIBLE_DEVICES=" + strings.Join(gpus, ",")
		return gpuIndexes, cudaStr, nil
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

		uuid := gouuid.New().String()
		cmd := exec.Command("./container", uuid, coordinatorURL)

		// assign GPUs if needed
		var gpuIndexes []int
		if request.Requirements["gpu"] > 0 {
			var cudaStr string
			var err error
			gpuIndexes, cudaStr, err = getGPUs(request.Requirements["gpu"])
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			for _, env := range os.Environ() {
				if !strings.Contains(env, "CUDA_VISIBLE_DEVICES") {
					cmd.Env = append(cmd.Env, env)
				}
			}
			cmd.Env = append(cmd.Env, cudaStr)
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
		log.Printf("[machine] container %s started (gpus=%v)", uuid, gpuIndexes)
		mu.Lock()
		containers[uuid] = Cmd{
			cmd: cmd,
			stdin: stdin,
			gpuIndexes: gpuIndexes,
		}
		mu.Unlock()

		rd := bufio.NewReader(stdout)
		line, err := rd.ReadString('\n')
		if err != nil {
			panic(err)
		}
		port := vaas.ParseInt(strings.TrimSpace(line))

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
			for _, gpuIdx := range cmd.gpuIndexes {
				log.Printf("[machine] ... release GPU idx=%d gpu=%s", gpuIdx, gpulist[gpuIdx])
				gpusInUse[gpuIdx] = false
			}
		}
		mu.Unlock()
		log.Printf("[machine] container %s stopped", uuid)
	})

	// register with the coordinator
	machine := vaas.Machine{
		BaseURL: fmt.Sprintf("http://%s:%d", myIP, port),
		Resources: map[string]int{
			"gpu": len(gpulist),
			"container": runtime.NumCPU() / 4,
		},
	}
	err := vaas.JsonPost(coordinatorURL, "/register-machine", machine, nil)
	if err != nil {
		panic(err)
	}

	log.Printf("starting on :%d", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		panic(err)
	}
}
