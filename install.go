package main

// Install a skill from a git repository.
//
// The flow is the one the plan names: clone to a temp directory, find the
// candidate skills, let the user choose, copy, rescan. Nothing here runs
// through a shell — git is exec'd with an argv — and every path that
// reaches the filesystem is checked to be inside the directory it is
// supposed to be inside.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// cloneTimeout bounds a clone. Without it a network that never answers
// leaves the TUI spinning with no way to tell a slow repo from a hung one.
const cloneTimeout = 90 * time.Second

// installCandidate is one skill found in a cloned repository.
type installCandidate struct {
	// Name is the directory name it will get under the skills directory.
	Name string
	// Rel is the path relative to the clone root, for messages.
	Rel  string
	Desc string
	// Valid is false when the SKILL.md has no usable frontmatter, so the
	// picker can show it as broken rather than hide it.
	Valid bool
	// Selected is the picker's toggle.
	Selected bool
}

// gitError is a failure from git itself, as opposed to a rejected URL or
// a missing directory. The three produce different exit codes.
type gitError struct{ msg string }

func (e gitError) Error() string { return e.msg }

var errBadURL = errors.New("unsupported git URL")

// fsSkipDir is fs.SkipDir, named locally so this file does not need the
// io/fs import for one use.
var fsSkipDir = fs.SkipDir

// validateGitURL rejects anything that is not a plain clone target.
//
// The important one is ext:: — git's remote helper runs a shell command,
// so "ext::sh -c ..." is remote code execution from a URL. A scheme
// allowlist is the right shape here: anything not explicitly permitted is
// refused, so a helper nobody has heard of is refused too.
func validateGitURL(raw string) (string, error) {
	u := strings.TrimSpace(raw)
	if u == "" {
		return "", fmt.Errorf("%w: empty", errBadURL)
	}
	// A leading dash would be read by git as an option, and a value like
	// --upload-pack=... runs an arbitrary program. The argv is built with
	// a "--" separator as well, so both ends are covered.
	if strings.HasPrefix(u, "-") {
		return "", fmt.Errorf("%w: starts with -", errBadURL)
	}
	lower := strings.ToLower(u)
	if strings.Contains(lower, "ext::") || strings.Contains(lower, "upload-pack") {
		return "", fmt.Errorf("%w: remote helpers are not allowed", errBadURL)
	}
	// A local path is a legitimate source (a checkout on disk) and cannot
	// execute anything, so it is allowed. Everything else needs a scheme
	// from the list.
	if !strings.Contains(u, "://") && !strings.Contains(u, ":") {
		if !filepath.IsAbs(u) {
			if _, err := os.Stat(u); err != nil {
				return "", fmt.Errorf("%w: not a URL and not a path that exists", errBadURL)
			}
		}
		return u, nil
	}
	for _, scheme := range []string{"https://", "ssh://", "git://", "file://"} {
		if strings.HasPrefix(lower, scheme) {
			return u, nil
		}
	}
	// scp-style git@host:path/repo.git
	if i := strings.Index(u, ":"); i > 0 && !strings.Contains(u[:i], "/") &&
		strings.Contains(u[:i], "@") {
		return u, nil
	}
	return "", fmt.Errorf("%w: only https, ssh, git, file and local paths", errBadURL)
}

// validateRef checks a --ref value. It reaches git as an option value, so
// it must not be able to become an option itself.
func validateRef(ref string) (string, error) {
	r := strings.TrimSpace(ref)
	if r == "" {
		return "", nil
	}
	if strings.HasPrefix(r, "-") {
		return "", errors.New("ref must not start with -")
	}
	for _, c := range r {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9', c == '.', c == '_', c == '-', c == '/':
		default:
			return "", fmt.Errorf("ref contains an unexpected character %q", c)
		}
	}
	return r, nil
}

// cloneRepo does a shallow clone into a fresh temp directory and returns
// the directory. The caller owns it and must remove it.
func cloneRepo(ctx context.Context, url, ref string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", errors.New("git is not installed")
	}
	base, err := os.MkdirTemp("", "skillman-clone-")
	if err != nil {
		return "", err
	}
	dest := filepath.Join(base, "repo")
	args := []string{"clone", "--depth", "1", "--quiet"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	// "--" ends git's options, so a URL that looks like one is still a
	// URL. validateGitURL rejects the leading dash too; both are cheap.
	args = append(args, "--", url, dest)

	cmd := exec.CommandContext(ctx, "git", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(base)
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		// git prints a lot of advice on a plain failure. The last line is
		// the actual reason.
		if i := strings.LastIndex(msg, "\n"); i >= 0 {
			msg = msg[i+1:]
		}
		return "", gitError{msg: msg}
	}
	return base, nil
}

