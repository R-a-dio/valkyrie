package mocks

import (
	"crypto/rand"
	"errors"
	"io/fs"
	"math"
	"time"
)

type RandomFile struct {
	name string
}
type randomFileInfo struct{ name string }

func (rfi *randomFileInfo) Name() string {
	return rfi.name
}

func (rfi *randomFileInfo) Size() int64 {
	return time.Now().UnixMilli()
}

func (rfi *randomFileInfo) Mode() fs.FileMode {
	return fs.FileMode(time.Now().UnixMilli() % math.MaxUint32)
}

func (rfi *randomFileInfo) ModTime() time.Time {
	return time.Now()
}

func (rfi *randomFileInfo) IsDir() bool {
	return time.Now().UnixMilli()%2 == 0
}

func (rfi *randomFileInfo) Sys() any {
	return nil
}

func (rf *RandomFile) Stat() (fs.FileInfo, error) {
	return new(randomFileInfo), nil
}

func (rf *RandomFile) Read(b []byte) (int, error) {
	return rand.Read(b)
}

func (rf *RandomFile) Close() error {
	return nil
}

type RandomFS struct{ Name string }

func (rfs *RandomFS) Open(name string) (fs.File, error) {
	return &RandomFile{rfs.Name}, nil
}

type ErrorFS struct{}

func (efs *ErrorFS) Open(name string) (fs.File, error) {
	return nil, errors.New("fucked")
}
