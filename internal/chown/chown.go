package chown

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// Config holds the configuration for a chown run.
type Config struct {
	Path   string
	UID    int
	GID    int
	DryRun bool
}

// Run reconciles ownership and shared access under cfg.Path.
func Run(ctx context.Context, cfg Config) error {
	if cfg.Path == "" {
		return fmt.Errorf("target directory is required")
	}
	// A trailing slash would make the kernel follow a symlink at the root.
	path := strings.TrimRight(cfg.Path, "/")
	if path == "" {
		path = "/"
	}
	root, err := unix.Open(path, unix.O_PATH|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open target directory %q: %w", path, err)
	}
	defer unix.Close(root)
	return walk(ctx, cfg, root, path)
}

func walk(ctx context.Context, cfg Config, fd int, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return fmt.Errorf("stat %q: %w", path, err)
	}
	if err := ensureSharedAccess(cfg, fd, path, stat); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return nil
	}

	// Paths are for display only. Every child is opened relative to its pinned parent.
	dirFD, err := unix.Openat(fd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open directory %q: %w", path, err)
	}
	dir := os.NewFile(uintptr(dirFD), path)
	names, err := dir.Readdirnames(-1)
	dir.Close()
	if err != nil {
		return fmt.Errorf("read directory %q: %w", path, err)
	}
	slices.Sort(names)
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		childPath := filepath.Join(path, name)
		child, err := unix.Openat(fd, name, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if errors.Is(err, unix.ENOENT) {
			continue // A download can be moved while the daily scan is running.
		}
		if err != nil {
			return fmt.Errorf("open %q: %w", childPath, err)
		}
		err = walk(ctx, cfg, child, childPath)
		unix.Close(child)
		if err != nil {
			return err
		}
	}
	return nil
}

func ensureSharedAccess(cfg Config, fd int, path string, stat unix.Stat_t) error {
	const modeBits = 07777
	var required uint32
	switch stat.Mode & unix.S_IFMT {
	case unix.S_IFDIR:
		required = 0770 | unix.S_ISGID
	case unix.S_IFREG:
		required = 0660
	}
	mode := uint32(stat.Mode) & modeBits
	changeOwner := int(stat.Uid) != cfg.UID || int(stat.Gid) != cfg.GID
	if cfg.DryRun {
		if changeOwner {
			fmt.Printf("[DRY-RUN] Would chown: %s (%d:%d)\n", path, cfg.UID, cfg.GID)
		}
		if mode&required != required {
			fmt.Printf("[DRY-RUN] Would chmod: %s (%04o)\n", path, mode|required)
		}
		return nil
	}
	if changeOwner {
		if err := unix.Fchownat(fd, "", cfg.UID, cfg.GID, unix.AT_EMPTY_PATH|unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return fmt.Errorf("chown %q: %w", path, err)
		}
		fmt.Printf("Chown: %s (%d:%d)\n", path, cfg.UID, cfg.GID)
		// Use the post-chown mode because changing ownership can clear mode bits.
		if err := unix.Fstat(fd, &stat); err != nil {
			return fmt.Errorf("stat %q after chown: %w", path, err)
		}
		mode = uint32(stat.Mode) & modeBits
	}
	if mode&required == required {
		return nil
	}
	if err := chmodEntry(fd, mode|required); err != nil {
		return fmt.Errorf("chmod %q: %w", path, err)
	}
	if err := unix.Fstat(fd, &stat); err != nil {
		return fmt.Errorf("stat %q after chmod: %w", path, err)
	}
	if uint32(stat.Mode)&required != required {
		return fmt.Errorf("%q could not retain required permissions %04o", path, required)
	}
	fmt.Printf("Chmod: %s (%04o)\n", path, uint32(stat.Mode)&modeBits)
	return nil
}

func chmodEntry(fd int, mode uint32) error {
	err := unix.Fchmodat(fd, "", mode, unix.AT_EMPTY_PATH)
	if errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.EINVAL) {
		// Older kernels resolve this magic link to the same pinned inode.
		return unix.Chmod("/proc/self/fd/"+strconv.Itoa(fd), mode)
	}
	return err
}
