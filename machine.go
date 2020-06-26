package main

import (
	"./skyhook"

	"log"
	"net/http"
	"os"
	"os/exec"
	"time"
)

func main() {
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
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			panic(err)
		}

		time.Sleep(time.Second)
		skyhook.JsonResponse(w, "http://localhost:8082")
	})
	log.Printf("starting on :8081")
	if err := http.ListenAndServe(":8081", nil); err != nil {
		panic(err)
	}
}
