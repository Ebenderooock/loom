package imports

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// RecycleFromRoot recycles a file from an already-open title folder. Unlike an
// absolute-path rename, rooted copy/remove cannot follow a concurrently swapped
// source or destination parent symlink outside the authorized directory.
func (r *RecycleBin) RecycleFromRoot(root *os.Root, name, libraryRoot string) error {
	if !r.Enabled || r.Path == "" {
		return root.Remove(name)
	}
	relativeRoot, err := filepath.Rel(libraryRoot, root.Name())
	if err != nil || !filepath.IsLocal(relativeRoot) || !filepath.IsLocal(name) {
		return errors.New("recycle source is outside library")
	}
	source, err := root.Open(name)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()
	info, err := source.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("recycle source is not a regular file")
	}
	if err := os.MkdirAll(r.Path, 0750); err != nil {
		return err
	}
	bin, err := os.OpenRoot(r.Path)
	if err != nil {
		return err
	}
	defer func() { _ = bin.Close() }()
	dest := filepath.Join(filepath.Base(libraryRoot), relativeRoot, name)
	if err := bin.MkdirAll(filepath.Dir(dest), 0750); err != nil {
		return err
	}
	target, err := bin.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if errors.Is(err, os.ErrExist) {
		dest = fmt.Sprintf("%s_%d", dest, time.Now().UnixNano())
		target, err = bin.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	}
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(target, source)
	syncErr := target.Sync()
	closeErr := target.Close()
	if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
		return errors.Join(err, bin.Remove(dest))
	}
	if err := root.Remove(name); err != nil {
		return errors.Join(err, bin.Remove(dest))
	}
	return nil
}
