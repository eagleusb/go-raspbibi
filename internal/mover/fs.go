package mover

import (
	"context"
	"fmt"
	"io"
	"os"
)

// ExistsAndMatches checks whether the destination file exists and whether
// its content matches the source. It compares file sizes first (instant),
// then falls back to checksum comparison only if sizes match.
// The algo parameter selects the checksum method: "sampling" (3MB reads) or "full" (entire file).
// It does not modify any files.
func ExistsAndMatches(src, dst, algo string) (exists bool, match bool, err error) {
	dstInfo, err := os.Stat(dst)
	if os.IsNotExist(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, err
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return true, false, err
	}

	// Size differs — files are definitely different
	if srcInfo.Size() != dstInfo.Size() {
		return true, false, nil
	}

	fmt.Printf("File exists at destination, computing checksums (%s): %s\n", algo, dst)

	var srcHash, dstHash uint32

	switch algo {
	case "full":
		srcHash, err = CRC32Sum(src)
		if err != nil {
			return true, false, err
		}
		dstHash, err = CRC32Sum(dst)
		if err != nil {
			return true, false, err
		}
	default: // "sampling"
		srcHash, err = SampleChecksum(src)
		if err != nil {
			return true, false, err
		}
		dstHash, err = SampleChecksum(dst)
		if err != nil {
			return true, false, err
		}
	}

	return true, srcHash == dstHash, nil
}

// progressWriter wraps an io.Writer and reports copy progress at 25% intervals.
// It checks ctx.Err() on every Write to abort mid-copy on signal interrupt.
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
	pct := pw.written * 100 / pw.total
	if pct >= pw.lastPct+25 {
		fmt.Printf("  Progress: %d%% (%d / %d MB)\n", pct, pw.written/1024/1024, pw.total/1024/1024)
		pw.lastPct = pct
	}
	return n, err
}

// MoveFile handles cross-device moves (copy + delete) since os.Rename fails across partitions.
// It first attempts an atomic rename, then falls back to copy + delete with progress reporting.
// On error or context cancellation, any partial destination file is removed.
func MoveFile(ctx context.Context, src, dst string) error {
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

	info, err := srcFile.Stat()
	if err != nil {
		return err
	}

	pw := &progressWriter{ctx: ctx, underlying: dstFile, total: info.Size()}
	buf := make([]byte, 32*1024)
	if _, err := io.CopyBuffer(pw, srcFile, buf); err != nil {
		dstFile.Close()
		os.Remove(dst)
		return fmt.Errorf("copy failed, removed partial file %s: %w", dst, err)
	}

	dstFile.Sync()
	return os.Remove(src)
}
