package skyhook

import (
	"bufio"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"
	"strings"
)

func ReadTextFile(fname string) string {
	bytes, err := ioutil.ReadFile(fname)
	if err != nil {
		panic(err)
	}
	return string(bytes)
}

func ReadJSONFile(fname string, res interface{}) {
	bytes, err := ioutil.ReadFile(fname)
	if err != nil {
		panic(err)
	}
	JsonUnmarshal(bytes, res)}

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}

func JsonMarshal(x interface{}) []byte {
	bytes, err := json.Marshal(x)
	if err != nil {
		panic(err)
	}
	return bytes
}

func JsonUnmarshal(bytes []byte, x interface{}) {
	err := json.Unmarshal(bytes, x)
	if err != nil {
		panic(err)
	}
}

func JsonResponse(w http.ResponseWriter, x interface{}) {
	bytes := JsonMarshal(x)
	w.Header().Set("Content-Type", "application/json")
	w.Write(bytes)
}

func JsonRequest(w http.ResponseWriter, r *http.Request, x interface{}) error {
	bytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(400)
		return err
	}
	if err := json.Unmarshal(bytes, x); err != nil {
		w.WriteHeader(400)
		return err
	}
	return nil
}

const Debug bool = false

func PrintStderr(prefix string, stderr io.ReadCloser, onlyDebug bool) {
	rd := bufio.NewReader(stderr)
	for {
		line, err := rd.ReadString('\n')
		if err == io.EOF {
			break
		} else if err != nil {
			// sometimes the cmd.Wait() may call before we finish reading stderr
			// to fix this we would need to have whoever calls cmd.Wait to wait for this
			//  function to exit before actually calling it
			// but for now we just log the error instead of panic
			log.Printf("[%s] error reading stderr: %v", prefix, err)
			break
		}
		if !onlyDebug || Debug {
			log.Printf("[%s] %s", prefix, strings.TrimSpace(line))
		}
	}
}

func Command(prefix string, onlyDebug bool, command string, args ...string) (*exec.Cmd, io.WriteCloser, io.ReadCloser) {
	cmd := exec.Command(command, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		panic(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		panic(err)
	}
	if err := cmd.Start(); err != nil {
		panic(err)
	}
	go PrintStderr(prefix, stderr, onlyDebug)
	return cmd, stdin, stdout
}

func Mod(a, b int) int {
	x := a%b
	if x < 0 {
		x = x+b
	}
	return x
}
