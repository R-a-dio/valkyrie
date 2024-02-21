package mocks

import "io/fs"

type FS interface {
	fs.GlobFS
	fs.ReadFileFS
	fs.StatFS
	fs.SubFS
}

type File = fs.File
type FileInfo = fs.FileInfo
