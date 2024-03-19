package graceful

import (
	"net"
	"os"
)

const (
	envKey       = "VALKYRIECHILD"
	envKeyPrefix = envKey + "="
)

// In order to keep the working directory the same as when we started we record
// it at startup.
var originalWD, _ = os.Getwd()

func StartChild() (*net.UnixConn, error) {
	left, right, err := SocketPair()
	if err != nil {
		return nil, err
	}
	defer left.Close()

	leftFd, _ := left.File()
	defer leftFd.Close()

	// setup files we're passing to the new process
	files := []*os.File{
		os.Stdin,
		os.Stdout,
		os.Stderr,
		leftFd,
	}
	// setup environment we're passing to the new process
	env := os.Environ()
	env = append(env, envKeyPrefix+"true")

	execName, err := os.Executable()
	if err != nil {
		right.Close()
		return nil, err
	}

	proc, err := os.StartProcess(execName, os.Args, &os.ProcAttr{
		Dir:   originalWD,
		Files: files,
		Env:   env,
	})
	if err != nil {
		right.Close()
		return nil, err
	}
	_ = proc

	return right, nil
}

// IsChild returns true if this executable was launched
// by StartChild
func IsChild() bool {
	_, ok := os.LookupEnv(envKey)
	return ok
}
