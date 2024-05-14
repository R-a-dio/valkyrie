package graceful

import (
	"context"
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

func StartChild(ctx context.Context) (*net.UnixConn, error) {
	g := get(ctx)
	if g == nil {
		return nil, ErrNoGraceful
	}

	if g.Child != nil {
		return g.Child, nil
	}

	return startChild()
}

func startChild() (*net.UnixConn, error) {
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
func IsChild(ctx context.Context) bool {
	v := ctx.Value(gracefulKey{})
	if v == nil {
		return false
	}
	return v.(Graceful).IsChild
}

func isChild() bool {
	_, ok := os.LookupEnv(envKey)
	return ok
}
