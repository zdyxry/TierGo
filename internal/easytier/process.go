package easytier

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

const coreProcessWaitDelay = time.Second

// coreProcess is the easytier-core this client started. The core writes its
// console log to stdout and stderr; both are merged into one stream of lines
// and drained until it exits, which also reaps it.
type coreProcess struct {
	cmd    *exec.Cmd
	lines  chan string
	done   chan struct{}
	waitMu sync.Mutex
	waitEr error
}

func startCoreProcess(binary string, args []string, dir string) (*coreProcess, error) {
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	stdout, stdoutWriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	stderr, stderrWriter, err := os.Pipe()
	if err != nil {
		_ = stdout.Close()
		_ = stdoutWriter.Close()
		return nil, err
	}
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter
	if err := cmd.Start(); err != nil {
		for _, closer := range []io.Closer{stdout, stdoutWriter, stderr, stderrWriter} {
			_ = closer.Close()
		}
		return nil, err
	}
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()

	process := &coreProcess{
		cmd:   cmd,
		lines: make(chan string, 256),
		done:  make(chan struct{}),
	}
	var wg sync.WaitGroup
	for _, reader := range []*os.File{stdout, stderr} {
		wg.Add(1)
		go func(reader *os.File) {
			defer wg.Done()
			scanner := bufio.NewScanner(reader)
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for scanner.Scan() {
				process.lines <- scanner.Text()
			}
		}(reader)
	}
	go func() {
		err := cmd.Wait()
		readersDone := make(chan struct{})
		go func() {
			wg.Wait()
			close(readersDone)
		}()
		timer := time.NewTimer(coreProcessWaitDelay)
		select {
		case <-readersDone:
			timer.Stop()
		case <-timer.C:
			_ = stdout.Close()
			_ = stderr.Close()
			<-readersDone
		}
		_ = stdout.Close()
		_ = stderr.Close()
		process.waitMu.Lock()
		process.waitEr = err
		process.waitMu.Unlock()
		close(process.lines)
		close(process.done)
	}()
	return process, nil
}

// output streams the core's console log until it exits.
func (p *coreProcess) output() <-chan string { return p.lines }

// Wait blocks until the core exits.
func (p *coreProcess) Wait() error {
	<-p.done
	p.waitMu.Lock()
	defer p.waitMu.Unlock()
	return p.waitEr
}

// Stop asks the core to exit, which lets it remove its TUN device and routes.
func (p *coreProcess) Stop() { p.stopSoft() }

// Kill ends the core at once, for one that ignores Stop.
func (p *coreProcess) Kill() {
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
}

// pid is the process number, or 0 when there is none.
func (p *coreProcess) pid() int {
	if p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

func (p *coreProcess) String() string {
	return fmt.Sprintf("easytier-core (pid %d)", p.cmd.Process.Pid)
}
