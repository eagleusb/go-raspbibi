package mover

import (
	"hash"
	"hash/crc32"
	"io"
	"os"
)

const sampleSize = 1024 * 1024 // 1MB per sample

// CRC32Sum computes the CRC-32 checksum of the file at path by streaming.
// Returns the checksum as a uint32.
func CRC32Sum(path string) (uint32, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	h := crc32.NewIEEE()
	if _, err := io.Copy(h, f); err != nil {
		return 0, err
	}
	return h.Sum32(), nil
}

// SampleChecksum reads three 1MB samples (start, middle, end) from the file
// and computes a CRC-32 over them. Fast fingerprint without reading the entire file.
func SampleChecksum(path string) (uint32, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}

	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	h := crc32.NewIEEE()
	buf := make([]byte, sampleSize)

	// Sample 1: start
	readSample(f, buf, h)

	// Sample 2: middle
	if info.Size() > 2*sampleSize {
		f.Seek(info.Size()/2, io.SeekStart)
		readSample(f, buf, h)
	}

	// Sample 3: end
	if info.Size() > sampleSize {
		f.Seek(info.Size()-sampleSize, io.SeekStart)
		readSample(f, buf, h)
	}

	return h.Sum32(), nil
}

func readSample(f *os.File, buf []byte, h hash.Hash32) {
	n, _ := f.Read(buf)
	if n > 0 {
		h.Write(buf[:n])
	}
}
