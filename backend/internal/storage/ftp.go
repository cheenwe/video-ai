package storage

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"github.com/jlaffaye/ftp"
)

type FTPDriver struct {
	host     string
	port     int
	username string
	password string
	basePath string
}

func NewFTPDriver(cfg FTPConfig) (*FTPDriver, error) {
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		return nil, fmt.Errorf("FTP 主机未配置")
	}
	port := cfg.Port
	if port == 0 {
		port = 21
	}
	base := strings.TrimSpace(cfg.BasePath)
	if base == "" {
		base = "/"
	}
	if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	base = strings.TrimSuffix(base, "/")
	if base == "" {
		base = "/"
	}
	return &FTPDriver{
		host:     host,
		port:     port,
		username: cfg.Username,
		password: cfg.Password,
		basePath: base,
	}, nil
}

func (d *FTPDriver) dial() (*ftp.ServerConn, error) {
	addr := fmt.Sprintf("%s:%d", d.host, d.port)
	conn, err := ftp.Dial(addr, ftp.DialWithTimeout(15*time.Second))
	if err != nil {
		return nil, err
	}
	if err := conn.Login(d.username, d.password); err != nil {
		conn.Quit()
		return nil, err
	}
	return conn, nil
}

func (d *FTPDriver) withConn(fn func(*ftp.ServerConn) error) error {
	conn, err := d.dial()
	if err != nil {
		return err
	}
	defer conn.Quit()
	return fn(conn)
}

func (d *FTPDriver) remotePath(virtualPath string) string {
	virtualPath = NormalizeVirtual(virtualPath)
	if virtualPath == "/" {
		return d.basePath
	}
	rel := strings.TrimPrefix(virtualPath, "/")
	if d.basePath == "/" {
		return "/" + rel
	}
	return d.basePath + "/" + rel
}

func (d *FTPDriver) virtualFromRemote(remote string) string {
	remote = "/" + strings.Trim(remote, "/")
	base := d.basePath
	if base != "/" {
		if remote == base {
			return "/"
		}
		prefix := base + "/"
		if strings.HasPrefix(remote, prefix) {
			remote = "/" + strings.TrimPrefix(remote, prefix)
		}
	}
	if remote == "" || remote == "/" {
		return "/"
	}
	return NormalizeVirtual(remote)
}

func (d *FTPDriver) List(dirPath string) ([]FileItem, error) {
	remote := d.remotePath(dirPath)
	var items []FileItem
	err := d.withConn(func(conn *ftp.ServerConn) error {
		entries, err := conn.List(remote)
		if err != nil {
			return err
		}
		items = make([]FileItem, 0, len(entries))
		for _, ent := range entries {
			name := ent.Name
			if name == "." || name == ".." {
				continue
			}
			isDir := ent.Type == ftp.EntryTypeFolder
			vpath := JoinVirtual(dirPath, name)
			item := FileItem{
				Name:     name,
				Path:     vpath,
				Size:     int64(ent.Size),
				IsDir:    isDir,
				Modified: ent.Time.UTC(),
			}
			if !isDir {
				item.MimeType = mime.TypeByExtension(filepath.Ext(name))
			}
			items = append(items, item)
		}
		return nil
	})
	return items, err
}

func (d *FTPDriver) Mkdir(dirPath string) error {
	remote := d.remotePath(dirPath)
	return d.withConn(func(conn *ftp.ServerConn) error {
		return conn.MakeDir(remote)
	})
}

func (d *FTPDriver) MkdirAll(dirPath string) error {
	dirPath = NormalizeVirtual(dirPath)
	if dirPath == "/" {
		return nil
	}
	for _, p := range AncestorsVirtual(dirPath) {
		if err := d.Mkdir(p); err != nil {
			// ignore exists
			if !strings.Contains(strings.ToLower(err.Error()), "exist") {
				return err
			}
		}
	}
	return nil
}

func (d *FTPDriver) SaveFile(dirPath, filename string, data []byte) error {
	return d.SaveFileAt(JoinVirtual(dirPath, filename), data)
}

