//go:build windows

package main

import "os"

func getFileInode(fileInfo os.FileInfo) uint64 {
	if fileInfo == nil {
		return 0
	}
	return 0
}
