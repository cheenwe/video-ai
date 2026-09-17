package storage

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type SFTPDriver struct {
	host     string
	port     int
	username string
	password string
	basePath string
}

func NewSFTPDriver(cfg FTPConfig) (*SFTPDriver, error) {
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		return nil, fmt.Errorf("SFTP 主机未配置")
	}
	port := cfg.Port
	if port == 0 {
		port = 22
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
	return &SFTPDriver{host: host, port: port, username: cfg.Username, password: cfg.Password, basePath: base}, nil
}

func (d *SFTPDriver) connect() (*sftp.Client, *ssh.Client, error) {
	addr := fmt.Sprintf("%s:%d", d.host, d.port)
	config := &ssh.ClientConfig{
		User:            d.username,
		Auth:            []ssh.AuthMethod{ssh.Password(d.password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, nil, err
	}
	client, err := sftp.NewClient(conn)
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	return client, conn, nil
}

func (d *SFTPDriver) withClient(fn func(*sftp.Client) error) error {
	client, conn, err := d.connect()
	if err != nil {
		return err
	}
	defer conn.Close()
	defer client.Close()
	return fn(client)
}

func (d *SFTPDriver) remotePath(virtualPath string) string {
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

func (d *SFTPDriver) virtualFromRemote(remote string) string {
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

func (d *SFTPDriver) List(dirPath string) ([]FileItem, error) {
	remote := d.remotePath(dirPath)
	var items []FileItem
	err := d.withClient(func(c *sftp.Client) error {
		entries, err := c.ReadDir(remote)
		if err != nil {
			return err
		}
		items = make([]FileItem, 0, len(entries))
		for _, ent := range entries {
			name := ent.Name()
			if name == "." || name == ".." {
				continue
			}
			isDir := ent.IsDir()
			vpath := JoinVirtual(dirPath, name)
			item := FileItem{
				Name:     name,
				Path:     vpath,
				Size:     ent.Size(),
				IsDir:    isDir,
				Modified: ent.ModTime().UTC(),
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

func (d *SFTPDriver) Mkdir(dirPath string) error {
	return d.withClient(func(c *sftp.Client) error {
		return c.Mkdir(d.remotePath(dirPath))
	})
}

func (d *SFTPDriver) MkdirAll(dirPath string) error {
	dirPath = NormalizeVirtual(dirPath)
	if dirPath == "/" {
		return nil
	}
	for _, p := range AncestorsVirtual(dirPath) {
		_ = d.Mkdir(p)
	}
	return nil
}

func (d *SFTPDriver) SaveFile(dirPath, filename string, data []byte) error {
	return d.SaveFileAt(JoinVirtual(dirPath, filename), data)
}

func (d *SFTPDriver) SaveFileAt(virtualPath string, data []byte) error {
	return d.SaveFileStream(virtualPath, bytes.NewReader(data), int64(len(data)))
}

func (d *SFTPDriver) SaveFileStream(virtualPath string, r io.Reader, size int64) error {
	remote := d.remotePath(virtualPath)
	parent := filepath.Dir(strings.ReplaceAll(remote, "\\", "/"))
	if parent != "" && parent != "." {
		_ = d.MkdirAll(d.virtualFromRemote(parent))
	}
	body := r
	if size > 0 {
		body = io.LimitReader(r, size)
	}
	return d.withClient(func(c *sftp.Client) error {
		f, err := c.Create(remote)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(f, body)
		return err
	})
}

func (d *SFTPDriver) Delete(paths []string) error {
	return d.withClient(func(c *sftp.Client) error {
		for _, p := range paths {
			remote := d.remotePath(p)
			if err := c.Remove(remote); err == nil {
				continue
			}
			if err := c.RemoveDirectory(remote); err != nil {
				return err
			}
		}
		return nil
	})
}

func (d *SFTPDriver) Rename(oldPath, newPath string) error {
	return d.withClient(func(c *sftp.Client) error {
		return c.Rename(d.remotePath(oldPath), d.remotePath(newPath))
	})
}

func (d *SFTPDriver) readAll(remote string) ([]byte, *FileItem, error) {
	var data []byte
	var item *FileItem
	err := d.withClient(func(c *sftp.Client) error {
		f, err := c.Open(remote)
		if err != nil {
			return err
		}
		defer f.Close()
		data, err = io.ReadAll(f)
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

func (d *SFTPDriver) Copy(srcPath, destDir string) error {
	data, _, err := d.readAll(d.remotePath(srcPath))
	if err != nil {
		return err
	}
	return d.SaveFileAt(JoinVirtual(destDir, BaseName(srcPath)), data)
}

func (d *SFTPDriver) Move(srcPath, destDir string) error {
	dest := d.remotePath(JoinVirtual(destDir, BaseName(srcPath)))
	srcRemote := d.remotePath(srcPath)
	return d.withClient(func(c *sftp.Client) error {
		return c.Rename(srcRemote, dest)
	})
}

func (d *SFTPDriver) Open(filePath string) (string, *FileItem, error) {
	return "", nil, ErrUnsupported
}

func (d *SFTPDriver) ReadFile(filePath string) (io.ReadCloser, *FileItem, error) {
	client, conn, err := d.connect()
	if err != nil {
		return nil, nil, err
	}
	remote := d.remotePath(filePath)
	st, err := client.Stat(remote)
	if err != nil {
		conn.Close()
		client.Close()
		return nil, nil, err
	}
	if st.IsDir() {
		conn.Close()
		client.Close()
		return nil, nil, ErrNotFound
	}
	f, err := client.Open(remote)
	if err != nil {
		conn.Close()
		client.Close()
		return nil, nil, err
	}
	vpath := d.virtualFromRemote(remote)
	item := &FileItem{
		Name:     BaseName(vpath),
		Path:     vpath,
		Size:     st.Size(),
		IsDir:    false,
		Modified: st.ModTime().UTC(),
		MimeType: mime.TypeByExtension(filepath.Ext(BaseName(vpath))),
	}
	rc := ReadCloserWithCleanup(f, func() error {
		client.Close()
		conn.Close()
		return nil
	})
	return rc, item, nil
}

func (d *SFTPDriver) Test() error {
	return d.withClient(func(c *sftp.Client) error {
		_, err := c.Getwd()
		return err
	})
}
