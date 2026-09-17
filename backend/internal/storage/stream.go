package storage

import (
	"io"
	"os"
)

// ReadAllSized reads r fully. When size > 0, pre-allocates and requires exact length.
func ReadAllSized(r io.Reader, size int64) ([]byte, error) {
	if size < 0 {
		return io.ReadAll(r)
	}
	if size == 0 {
		return []byte{}, nil
	}
	buf := make([]byte, size)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

type cleanupReadCloser struct {
	io.ReadCloser
	cleanup func() error
}

func (r *cleanupReadCloser) Close() error {
	err := r.ReadCloser.Close()
	if r.cleanup != nil {
		if cerr := r.cleanup(); err == nil {
			err = cerr
		}
	}
	return err
}

// ReadCloserWithCleanup wraps rc; cleanup runs after rc is closed (e.g. release remote connection).
func ReadCloserWithCleanup(rc io.ReadCloser, cleanup func() error) io.ReadCloser {
	return &cleanupReadCloser{ReadCloser: rc, cleanup: cleanup}
}

// DrainToTemp copies r to a temp file (for drivers that need a seekable / byte API).
// Caller must call cleanup when done.
func DrainToTemp(r io.Reader) (path string, size int64, cleanup func(), err error) {
	f, err := os.CreateTemp("", "nextfile-upload-*")
	if err != nil {
		return "", 0, nil, err
	}
	path = f.Name()
	cleanup = func() { _ = os.Remove(path) }
	n, copyErr := io.Copy(f, r)
	closeErr := f.Close()
	if copyErr != nil {
		cleanup()
		return "", 0, nil, copyErr
	}
	if closeErr != nil {
		cleanup()
		return "", 0, nil, closeErr
	}
	return path, n, cleanup, nil
}
