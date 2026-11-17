//go:build !windows

package updater

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func snapshotFile(root *os.Root, name, dest string, limit int64) (fileMeta, error) {
	var meta fileMeta
	info, err := root.Lstat(name)
	if err != nil {
		return meta, err
	}
	if !info.Mode().IsRegular() {
		return meta, errors.New("snapshot refuses links and special files")
	}
	in, err := root.Open(name)
	if err != nil {
		return meta, err
	}
	defer func() { _ = in.Close() }()
	info, err = in.Stat()
	if err != nil {
		return meta, err
	}
	meta, links, err := metadata(name, info)
	if err != nil || links != 1 || !info.Mode().IsRegular() || info.Size() > limit || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return meta, errors.New("snapshot refuses hardlinks, oversized files or special modes")
	}
	if err := noACL(in); err != nil {
		return meta, err
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600) // #nosec G304 -- destination is inside the inaccessible root-owned transaction
	if err != nil {
		return meta, err
	}
	n, copyErr := io.Copy(out, io.LimitReader(in, limit+1))
	if n != info.Size() || n > limit {
		copyErr = errors.New("snapshot source changed or exceeded size limit")
	}
	err = errors.Join(copyErr, out.Sync(), out.Close())
	return meta, err
}

func snapshotTree(source, dest string) ([]fileMeta, error) {
	root, err := os.OpenRoot(source)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	var entries []fileMeta
	var total int64
	err = fs.WalkDir(root.FS(), ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if len(entries) >= 10000 {
			return errors.New("configuration snapshot contains too many entries")
		}
		info, err := root.Lstat(path)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			meta, err := snapshotFile(root, path, filepath.Join(dest, path), 64<<20)
			if err != nil {
				return err
			}
			total += info.Size()
			if total > 64<<20 {
				return errors.New("configuration exceeds 64 MiB snapshot limit")
			}
			entries = append(entries, meta)
			return nil
		}
		meta, _, err := metadata(path, info)
		if err != nil {
			return err
		}
		f, err := root.Open(path)
		if err != nil {
			return err
		}
		aclErr := noACL(f)
		_ = f.Close()
		if aclErr != nil {
			return aclErr
		}
		if err := os.Mkdir(filepath.Join(dest, path), 0700); err != nil {
			return err
		}
		entries = append(entries, meta)
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Flush leaves before their parents, so journal publication cannot precede
	// durable backup directory entries.
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Dir {
			if err := syncDir(filepath.Join(dest, entries[i].Path)); err != nil {
				return nil, err
			}
		}
	}
	return entries, nil
}

func restoreTree(dest, backup string, entries []fileMeta) error {
	root, err := os.OpenRoot(dest)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	d, err := root.Open(".")
	if err != nil {
		return err
	}
	children, err := d.ReadDir(-1)
	_ = d.Close()
	if err != nil {
		return err
	}
	for _, child := range children {
		// Root.RemoveAll never follows a symlink outside the pinned directory.
		if err := root.RemoveAll(child.Name()); err != nil {
			return err
		}
	}
	for _, meta := range entries {
		if meta.Dir {
			if meta.Path != "." {
				if err := root.Mkdir(meta.Path, 0700); err != nil {
					return err
				}
			}
		} else if err := replaceInRoot(root, meta.Path, filepath.Join(backup, meta.Path), meta); err != nil {
			return err
		}
	}
	// Apply directory permissions last so a read-only directory in the snapshot
	// does not prevent its children being reconstructed.
	for i := len(entries) - 1; i >= 0; i-- {
		meta := entries[i]
		if !meta.Dir {
			continue
		}
		if err := root.Chown(meta.Path, meta.UID, meta.GID); err != nil {
			return err
		}
		if err := root.Chmod(meta.Path, os.FileMode(meta.Mode)); err != nil {
			return err
		}
		f, err := root.Open(meta.Path)
		if err != nil {
			return err
		}
		err = f.Sync()
		_ = f.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func replaceFile(dir, name, source string, meta fileMeta) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	return replaceInRoot(root, name, source, meta)
}

func replaceInRoot(root *os.Root, name, source string, meta fileMeta) error {
	in, err := os.Open(source) // #nosec G304 -- source is a fixed root-private backup or verified staging file
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	tmp := name + ".loom-update"
	// The root-owned transaction lock serializes replacements. Remove only
	// this exact leaf from an interrupted previous attempt, never its target.
	if err := root.Remove(tmp); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	out, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	if copyErr == nil {
		copyErr = out.Chown(meta.UID, meta.GID)
	}
	if copyErr == nil {
		copyErr = out.Chmod(os.FileMode(meta.Mode))
	}
	if copyErr == nil {
		copyErr = out.Sync()
	}
	closeErr := out.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return err
	}
	if err := root.Rename(tmp, name); err != nil {
		return err
	}
	parent, err := root.Open(filepath.Dir(name))
	if err != nil {
		return err
	}
	defer func() { _ = parent.Close() }()
	return parent.Sync()
}

func atomicJSON(dir, name string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, name+".tmp")
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		return err
	}
	if err := syncFile(tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(dir, name)); err != nil {
		return err
	}
	return syncDir(dir)
}

func syncDir(path string) error {
	f, err := os.Open(path) // #nosec G304 -- fsync of internal fixed directories, not service-provided paths
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.Sync()
}
func removeDurable(dir, name string) error {
	if err := os.Remove(filepath.Join(dir, name)); err != nil {
		return err
	}
	return syncDir(dir)
}
