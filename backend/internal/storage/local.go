package storage

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type LocalDriver struct {
	BasePath       string
	ProtectedRoots []string // 绝对路径：系统数据区，禁止改删
}

func NewLocalDriver(basePath string, protectedRoots ...string) (*LocalDriver, error) {
	abs, err := filepath.Abs(basePath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	roots := make([]string, 0, len(protectedRoots))
	for _, r := range protectedRoots {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		if a, err := filepath.Abs(r); err == nil {
			roots = append(roots, a)
		} else {
			roots = append(roots, filepath.Clean(r))
		}
	}
	return &LocalDriver{BasePath: abs, ProtectedRoots: roots}, nil
}

func (d *LocalDriver) guardWrite(virtPath string) error {
	if IsProtectedVirtualPath(virtPath) {
		return ErrProtected
	}
	abs, err := d.resolve(virtPath)
	if err != nil {
		return err
	}
	if IsUnderAnyRoot(abs, d.ProtectedRoots) {
		return ErrProtected
	}
	return nil
}

func (d *LocalDriver) resolve(virtualPath string) (string, error) {
	virtualPath = NormalizeVirtual(virtualPath)
	base, err := filepath.Abs(d.BasePath)
	if err != nil {
		return "", err
	}
	if virtualPath == "/" {
		return base, nil
	}
	rel := strings.TrimPrefix(virtualPath, "/")
	abs := filepath.Join(base, filepath.FromSlash(rel))
	abs, err = filepath.Abs(abs)
	if err != nil {
		return "", err
	}
	if !isPathWithin(base, abs) {
		return "", ErrPathTraversal
	}
	return d.resolveSymlinks(base, abs)
}

// resolveSymlinks verifies the deepest existing ancestor against the real base
// directory. This also protects create operations whose final path does not yet
// exist but whose parent contains a symlink escaping BasePath.
func (d *LocalDriver) resolveSymlinks(base, target string) (string, error) {
	realBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		return "", err
	}
	existing := target
	var suffix []string
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", ErrPathTraversal
		}
		suffix = append(suffix, filepath.Base(existing))
		existing = parent
	}
	realExisting, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", err
	}
	if !isPathWithin(realBase, realExisting) {
		return "", ErrPathTraversal
	}
	resolved := realExisting
	for i := len(suffix) - 1; i >= 0; i-- {
		resolved = filepath.Join(resolved, suffix[i])
	}
	if !isPathWithin(realBase, resolved) {
		return "", ErrPathTraversal
	}
	return target, nil
}

func isPathWithin(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func (d *LocalDriver) toVirtual(absPath string) (string, error) {
	base, err := filepath.Abs(d.BasePath)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(absPath)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(base, abs)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return "/", nil
	}
	if strings.HasPrefix(rel, "..") {
		return "", ErrPathTraversal
	}
	return NormalizeVirtual("/" + filepath.ToSlash(rel)), nil
}

func (d *LocalDriver) statEntry(absPath string) (*FileItem, error) {
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	vpath, err := d.toVirtual(absPath)
	if err != nil {
		return nil, err
	}
	item := &FileItem{
		Name:     info.Name(),
		Path:     vpath,
		Size:     info.Size(),
		IsDir:    info.IsDir(),
		Modified: info.ModTime().UTC(),
	}
	if !info.IsDir() {
		item.MimeType = mime.TypeByExtension(filepath.Ext(info.Name()))
	}
	return item, nil
}