func (d *FTPDriver) SaveFileAt(virtualPath string, data []byte) error {
	return d.SaveFileStream(virtualPath, bytes.NewReader(data), int64(len(data)))
}

func (d *FTPDriver) SaveFileStream(virtualPath string, r io.Reader, size int64) error {
	remote := d.remotePath(virtualPath)
	parent := filepath.Dir(strings.ReplaceAll(remote, "\\", "/"))
	if parent != "" && parent != "." {
		_ = d.MkdirAll(d.virtualFromRemote(parent))
	}
	body := r
	if size > 0 {
		body = io.LimitReader(r, size)
	}
	return d.withConn(func(conn *ftp.ServerConn) error {
		return conn.Stor(remote, body)
	})
}

func (d *FTPDriver) Delete(paths []string) error {
	return d.withConn(func(conn *ftp.ServerConn) error {
		for _, p := range paths {
			remote := d.remotePath(p)
			if err := conn.Delete(remote); err == nil {
				continue
			}
			if err := conn.RemoveDir(remote); err != nil {
				return err
			}
		}
		return nil
	})
}

func (d *FTPDriver) Rename(oldPath, newPath string) error {
	oldRemote := d.remotePath(oldPath)
	newRemote := d.remotePath(newPath)
	return d.withConn(func(conn *ftp.ServerConn) error {
		return conn.Rename(oldRemote, newRemote)
	})
}

func (d *FTPDriver) readAll(remote string) ([]byte, *FileItem, error) {
	var data []byte
	var item *FileItem
	err := d.withConn(func(conn *ftp.ServerConn) error {
		rc, err := conn.Retr(remote)
		if err != nil {
			return err
		}
		defer rc.Close()
		data, err = io.ReadAll(rc)
		if err != nil {
			return err
		}
		vpath := d.virtualFromRemote(remote)
		item = &FileItem{
			Name:     BaseName(vpath),
			Path:     vpath,
			Size:     int64(len(data)),
			IsDir:    false,
			Modified: time.Now().UTC(),
			MimeType: mime.TypeByExtension(filepath.Ext(BaseName(vpath))),
		}
		return nil
	})
	return data, item, err
}

func (d *FTPDriver) Copy(srcPath, destDir string) error {
	srcRemote := d.remotePath(srcPath)
	data, _, err := d.readAll(srcRemote)
	if err != nil {
		return err
	}
	dest := JoinVirtual(destDir, BaseName(srcPath))
	return d.SaveFileAt(dest, data)
}

func (d *FTPDriver) Move(srcPath, destDir string) error {
	dest := d.remotePath(JoinVirtual(destDir, BaseName(srcPath)))
	srcRemote := d.remotePath(srcPath)
	return d.withConn(func(conn *ftp.ServerConn) error {
		return conn.Rename(srcRemote, dest)
	})
}

func (d *FTPDriver) Open(filePath string) (string, *FileItem, error) {
	return "", nil, ErrUnsupported
}

func (d *FTPDriver) ReadFile(filePath string) (io.ReadCloser, *FileItem, error) {
	conn, err := d.dial()
	if err != nil {
		return nil, nil, err
	}
	remote := d.remotePath(filePath)
	size, sizeErr := conn.FileSize(remote)
	resp, err := conn.Retr(remote)
	if err != nil {
		conn.Quit()
		return nil, nil, err
	}
	vpath := d.virtualFromRemote(remote)
	item := &FileItem{
		Name:     BaseName(vpath),
		Path:     vpath,
		IsDir:    false,
		Modified: time.Now().UTC(),
		MimeType: mime.TypeByExtension(filepath.Ext(BaseName(vpath))),
	}
	if sizeErr == nil {
		item.Size = size
	}
	rc := ReadCloserWithCleanup(resp, func() error {
		return conn.Quit()
	})
	return rc, item, nil
}

func (d *FTPDriver) Test() error {
	return d.withConn(func(conn *ftp.ServerConn) error {
		_, err := conn.CurrentDir()
		return err
	})
}
