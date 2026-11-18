package mover

import (
	"bytes"
	"context"
	"fmt"
	"hash/crc32"
	"io"
	"os"
)

const sampleSize = 1024 * 1024

type contextReader struct {
	ctx context.Context
	io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(p)
}

func checksum(ctx context.Context, file *regularFile, algo string) (uint32, error) {
	h := crc32.NewIEEE()
	size := file.info.Size()
	if algo == "full" {
		_, err := io.CopyN(h, contextReader{ctx, io.NewSectionReader(file, 0, size)}, size)
		return h.Sum32(), err
	}

	offsets := []int64{0}
	if size > 2*sampleSize {
		offsets = append(offsets, size/2)
	}
	if size > sampleSize {
		offsets = append(offsets, size-sampleSize)
	}
	for _, offset := range offsets {
		length := min(sampleSize, size-offset)
		if _, err := io.CopyN(h, contextReader{ctx, io.NewSectionReader(file, offset, length)}, length); err != nil {
			return 0, err
		}
	}
	return h.Sum32(), nil
}

func filesMatch(ctx context.Context, src, dst *regularFile, algo string) (bool, error) {
	if os.SameFile(src.info, dst.info) {
		return false, fmt.Errorf("source and destination are the same file: %q", dst.Name())
	}
	size := src.info.Size()
	if size != dst.info.Size() {
		return false, nil
	}
	left, err := checksum(ctx, src, algo)
	if err != nil {
		return false, err
	}
	right, err := checksum(ctx, dst, algo)
	if err != nil || left != right {
		return false, err
	}

	// A checksum match only qualifies the files for an exact comparison.
	srcBytes := make([]byte, 64*1024)
	dstBytes := make([]byte, len(srcBytes))
	for offset := int64(0); offset < size; {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		length := min(int64(len(srcBytes)), size-offset)
		if _, err := src.ReadAt(srcBytes[:length], offset); err != nil {
			return false, err
		}
		if _, err := dst.ReadAt(dstBytes[:length], offset); err != nil {
			return false, err
		}
		if !bytes.Equal(srcBytes[:length], dstBytes[:length]) {
			return false, nil
		}
		offset += length
	}
	return true, nil
}
