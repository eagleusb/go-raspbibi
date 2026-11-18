package mover

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func writeFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}

func expectContents(t *testing.T, path string, want []byte) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, want) {
		t.Fatalf("unexpected contents at %s: read error %v", path, err)
	}
}

func TestSamplingMatchRequiresEqualContents(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	source := filepath.Join(src, "movie.MKV")
	destination := filepath.Join(dst, "movie.mkv")
	data := make([]byte, 5*sampleSize)
	writeFile(t, destination, data, 0644)
	data[sampleSize+1] = 1 // Outside the three sample regions.
	writeFile(t, source, data, 0644)
	if err := Run(t.Context(), Config{Src: src, Dst: dst, Algo: "sampling"}); err != nil {
		t.Fatal(err)
	}
	expectContents(t, destination, data)
	if _, err := os.Lstat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source should have been moved: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dst, "movie.MKV.mkv")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("normalized extension was duplicated: %v", err)
	}
}

func TestIdenticalFilesKeepTheDestination(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	source, destination := filepath.Join(src, "movie.mkv"), filepath.Join(dst, "movie.mkv")
	data := []byte("same contents")
	writeFile(t, source, data, 0600)
	writeFile(t, destination, data, 0644)
	before, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if err := Run(t.Context(), Config{Src: src, Dst: dst, Algo: "full"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected duplicate source removal: %v", err)
	}
	after, err := os.Stat(destination)
	if err != nil || !os.SameFile(before, after) {
		t.Fatalf("destination should retain its original inode: %v", err)
	}
}

func TestRunRejectsAliasesAndNonRegularFiles(t *testing.T) {
	for _, kind := range []string{"source symlink", "source fifo", "destination symlink", "destination directory", "hardlink", "same directory"} {
		t.Run(kind, func(t *testing.T) {
			src, dst := t.TempDir(), t.TempDir()
			source, destination := filepath.Join(src, "movie.mkv"), filepath.Join(dst, "movie.mkv")
			data := []byte("original content")
			var err error
			switch kind {
			case "source symlink":
				target := filepath.Join(t.TempDir(), "private")
				writeFile(t, target, data, 0600)
				err = os.Symlink(target, source)
			case "source fifo":
				err = unix.Mkfifo(source, 0600)
			default:
				writeFile(t, source, data, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "destination symlink":
				err = os.Symlink(source, destination)
			case "destination directory":
				err = os.Mkdir(destination, 0755)
			case "hardlink":
				err = os.Link(source, destination)
			case "same directory":
				err = os.Remove(dst)
				if err == nil {
					err = os.Symlink(src, dst)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := Run(t.Context(), Config{Src: src, Dst: dst, Algo: "sampling"}); err == nil {
				t.Fatal("expected the mover to reject the unsafe file")
			}
			if _, err := os.Lstat(source); err != nil {
				t.Fatalf("source should remain: %v", err)
			}
			if strings.HasPrefix(kind, "source ") {
				if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("unexpected destination: %v", err)
				}
			} else {
				expectContents(t, source, data)
			}
		})
	}
}

func TestCrossDeviceCopyPublishesOnlyCompleteFiles(t *testing.T) {
	for _, outcome := range []string{"success", "cancelled", "read failure"} {
		t.Run(outcome, func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "movie.mkv")
			dir := t.TempDir()
			destination := filepath.Join(dir, "movie.mkv")
			data, old := []byte("source data"), []byte("previous destination")
			writeFile(t, source, data, 0600)
			writeFile(t, destination, old, 0644)
			src, err := openRegular(source)
			if err != nil {
				t.Fatal(err)
			}
			defer src.Close()
			dstInfo, err := os.Lstat(destination)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch outcome {
			case "cancelled":
				cancel()
			case "read failure":
				if err := src.Close(); err != nil {
					t.Fatal(err)
				}
			}
			err = copyAcrossDevices(ctx, src, destination, dstInfo)
			if outcome == "success" {
				if err != nil {
					t.Fatal(err)
				}
				expectContents(t, destination, data)
				info, err := os.Stat(destination)
				if err != nil || info.Mode().Perm() != 0600 {
					t.Fatalf("private destination permissions must be preserved: %v, %v", info, err)
				}
				if _, err := os.Lstat(source); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("source should have been removed: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected copy failure")
				}
				expectContents(t, source, data)
				expectContents(t, destination, old)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("unexpected staging files left behind: %v, %v", entries, err)
			}
		})
	}
}

func TestMoveReturnsNonEXDEVRenameError(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "movie.mkv")
	writeFile(t, source, []byte("source data"), 0600)
	src, err := openRegular(source)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	var renameErr *os.LinkError
	err = moveFile(t.Context(), src, filepath.Join(dir, "missing", "movie.mkv"), nil)
	if !errors.As(err, &renameErr) || renameErr.Op != "rename" || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected the original rename error, got %v", err)
	}
	expectContents(t, source, []byte("source data"))
}

func TestUnreadableDestinationIsNotOverwritten(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read mode 000 files")
	}
	src, dst := t.TempDir(), t.TempDir()
	source, destination := filepath.Join(src, "movie.mkv"), filepath.Join(dst, "movie.mkv")
	data, old := []byte("source"), []byte("target")
	writeFile(t, source, data, 0600)
	writeFile(t, destination, old, 0000)
	if err := Run(t.Context(), Config{Src: src, Dst: dst, Algo: "full"}); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected destination access error, got %v", err)
	}
	if err := os.Chmod(destination, 0600); err != nil {
		t.Fatal(err)
	}
	expectContents(t, source, data)
	expectContents(t, destination, old)
}
