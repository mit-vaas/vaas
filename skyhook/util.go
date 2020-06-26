package skyhook

import (
	"github.com/googollee/go-socket.io"

	"bytes"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
)

var SetupFuncs []func(*socketio.Server)

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

func ParseJsonRequest(w http.ResponseWriter, r *http.Request, x interface{}) error {
	bytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("json decode error: %v", err), 400)
		return err
	}
	if err := json.Unmarshal(bytes, x); err != nil {
		http.Error(w, fmt.Sprintf("json decode error: %v", err), 400)
		return err
	}
	return nil
}

func JsonPost(baseURL string, path string, request interface{}, response interface{}) error {
	body := bytes.NewBuffer(JsonMarshal(request))
	resp, err := http.Post(baseURL + path, "application/json", body)
	if err != nil {
		return fmt.Errorf("error performing HTTP request: %v", err)
	}
	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error performing HTTP request: %v", err)
	} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(bytes))
	}
	if response != nil {
		JsonUnmarshal(bytes, response)
	}
	return nil
}

func ParseInt(str string) int {
	x, err := strconv.Atoi(str)
	if err != nil {
		panic(err)
	}
	return x
}

func ParseFloat(str string) float64 {
	x, err := strconv.ParseFloat(str, 64)
	if err != nil {
		panic(err)
	}
	return x
}

const Debug bool = false

type Cmd struct {
	prefix string
	cmd *exec.Cmd
	stdin io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	// if not nil, means PrintStderr will send last line it got before exiting
	stderrCh chan string
	closed bool
}

func (cmd *Cmd) Stdin() io.WriteCloser {
	return cmd.stdin
}

func (cmd *Cmd) Stdout() io.ReadCloser {
	return cmd.stdout
}

func (cmd *Cmd) Stderr() io.ReadCloser {
	return cmd.stderr
}

func (cmd *Cmd) Wait() error {
	if cmd.closed {
		panic(fmt.Errorf("closed twice"))
	}
	cmd.closed = true
	if cmd.stdin != nil {
		cmd.stdin.Close()
	}
	if cmd.stdout != nil {
		cmd.stdout.Close()
	}
	var lastLine string
	if cmd.stderrCh != nil {
		lastLine = <- cmd.stderrCh
	}
	err := cmd.cmd.Wait()
	if err != nil {
		log.Printf("[%s] exit error: %v (%s)", cmd.prefix, err, lastLine)
	}
	return err
}

func (cmd *Cmd) printStderr(onlyDebug bool) {
	rd := bufio.NewReader(cmd.stderr)
	var lastLine string
	for {
		line, err := rd.ReadString('\n')
		if err == io.EOF {
			break
		} else if err != nil {
			panic(err)
		}
		line = strings.TrimSpace(line)
		lastLine = line
		if !onlyDebug || Debug {
			log.Printf("[%s] %s", cmd.prefix, line)
		}
	}
	cmd.stderrCh <- lastLine
}

type CommandOptions struct {
	NoStdin bool
	NoStdout bool
	NoStderr bool
	NoPrintStderr bool
	F func(*exec.Cmd)
	OnlyDebug bool
}

func Command(prefix string, opts CommandOptions, command string, args ...string) *Cmd {
	log.Printf("[util] %s %v", command, args)
	cmd := exec.Command(command, args...)
	var stdin io.WriteCloser
	if !opts.NoStdin {
		var err error
		stdin, err = cmd.StdinPipe()
		if err != nil {
			panic(err)
		}
	}
	var stdout io.ReadCloser
	if !opts.NoStdout {
		var err error
		stdout, err = cmd.StdoutPipe()
		if err != nil {
			panic(err)
		}
	}
	var stderr io.ReadCloser
	if !opts.NoStderr {
		var err error
		stderr, err = cmd.StderrPipe()
		if err != nil {
			panic(err)
		}
	}
	if opts.F != nil {
		opts.F(cmd)
	}
	if err := cmd.Start(); err != nil {
		panic(err)
	}
	mycmd := &Cmd{
		prefix: prefix,
		cmd: cmd,
		stdin: stdin,
		stdout: stdout,
		stderr: stderr,
	}
	if stderr != nil && !opts.NoPrintStderr {
		mycmd.stderrCh = make(chan string)
		go mycmd.printStderr(opts.OnlyDebug)
	}
	return mycmd
}

func Mod(a, b int) int {
	x := a%b
	if x < 0 {
		x = x+b
	}
	return x
}
