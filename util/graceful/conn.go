// Copyright 2023 Sneller, Inc.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

//go:build linux || netbsd || openbsd || solaris || freebsd || aix || dragonfly
// +build linux netbsd openbsd solaris freebsd aix dragonfly

// Package usock implements a wrapper
// around the unix(7) SCM_RIGHTS API,
// which allows processes to exchange
// file handles over a unix(7) control socket.
package graceful

import (
	"fmt"
	"net"
	"os"
	"syscall"
)

// this needs to be large enough
// to accept a control message that
// passes a single file descriptor
const scmBufSize = 32

// SocketPair returns a pair of connected unix sockets.
func SocketPair() (*net.UnixConn, *net.UnixConn, error) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_DGRAM|syscall.SOCK_NONBLOCK, 0)
	if err != nil {
		return nil, nil, err
	}
	left, err := FD2Unix(fds[0])
	if err != nil {
		syscall.Close(fds[0])
		syscall.Close(fds[1])
		return nil, nil, err
	}
	right, err := FD2Unix(fds[1])
	if err != nil {
		left.Close()
		syscall.Close(fds[1])
		return nil, nil, err
	}
	return left, right, nil
}

func FD2Unix(fd int) (*net.UnixConn, error) {
	osf := os.NewFile(uintptr(fd), "")
	if osf == nil {
		return nil, fmt.Errorf("bad file descriptor %d", fd)
	}
	defer osf.Close() // net.FileConn will dup(2) the fd
	fc, err := net.FileConn(osf)
	if err != nil {
		return nil, err
	}
	uc, ok := fc.(*net.UnixConn)
	if !ok {
		fc.Close()
		return nil, fmt.Errorf("couldn't convert %T to net.UnixConn", fc)
	}
	return uc, nil
}

func writeWithSysconn(dst *net.UnixConn, msg []byte, rc syscall.RawConn) (int, error) {
	var reterr error
	var n int
	// capture the input file descriptor for
	// long enough that the call to sendmsg(2) completes
	err := rc.Control(func(fd uintptr) {
		oob := syscall.UnixRights(int(fd))
		n, _, reterr = dst.WriteMsgUnix(msg, oob, nil)
	})
	if err != nil {
		return 0, err
	}
	return n, reterr
}

// WriteWithFile writes a message to dst,
// including the provided file handle in an
// out-of-band control message.
func WriteWithFile(dst *net.UnixConn, msg []byte, handle *os.File) (int, error) {
	rc, err := handle.SyscallConn()
	if err != nil {
		return 0, err
	}
	return writeWithSysconn(dst, msg, rc)
}

// ReadWithFile reads data from src,
// and if it includes an out-of-band control message,
// it will try to turn it into a file handle.
func ReadWithFile(src *net.UnixConn, dst []byte) (int, *os.File, error) {
	oob := make([]byte, scmBufSize)
	n, oobn, _, _, err := src.ReadMsgUnix(dst, oob)
	if err != nil {
		return n, nil, err
	}
	oob = oob[:oobn]
	if len(oob) > 0 {
		scm, err := syscall.ParseSocketControlMessage(oob)
		if err != nil {
			return n, nil, err
		}
		if len(scm) != 1 {
			return n, nil, fmt.Errorf("%d socket control messages", len(scm))
		}
		fds, err := syscall.ParseUnixRights(&scm[0])
		if err != nil {
			return n, nil, fmt.Errorf("parsing unix rights: %s", err)
		}
		if len(fds) == 1 {
			// try to set this fd as non-blocking
			syscall.SetNonblock(fds[0], true)
			return n, os.NewFile(uintptr(fds[0]), "<socketconn>"), nil
		}
		if len(fds) > 1 {
			for i := range fds {
				syscall.Close(fds[i])
			}
			return n, nil, fmt.Errorf("control message sent %d fds", len(fds))
		}
		// fallthrough; len(fds) == 0
	}
	return n, nil, nil
}
