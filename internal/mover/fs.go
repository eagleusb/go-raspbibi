package mover

import (
	"fmt"
	"io"
	"os"
)

// MoveFile handles cross-device moves (copy + delete) since os.Rename fails across partitions.
// It first attempts an atomic rename, then falls back to copy + delete.
func MoveFile(src, dst string) error {
	// Try atomic rename first
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	fmt.Printf("Cross-device move, falling back to copy+delete: %s -> %s\n", src, dst)

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	dstFile.Sync()
	return os.Remove(src)
}
