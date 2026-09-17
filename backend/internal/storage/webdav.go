package storage

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"github.com/studio-b12/gowebdav"
)

type WebDAVDriver struct {
	client *gowebdav.Client
	root   string
}

func NewWebDAVDriver(cfg WebDAVConfig) (*WebDAVDriver, error) {
	url := strings.TrimSpace(cfg.URL)
	if url == "" {
		return nil, fmt.Errorf("WebDAV URL 未配置")
	}
	url = strings.TrimSuffix(url, "/")
	client := gowebdav.NewClient(url, cfg.Username, cfg.Password)
	return &WebDAVDriver{client: client, root: url}, nil
}

func (d *WebDAVDriver) remotePath(virtualPath string) string {
	virtualPath = NormalizeVirtual(virtualPath)
	if virtualPath == "/" {
		return ""
	}
	return strings.TrimPrefix(virtualPath, "/")
}

func (d *WebDAVDriver) virtualFromRemote(remote string) string {
	remote = strings.Trim(remote, "/")
	if remote == "" {
		return "/"
	}
	return NormalizeVirtual("/" + remote)
}

func (d *WebDAVDriver) List(dirPath string) ([]FileItem, error) {
	remote := d.remotePath(dirPath)
	infos, err := d.client.ReadDir(remote)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil, ErrNotFound
		}
		return nil, err
	}
	items := make([]FileItem, 0, len(infos))
	for _, info := range infos {
		name := info.Name()
		if name == "." || name == ".." {
			continue
		}
		isDir := info.IsDir()
		vpath := JoinVirtual(dirPath, name)
		item := FileItem{
			Name:     name,
			Path:     vpath,
			Size:     info.Size(),
			IsDir:    isDir,
			Modified: info.ModTime().UTC(),
		}
		if !isDir {
			item.MimeType = mime.TypeByExtension(filepath.Ext(name))
		}
		items = append(items, item)
	}
	return items, nil
}

func (d *WebDAVDriver) Mkdir(dirPath string) error {
	remote := d.remotePath(dirPath)
	if remote == "" {
		return nil
	}
	return d.client.Mkdir(remote, 0o755)
}

func (d *WebDAVDriver) MkdirAll(dirPath string) error {
	dirPath = NormalizeVirtual(dirPath)
	if dirPath == "/" {
		return nil
	}
	for _, p := range AncestorsVirtual(dirPath) {
		if err := d.Mkdir(p); err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "exist") {
				return err
			}
		}
	}
	return nil
}

func (d *WebDAVDriver) SaveFile(dirPath, filename string, data []byte) error {
	return d.SaveFileAt(JoinVirtual(dirPath, filename), data)
}

func (d *WebDAVDriver) SaveFileAt(virtualPath string, data []byte) error {
	return d.SaveFileStream(virtualPath, bytes.NewReader(data), int64(len(data)))
}

func (d *WebDAVDriver) SaveFileStream(virtualPath string, r io.Reader, size int64) error {
	data, err := ReadAllSized(r, size)
	if err != nil {
		return err
	}
	remote := d.remotePath(virtualPath)
	parent := filepath.Dir(strings.ReplaceAll(remote, "\\", "/"))
	if parent != "" && parent != "." {
		_ = d.MkdirAll(d.virtualFromRemote(parent))
	}
	return d.client.Write(remote, data, 0o644)
}

func (d *WebDAVDriver) Delete(paths []string) error {
	for _, p := range paths {
		remote := d.remotePath(p)
		if err := d.client.Remove(remote); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "collection") {
				if err := d.client.RemoveAll(remote); err != nil {
					return err
				}
				continue
			}
			return err
		}
	}
	return nil
}

func (d *WebDAVDriver) Rename(oldPath, newPath string) error {
	return d.client.Rename(d.remotePath(oldPath), d.remotePath(newPath), true)
}

func (d *WebDAVDriver) readAll(remote string) ([]byte, *FileItem, error) {
	data, err := d.client.Read(remote)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	vpath := d.virtualFromRemote(remote)
	item := &FileItem{
		Name:     BaseName(vpath),
		Path:     vpath,
		Size:     int64(len(data)),
		IsDir:    false,
		Modified: time.Now().UTC(),
		MimeType: mime.TypeByExtension(filepath.Ext(BaseName(vpath))),
	}
	return data, item, nil
}

func (d *WebDAVDriver) Copy(srcPath, destDir string) error {
	srcRemote := d.remotePath(srcPath)
	data, _, err := d.readAll(srcRemote)
	if err != nil {
		return err
	}
	dest := JoinVirtual(destDir, BaseName(srcPath))
	return d.SaveFileAt(dest, data)
}

func (d *WebDAVDriver) Move(srcPath, destDir string) error {
	destRemote := d.remotePath(JoinVirtual(destDir, BaseName(srcPath)))
	srcRemote := d.remotePath(srcPath)
	return d.client.Rename(srcRemote, destRemote, true)
}

func (d *WebDAVDriver) Open(filePath string) (string, *FileItem, error) {
	return "", nil, ErrUnsupported
}

func (d *WebDAVDriver) ReadFile(filePath string) (io.ReadCloser, *FileItem, error) {
	remote := d.remotePath(filePath)
	info, err := d.client.Stat(remote)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	if info.IsDir() {
		return nil, nil, ErrNotFound
	}
	rc, err := d.client.ReadStream(remote)
	if err != nil {
		return nil, nil, err
	}
	vpath := d.virtualFromRemote(remote)
	item := &FileItem{
		Name:     BaseName(vpath),
		Path:     vpath,
		Size:     info.Size(),
		IsDir:    false,
		Modified: info.ModTime().UTC(),
		MimeType: mime.TypeByExtension(filepath.Ext(BaseName(vpath))),
	}
	return rc, item, nil
}

func (d *WebDAVDriver) Test() error {
	_, err := d.client.ReadDir("")
	return err
}
