package autoupdate

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Publication never reopens a named temporary file. CreateTemp uses O_EXCL and
// mode 0600. Concurrent attempts own different inodes and clean only their own.
type publicationOps struct {
	write  func(*os.File, []byte) (int, error)
	sync   func(*os.File) error
	close  func(*os.File) error
	rename func(string, string) error
}
type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.r.Read(p)
	if canceled := r.ctx.Err(); canceled != nil {
		return n, canceled
	}
	return n, err
}

type contextWriter struct {
	ctx   context.Context
	write func([]byte) (int, error)
}

func (w contextWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := w.write(p)
	if canceled := w.ctx.Err(); canceled != nil {
		return n, canceled
	}
	return n, err
}

func writeAtomic(ctx context.Context, dest string, r io.Reader, mode os.FileMode) error {
	return writeAtomicWith(ctx, dest, r, mode, publicationOps{})
}
func writeAtomicWith(ctx context.Context, dest string, r io.Reader, mode os.FileMode, ops publicationOps) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(dest), ".backscroll-publish-*")
	if err != nil {
		return err
	}
	temp := f.Name()
	closed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
		_ = os.Remove(temp)
	}()
	if ops.write == nil {
		ops.write = func(f *os.File, p []byte) (int, error) { return f.Write(p) }
	}
	if ops.sync == nil {
		ops.sync = func(f *os.File) error { return f.Sync() }
	}
	if ops.close == nil {
		ops.close = func(f *os.File) error { return f.Close() }
	}
	if ops.rename == nil {
		ops.rename = os.Rename
	}
	_, err = io.Copy(contextWriter{ctx, func(p []byte) (int, error) { return ops.write(f, p) }}, contextReader{ctx, r})
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err = f.Chmod(mode); err != nil {
		return err
	}
	if err = ops.sync(f); err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if err = ops.close(f); err != nil {
		return err
	}
	closed = true
	current, err := os.Lstat(temp)
	if err != nil {
		return err
	}
	if !current.Mode().IsRegular() || !os.SameFile(info, current) {
		return fmt.Errorf("prepared publication file changed")
	}
	if destInfo, err := os.Lstat(dest); err == nil {
		if !destInfo.Mode().IsRegular() {
			return fmt.Errorf("publication destination is not regular")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return ops.rename(temp, dest)
}

// Portable checks reject observed symlinks and check opened-file identity.
// This is not an atomic O_NOFOLLOW guarantee; parent-directory and final
// namespace races require a trusted per-user cache/installation directory.
func openRegular(path string) (*os.File, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file: %s", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err == nil {
		after, statErr := os.Lstat(path)
		err = statErr
		if err == nil && (!after.Mode().IsRegular() || !info.Mode().IsRegular() || !os.SameFile(before, info) || !os.SameFile(after, info)) {
			err = fmt.Errorf("file changed while opening: %s", path)
		}
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

type contextReaderAt struct {
	ctx context.Context
	r   io.ReaderAt
}

func (r contextReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.r.ReadAt(p, off)
	if canceled := r.ctx.Err(); canceled != nil {
		return n, canceled
	}
	return n, err
}

// Reject already-observable symlink directory components. This deliberately
// does not claim protection against concurrent directory replacement.
func stagingDirectory(ctx context.Context, cache string, parts []string, create bool) error {
	path := cache
	for i := 0; i <= len(parts); i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if i > 0 {
			path = filepath.Join(path, parts[i-1])
		}
		if create {
			var err error
			if i == 0 {
				err = os.MkdirAll(path, 0755)
			} else {
				err = os.Mkdir(path, 0755)
			}
			if err != nil && !os.IsExist(err) {
				return err
			}
		}
		info, err := os.Lstat(path)
		if !create && os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("staging directory is not a real directory: %s", path)
		}
	}
	return ctx.Err()
}
