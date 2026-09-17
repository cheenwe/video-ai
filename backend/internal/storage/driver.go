package storage

import (
	"io"
	"time"
)

type FileItem struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	IsDir    bool      `json:"is_dir"`
	Modified time.Time `json:"modified"`
	MimeType string    `json:"mime_type,omitempty"`
}

type Driver interface {
	List(dirPath string) ([]FileItem, error)
	Mkdir(dirPath string) error
	MkdirAll(dirPath string) error
	SaveFile(dirPath, filename string, data []byte) error
	SaveFileAt(virtualPath string, data []byte) error
	SaveFileStream(virtualPath string, r io.Reader, size int64) error
	Delete(paths []string) error
	Rename(oldPath, newPath string) error
	Copy(srcPath, destDir string) error
	Move(srcPath, destDir string) error
	Open(filePath string) (localPath string, item *FileItem, err error)
	ReadFile(filePath string) (io.ReadCloser, *FileItem, error)
	Test() error
}
