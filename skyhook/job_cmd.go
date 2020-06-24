package skyhook

import (
	"bufio"
	"os/exec"
	"strings"
	"sync"
)

type CmdJob struct {
	label string
	cmd string
	args []string

	l []string
	mu sync.Mutex
}

func NewCmdJob(label string, cmd string, args ...string) *CmdJob {
	return &CmdJob{
		label: label,
		cmd: cmd,
		args: args,
	}
}

func (j *CmdJob) Name() string {
	return j.label
}

func (j *CmdJob) Type() string {
	return "cmd"
}

func (j *CmdJob) Run(statusFunc func(string)) error {
	statusFunc("Running")
	cmd := exec.Command(j.cmd, j.args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		panic(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		rd := bufio.NewReader(stderr)
		for {
			line, err := rd.ReadString('\n')
			if err != nil {
				break
			}
			j.mu.Lock()
			j.l = append(j.l, "[stderr] " + strings.TrimSpace(line))
			j.mu.Unlock()
		}
		stderr.Close()
	}()
	go func() {
		rd := bufio.NewReader(stdout)
		for {
			line, err := rd.ReadString('\n')
			if err != nil {
				break
			}
			j.mu.Lock()
			j.l = append(j.l, "[stdout] " + strings.TrimSpace(line))
			j.mu.Unlock()
		}
		stdout.Close()
	}()
	return cmd.Wait()
}

func (j *CmdJob) Detail() interface{} {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.l
}
