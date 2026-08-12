//go:build !windows

package main

import (
	"os"
	"syscall"
)

func getFileInode(fileInfo os.FileInfo) uint64 {
	if fileInfo == nil {
		return 0
	}
	if sys, ok := fileInfo.Sys().(*syscall.Stat_t); ok {
		return uint64(sys.Ino)
	}
	return 0
}
