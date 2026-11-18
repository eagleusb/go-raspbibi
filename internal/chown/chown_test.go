package chown

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestRunTreatsSymlinksAsEntries(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink("missing", filepath.Join(root, "dangling")); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Path: root, UID: os.Geteuid(), GID: os.Getegid(), DryRun: true}
	if err := Run(t.Context(), cfg); err != nil {
		t.Fatalf("dangling symlink should be inspected as a leaf: %v", err)
	}
	// Exercise the descriptor-based ownership call without needing another mapped group.
	cfg.GID = -1
	cfg.DryRun = false
	if err := Run(t.Context(), cfg); err != nil {
		t.Fatalf("chown should use the dangling link's own descriptor: %v", err)
	}
	link := filepath.Join(t.TempDir(), "linked-root")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	cfg.Path = link + "/"
	if err := Run(t.Context(), cfg); err == nil {
		t.Fatal("expected symlink root to be rejected")
	}
}

func TestRunChangesLinkOwnershipWithoutChangingTarget(t *testing.T) {
	groups, err := os.Getgroups()
	if err != nil {
		t.Fatal(err)
	}
	gid := -1
	for _, group := range groups {
		if group != os.Getegid() {
			gid = group
			break
		}
	}
	if gid == -1 {
		t.Skip("requires a supplementary group other than the effective group")
	}
	root := t.TempDir()
	if err := os.Chown(root, -1, gid); err != nil {
		t.Skipf("cannot change group in this environment: %v", err)
	}
	target := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(target, nil, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	var before unix.Stat_t
	if err := unix.Lstat(target, &before); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Path: root, UID: os.Geteuid(), GID: gid}
	if err := Run(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	var linkStat, targetStat unix.Stat_t
	if err := unix.Lstat(link, &linkStat); err != nil {
		t.Fatal(err)
	}
	if err := unix.Lstat(target, &targetStat); err != nil {
		t.Fatal(err)
	}
	if int(linkStat.Gid) != gid || targetStat.Gid != before.Gid {
		t.Fatalf("link group = %d, target group = %d, want %d and %d", linkStat.Gid, targetStat.Gid, gid, before.Gid)
	}
}

func TestWalkUsesPinnedDirectory(t *testing.T) {
	parent := t.TempDir()
	path := filepath.Join(parent, "selected")
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "media"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	fd, err := unix.Open(path, unix.O_PATH|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	if err := os.Rename(path, filepath.Join(parent, "renamed")); err != nil {
		t.Fatal(err)
	}
	cfg := Config{UID: os.Geteuid(), GID: os.Getegid(), DryRun: true}
	if err := walk(t.Context(), cfg, fd, path); err != nil {
		t.Fatalf("walk should use the pinned directory: %v", err)
	}
}

func TestRunEnsuresSharedPermissions(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	file := filepath.Join(nested, "movie.mkv")
	executable := filepath.Join(nested, "script")
	if err := os.Mkdir(nested, 0750); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{file, executable} {
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(target, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(nested, "link")); err != nil {
		t.Fatal(err)
	}

	files := []struct {
		path          string
		initial, want uint32
	}{
		{root, 0711, 02771},
		{nested, 02750, 02770},
		{file, 0404, 0664},
		{executable, 0700, 0760},
		{target, 0600, 0600},
	}
	for _, entry := range files {
		if err := unix.Chmod(entry.path, entry.initial); err != nil {
			t.Fatal(err)
		}
	}
	cfg := Config{Path: root, UID: os.Geteuid(), GID: os.Getegid(), DryRun: true}
	for _, dryRun := range []bool{true, false, false} {
		cfg.DryRun = dryRun
		if err := Run(t.Context(), cfg); err != nil {
			t.Fatal(err)
		}
		for _, entry := range files {
			var stat unix.Stat_t
			if err := unix.Lstat(entry.path, &stat); err != nil {
				t.Fatal(err)
			}
			want := entry.want
			if dryRun {
				want = entry.initial
			}
			if got := uint32(stat.Mode) & 07777; got != want {
				t.Errorf("dry-run %v, mode of %q = %04o, want %04o", dryRun, entry.path, got, want)
			}
		}
	}
	newDir := filepath.Join(nested, "new-season")
	if err := os.Mkdir(newDir, 0750); err != nil {
		t.Fatal(err)
	}
	var stat unix.Stat_t
	if err := unix.Lstat(newDir, &stat); err != nil {
		t.Fatal(err)
	}
	if int(stat.Gid) != cfg.GID || stat.Mode&unix.S_ISGID == 0 {
		t.Fatalf("new directory group = %d, mode = %o, expected group %d and setgid", stat.Gid, stat.Mode, cfg.GID)
	}
}