func (d *LocalDriver) List(dirPath string) ([]FileItem, error) {
	abs, err := d.resolve(dirPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, ErrNotFound
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	items := make([]FileItem, 0, len(entries))
	for _, ent := range entries {
		if IsProtectedName(ent.Name()) {
			continue
		}
		entryPath := filepath.Join(abs, ent.Name())
		if IsUnderAnyRoot(entryPath, d.ProtectedRoots) {
			continue
		}
		item, err := d.statEntry(entryPath)
		if err != nil {
			continue
		}
		items = append(items, *item)
	}
	return items, nil
}

func (d *LocalDriver) Mkdir(dirPath string) error {
	if err := d.guardWrite(dirPath); err != nil {
		return err
	}
	abs, err := d.resolve(dirPath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err == nil {
		return ErrExists
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.MkdirAll(abs, 0o755)
}

func (d *LocalDriver) MkdirAll(dirPath string) error {
	abs, err := d.resolve(dirPath)
	if err != nil {
		return err
	}
	return os.MkdirAll(abs, 0o755)
}

func (d *LocalDriver) SaveFile(dirPath, filename string, data []byte) error {
	filename = strings.TrimSpace(filename)
	if filename == "" || strings.Contains(filename, "/") || strings.Contains(filename, `\`) {
		return ErrPathTraversal
	}
	if IsProtectedName(filename) {
		return ErrProtected
	}
	targetVirt := JoinVirtual(dirPath, filename)
	if err := d.guardWrite(targetVirt); err != nil {
		return err
	}
	dirAbs, err := d.resolve(dirPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dirAbs, 0o755); err != nil {
		return err
	}
	target := filepath.Join(dirAbs, filename)
	if _, err := os.Stat(target); err == nil {
		return ErrExists
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(target, data, 0o644)
}

func (d *LocalDriver) SaveFileAt(virtualPath string, data []byte) error {
	return d.SaveFileStream(virtualPath, bytes.NewReader(data), int64(len(data)))
}

func (d *LocalDriver) SaveFileStream(virtualPath string, r io.Reader, size int64) error {
	virtualPath = NormalizeVirtual(virtualPath)
	if err := d.guardWrite(virtualPath); err != nil {
		return err
	}
	parent := ParentVirtual(virtualPath)
	filename := BaseName(virtualPath)
	if filename == "" || filename == "/" {
		return ErrPathTraversal
	}
	if strings.Contains(filename, "/") || strings.Contains(filename, `\`) {
		return ErrPathTraversal
	}
	dirAbs, err := d.resolve(parent)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dirAbs, 0o755); err != nil {
		return err
	}
	target := filepath.Join(dirAbs, filename)
	tmp := target + ".uploading"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	var written int64
	if size > 0 {
		written, err = io.Copy(f, io.LimitReader(r, size))
	} else {
		written, err = io.Copy(f, r)
	}
	closeErr := f.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if size > 0 && written != size {
		_ = os.Remove(tmp)
		return fmt.Errorf("incomplete write: got %d want %d", written, size)
	}
	return os.Rename(tmp, target)
}

func (d *LocalDriver) Delete(paths []string) error {
	for _, p := range paths {
		if err := d.guardWrite(p); err != nil {
			return err
		}
		abs, err := d.resolve(p)
		if err != nil {
			return err
		}
		info, err := os.Stat(abs)
		if err != nil {
			if os.IsNotExist(err) {
				return ErrNotFound
			}
			return err
		}
		if info.IsDir() {
			entries, err := os.ReadDir(abs)
			if err != nil {
				return err
			}
			if len(entries) > 0 {
				return ErrNotEmpty
			}
			if err := os.Remove(abs); err != nil {
				return err
			}
		} else {
			if err := os.Remove(abs); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *LocalDriver) Rename(oldPath, newPath string) error {
	if err := d.guardWrite(oldPath); err != nil {
		return err
	}
	if err := d.guardWrite(newPath); err != nil {
		return err
	}
	oldAbs, err := d.resolve(oldPath)
	if err != nil {
		return err
	}
	newAbs, err := d.resolve(newPath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(oldAbs); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	if _, err := os.Stat(newAbs); err == nil {
		return ErrExists
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(newAbs), 0o755); err != nil {
		return err
	}
	return os.Rename(oldAbs, newAbs)
}

func (d *LocalDriver) Copy(srcPath, destDir string) error {
	srcAbs, err := d.resolve(srcPath)
	if err != nil {
		return err
	}
	destDirAbs, err := d.resolve(destDir)
	if err != nil {
		return err
	}
	info, err := os.Stat(srcAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	destAbs := filepath.Join(destDirAbs, info.Name())
	if _, err := os.Stat(destAbs); err == nil {
		return ErrExists
	} else if !os.IsNotExist(err) {
		return err
	}
	return copyPath(srcAbs, destAbs, info)
}

func (d *LocalDriver) Move(srcPath, destDir string) error {
	srcAbs, err := d.resolve(srcPath)
	if err != nil {
		return err
	}
	destDirAbs, err := d.resolve(destDir)
	if err != nil {
		return err
	}
	info, err := os.Stat(srcAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	destAbs := filepath.Join(destDirAbs, info.Name())
	if _, err := os.Stat(destAbs); err == nil {
		return ErrExists
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(destDirAbs, 0o755); err != nil {
		return err
	}
	return os.Rename(srcAbs, destAbs)
}

func copyPath(src, dest string, info os.FileInfo) error {
	if info.IsDir() {
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, ent := range entries {
			subInfo, err := ent.Info()
			if err != nil {
				return err
			}
			if err := copyPath(filepath.Join(src, ent.Name()), filepath.Join(dest, ent.Name()), subInfo); err != nil {
				return err
			}
		}
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func (d *LocalDriver) Open(filePath string) (string, *FileItem, error) {
	abs, err := d.resolve(filePath)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, ErrNotFound
		}
		return "", nil, err
	}
	if info.IsDir() {
		return "", nil, ErrNotFound
	}
	item, err := d.statEntry(abs)
	if err != nil {
		return "", nil, err
	}
	return abs, item, nil
}

func (d *LocalDriver) ReadFile(filePath string) (io.ReadCloser, *FileItem, error) {
	abs, item, err := d.Open(filePath)
	if err != nil {
		return nil, nil, err
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, nil, err
	}
	return f, item, nil
}

func (d *LocalDriver) Test() error {
	return filepath.WalkDir(d.BasePath, func(path string, dentry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return nil
	})
}

func CopyToWriter(srcPath string, w io.Writer) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

func FormatModified(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