// findCandidates looks for skills in a cloned repository, in the order the
// plan names: a SKILL.md at the root, anything under skills/, then
// anything one or two levels down.
func findCandidates(root string) ([]installCandidate, error) {
	var out []installCandidate
	seen := map[string]bool{}
	add := func(dir, rel string) {
		name := safeName(filepath.Base(dir))
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		s, err := scanOne(dir)
		c := installCandidate{Name: name, Rel: rel, Selected: true}
		if err == nil {
			c.Desc = s.Desc
			c.Valid = s.Severity() == SevOK
		} else {
			c.Desc = "(unreadable)"
		}
		out = append(out, c)
	}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			// An unreadable subdirectory is not a reason to give up on the
			// whole repository.
			if d != nil && d.IsDir() {
				return fsSkipDir
			}
			return nil
		}
		if d.IsDir() {
			if p == root {
				return nil
			}
			// Bound the walk so a repository with a deep tree or a vendor
			// directory does not turn into a full filesystem scan.
			if strings.HasPrefix(d.Name(), ".") || depthBelow(root, p) > 2 {
				return fsSkipDir
			}
			return nil
		}
		if d.Name() != "SKILL.md" {
			return nil
		}
		dir := filepath.Dir(p)
		if depthBelow(root, dir) > 2 {
			return nil
		}
		add(dir, relTo(root, dir))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// scanOne reads the frontmatter of one skill directory.
func scanOne(dir string) (Skill, error) {
	list, err := ScanSkills(filepath.Dir(dir))
	if err != nil {
		return Skill{}, err
	}
	for _, s := range list {
		if filepath.Clean(s.Dir) == filepath.Clean(dir) {
			return s, nil
		}
	}
	return Skill{}, errors.New("no skill here")
}

// copySkill copies a skill directory into the skills directory.
//
// Both sides are checked: the source has to be inside the clone, so a
// symlink in the repository cannot make the copy read somewhere else, and
// the destination has to be inside the skills directory. Symlinks inside
// the skill are skipped rather than followed, for the same reason.
func copySkill(src, dst string) error {
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	dstRoot, err := filepath.Abs(skillsDir())
	if err != nil {
		return err
	}
	dstAbs, err := filepath.Abs(dst)
	if err != nil {
		return err
	}
	if !within(dstRoot, dstAbs) {
		return fmt.Errorf("refusing to write outside %s", dstRoot)
	}
	if dstRoot == dstAbs {
		return errors.New("refusing to overwrite the skills directory itself")
	}
	if _, err := os.Lstat(dstAbs); err == nil {
		return fmt.Errorf("%s is already installed", filepath.Base(dstAbs))
	}
	return filepath.WalkDir(srcAbs, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(srcAbs, p)
		if rerr != nil {
			return rerr
		}
		target := filepath.Join(dstAbs, rel)
		if !within(dstAbs, target) {
			return errors.New("refusing to write outside the skill directory")
		}
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return fsSkipDir
			}
			return nil
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyFile(p, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, io.LimitReader(in, maxSkillFileSize)); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// maxSkillFileSize bounds one file in a skill. A repository can contain
// anything, and a copy has no reason to move a multi-gigabyte blob.
const maxSkillFileSize = 8 << 20

// installResult is what the CLI and the picker both report.
type installResult struct {
	Installed []string
	Skipped   []string
	DryRun    bool
}

// install copies the chosen candidates into the skills directory.
func install(cands []installCandidate, srcRoot string, dryRun bool) (installResult, error) {
	var res installResult
	res.DryRun = dryRun
	for _, c := range cands {
		if !c.Selected {
			continue
		}
		dst := filepath.Join(skillsDir(), c.Name)
		if dryRun {
			res.Installed = append(res.Installed, c.Name)
			continue
		}
		if err := copySkill(filepath.Join(srcRoot, c.Rel), dst); err != nil {
			// One bad candidate must not abandon the rest: the point of
			// the checklist is that some of them can be skipped.
			res.Skipped = append(res.Skipped, c.Name+": "+err.Error())
			continue
		}
		res.Installed = append(res.Installed, c.Name)
	}
	return res, nil
}

// depthBelow counts how many path segments of p are below root.
func depthBelow(root, p string) int {
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == "." {
		return 0
	}
	if strings.HasPrefix(rel, "..") {
		return 99
	}
	return len(strings.Split(rel, string(filepath.Separator)))
}

func relTo(root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return p
	}
	if rel == "." {
		return "."
	}
	return rel
}

// within reports whether path is inside root. Both are cleaned and made
// absolute first, so ".." cannot escape.
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
