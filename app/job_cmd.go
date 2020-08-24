package app

import (
	"bufio"
	"os/exec"
	"strings"
	"sync"
)

// keep the last N lines
type LinesBuffer struct {
	N int
	l []string
	mu sync.Mutex
}

func (b *LinesBuffer) Append(s string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// set N if it wasn't set
	if b.N == 0 {
		b.N = 2000
	}

	if len(b.l) < b.N {
		b.l = append(b.l, s)
	} else {
		copy(b.l[0:], b.l[1:])
		b.l[b.N-1] = s
	}
}

func (b *LinesBuffer) Get() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	cp := make([]string, len(b.l))
	copy(cp, b.l)
	return cp
}

type CmdJob struct {
	label string
	cmd string
	args []string

	// function to edit cmd parameters before running it, e.g. the working dir
	F func(cmd *exec.Cmd)

	lines *LinesBuffer
}

func NewCmdJob(label string, cmd string, args ...string) *CmdJob {
	return &CmdJob{
		label: label,
		cmd: cmd,
		args: args,
		lines: new(LinesBuffer),
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
	if j.F != nil {
		j.F(cmd)
	}
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
			j.lines.Append("[stderr] " + strings.TrimSpace(line))
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
			j.lines.Append("[stdout] " + strings.TrimSpace(line))
		}
		stdout.Close()
	}()
	return cmd.Wait()
}

func (j *CmdJob) Detail() interface{} {
	return j.lines.Get()
}
