//go:build !windows

package main

import "os"

func replaceFile(source, destination string) error {
	return os.Rename(source, destination)
}
