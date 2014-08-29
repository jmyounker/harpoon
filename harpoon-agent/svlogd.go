package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"syscall"
)

const maxLogLineLength = 50000

var (
	// persist container logs to disk
	logConfig = `
# rotate if current log is larger than 5242880 bytes
s5242880
# retain at least 20 rotated log
N20
# retain no more than 50 rotated logs
n50
# rotate if current log is older than 30 minutes
t1800
# forward to UDP
u%s
# prefix with container id
pcontainer[%s]:
`
)

func startLogger(name, logdir string) (io.WriteCloser, error) {
	{
		config, err := os.Create(path.Join(logdir, "config"))
		if err != nil {
			return nil, err
		}

		if _, err := fmt.Fprintf(config, logConfig, "0.0.0.0:3334", name); err != nil {
			return nil, err
		}
	}

	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	logger := exec.Command("svlogd",
		"-tt",                              // prefix each line with a UTC timestamp
		"-l", fmt.Sprint(maxLogLineLength), // max line length
		"-b", fmt.Sprint(maxLogLineLength+1), // buffer size for reading/writing
		path.Join(logdir),
	)
	logger.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	logger.Stdin = pr

	if err := logger.Start(); err != nil {
		pw.Close()
		return nil, err
	}

	go logger.Wait()

	return pw, nil
}
