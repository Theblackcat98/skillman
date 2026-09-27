package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Trash-based delete with undo. Never rm -rf.

// pendingUndo is the in-memory undo window. It carries no deadline: the
// timer is the deadline, and a second field that is set but never read is
// a second source of truth about when the window closes (review F1).
type pendingUndo struct {
	Name      string
	TrashPath string
	Active    bool
}

// TrashEntry is one deleted skill sitting in the trash.
type TrashEntry struct {
	Name    string    `json:"name"`
	Slug    string    `json:"slug"`
	Path    string    `json:"path"`
	Deleted time.Time `json:"deleted"`
}

// trashSlug matches the <name>-<timestamp>[-<n>] directory names that
// deleteSkillToTrash creates. Skill names may themselves contain dashes,
// so the timestamp is the fixed part that anchors the match.
var trashSlug = regexp.MustCompile(`^(.+)-\d{8}-\d{6}(-\d+)?$`)

func deleteSkillToTrash(s Skill) (string, error) {
	td := trashDir()
	// 0700: the trash holds whole skills, which may be private. The old
	// 0755 relied on the caller's umask to protect them.
	if err := os.MkdirAll(td, 0o700); err != nil {
		return "", err
	}
	// Unique destination even for two deletes in the same second (B13).
	ts := time.Now().Format("20060102-150405")
	base := fmt.Sprintf("%s-%s", s.Name, ts)
	dest := filepath.Join(td, base)
	for i := 1; ; i++ {
		if _, err := os.Stat(dest); err != nil {
			break
		}
		dest = filepath.Join(td, fmt.Sprintf("%s-%d", base, i))
	}
	if err := movePath(s.Dir, dest); err != nil {
		return "", err
	}
	logf("delete %s -> %s", s.Name, dest)
	return dest, nil
}

func undoDelete(name, trashPath string) error {
	if err := restoreTo(trashPath, name); err != nil {
		return err
	}
	logf("undo %s <- %s", name, trashPath)
	return nil
}

// restoreTo moves a trashed skill back to <skillsDir>/<name>. If the
// name exists again, restoring would need either a rename that breaks
// dirname == frontmatter or an overwrite that destroys the new skill, so
// it refuses and leaves the copy in the trash (audit B14).
func restoreTo(trashPath, name string) error {
	dest := filepath.Join(skillsDir(), name)
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("%s exists again — not restoring (left in trash at %s)", name, trashPath)
	}
	return movePath(trashPath, dest)
}

// listTrash returns everything in the trash, newest first. A missing
// trash dir is an empty trash, not an error.
func listTrash() ([]TrashEntry, error) {
	entries, err := os.ReadDir(trashDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []TrashEntry
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		name := e.Name()
		if m := trashSlug.FindStringSubmatch(name); m != nil {
			name = m[1]
		}
		entry := TrashEntry{Name: name, Slug: e.Name(), Path: filepath.Join(trashDir(), e.Name())}
		if info, ierr := e.Info(); ierr == nil {
			entry.Deleted = info.ModTime()
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Deleted.After(out[j].Deleted) })
	return out, nil
}

// restoreFromTrash puts a trashed skill back. name may be the original
// skill name or the trash directory name; the newest match wins.
func restoreFromTrash(name string) (string, error) {
	entries, err := listTrash()
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("trash is empty")
	}
	pick := func(match func(TrashEntry) bool) (TrashEntry, bool) {
		for _, e := range entries { // newest first
			if match(e) {
				return e, true
			}
		}
		return TrashEntry{}, false
	}
	entry, ok := pick(func(e TrashEntry) bool { return e.Slug == name })
	if !ok {
		entry, ok = pick(func(e TrashEntry) bool { return e.Name == name })
	}
	if !ok {
		return "", fmt.Errorf("not in trash: %s", name)
	}
	if err := restoreTo(entry.Path, entry.Name); err != nil {
		return "", err
	}
	logf("restore %s <- %s", entry.Name, entry.Path)
	return entry.Name, nil
}

// purgeTrash removes trashed skills. A non-zero maxAge keeps anything
// newer; zero purges everything and is only reachable behind --all.
func purgeTrash(maxAge time.Duration) ([]string, error) {
	entries, err := listTrash()
	if err != nil {
		return nil, err
	}
	cutoff := time.Now().Add(-maxAge)
	var purged []string
	for _, e := range entries {
		if maxAge > 0 && e.Deleted.After(cutoff) {
			continue
		}
		if err := os.RemoveAll(e.Path); err != nil {
			return purged, fmt.Errorf("purge %s: %w", e.Slug, err)
		}
		purged = append(purged, e.Slug)
	}
	if len(purged) > 0 {
		logf("purge %s", strings.Join(purged, " "))
	}
	return purged, nil
}

// parseAge reads a Go duration ("720h") or a day count ("30d").
func parseAge(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("empty age")
	}
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil || n < 0 {
			return 0, fmt.Errorf("bad age: %s", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("bad age: %s", s)
	}
	return d, nil
}

// movePath renames src to dst, falling back to copy+remove across
// filesystems: os.Rename fails with EXDEV when the skills dir and the
// data dir sit on different mounts (plausible on Termux/storage) (B13).
func movePath(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	if !errors.Is(err, syscall.EXDEV) {
		return err
	}
	if cerr := copyDir(src, dst); cerr != nil {
		_ = os.RemoveAll(dst)
		return fmt.Errorf("move failed (%v) and copy failed (%v)", err, cerr)
	}
	return os.RemoveAll(src)
}

// copyDir recursively copies src into dst preserving permission bits
// and symlinks.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, path)
		if rerr != nil {
			return rerr
		}
		target := filepath.Join(dst, rel)
		switch {
		case info.IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		case info.Mode()&os.ModeSymlink != 0:
			link, lerr := os.Readlink(path)
			if lerr != nil {
				return lerr
			}
			return os.Symlink(link, target)
		default:
			in, oerr := os.Open(path)
			if oerr != nil {
				return oerr
			}
			defer in.Close()
			out, werr := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
			if werr != nil {
				return werr
			}
			defer out.Close()
			_, cerr := io.Copy(out, in)
			return cerr
		}
	})
}
