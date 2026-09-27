package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Trash-based delete with undo. Never rm -rf.

type pendingUndo struct {
	Name      string
	TrashPath string
	Active    bool
	ExpiresAt time.Time
}

func deleteSkillToTrash(s Skill) (string, error) {
	td := trashDir()
	if err := os.MkdirAll(td, 0o755); err != nil {
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
	dest := filepath.Join(skillsDir(), name)
	// The name exists again (re-created since the delete): restoring
	// would need a rename that breaks dirname==frontmatter or an
	// overwrite that destroys the new skill — refuse instead (B14).
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("%s exists again — not restoring (left in trash at %s)", name, trashPath)
	}
	if err := movePath(trashPath, dest); err != nil {
		return err
	}
	logf("undo %s <- %s", name, trashPath)
	return nil
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
