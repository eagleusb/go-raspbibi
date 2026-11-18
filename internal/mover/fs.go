package mover

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

type regularFile struct {
	*os.File
	info os.FileInfo
}

func openRegular(path string) (*regularFile, error) {
	entry, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !entry.Mode().IsRegular() {
		return nil, fmt.Errorf("%q is not a regular file", path)
	}
	// A replacement symlink must not be followed, and a replacement FIFO must not block.
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	f := os.NewFile(uintptr(fd), path)
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() || !os.SameFile(entry, info) {
		f.Close()
		return nil, fmt.Errorf("%q changed while opening it", path)
	}
	return &regularFile{File: f, info: info}, nil
}

// Check the directory entry again before replacing or removing it. A nil
// original denotes a destination that was absent during comparison.
func checkUnchanged(path string, original os.FileInfo) error {
	info, err := os.Lstat(path)
	if original == nil && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if original == nil || !os.SameFile(original, info) || info.Mode() != original.Mode() ||
		info.Size() != original.Size() || !info.ModTime().Equal(original.ModTime()) {
		return fmt.Errorf("%q changed during processing", path)
	}
	return nil
}

type progressWriter struct {
	ctx        context.Context
	underlying io.Writer
	total      int64
	written    int64
	lastPct    int64
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	if err := pw.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := pw.underlying.Write(p)
	pw.written += int64(n)
	if pw.total > 0 {
		pct := pw.written * 100 / pw.total
		if pct >= pw.lastPct+25 {
			fmt.Printf("  Progress: %d%% (%d / %d MB)\n", pct, pw.written/1024/1024, pw.total/1024/1024)
			pw.lastPct = pct
		}
	}
	return n, err
}

func moveFile(ctx context.Context, src *regularFile, dst string, dstInfo os.FileInfo) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := checkUnchanged(src.Name(), src.info); err != nil {
		return err
	}
	if err := checkUnchanged(dst, dstInfo); err != nil {
		return err
	}
	if err := os.Rename(src.Name(), dst); !errors.Is(err, unix.EXDEV) {
		return err
	}
	fmt.Printf("Cross-device move, falling back to copy+delete: %s -> %s\n", src.Name(), dst)
	return copyAcrossDevices(ctx, src, dst, dstInfo)
}

func copyAcrossDevices(ctx context.Context, src *regularFile, dst string, dstInfo os.FileInfo) error {
	dir, err := os.Open(filepath.Dir(dst))
	if err != nil {
		return err
	}
	defer dir.Close()
	temp, err := os.CreateTemp(dir.Name(), ".raspbibi-*")
	if err != nil {
		return err
	}
	published := false
	defer func() {
		temp.Close()
		if !published {
			os.Remove(temp.Name())
		}
	}()

	writer := &progressWriter{ctx: ctx, underlying: temp, total: src.info.Size()}
	if _, err := io.CopyN(writer, io.NewSectionReader(src, 0, src.info.Size()), src.info.Size()); err != nil {
		return fmt.Errorf("copy to temporary file: %w", err)
	}
	if err := temp.Chmod(src.info.Mode().Perm()); err != nil {
		return fmt.Errorf("set copied file permissions: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync copied file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close copied file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := checkUnchanged(src.Name(), src.info); err != nil {
		return err
	}
	if err := checkUnchanged(dst, dstInfo); err != nil {
		return err
	}
	if err := os.Rename(temp.Name(), dst); err != nil {
		return err
	}
	published = true
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync destination directory: %w", err)
	}
	if err := checkUnchanged(src.Name(), src.info); err != nil {
		return err
	}
	return unix.Unlink(src.Name())
}
