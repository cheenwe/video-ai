package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Driver struct {
	client *minio.Client
	bucket string
	prefix string
}

func NewS3Driver(cfg S3Config) (*S3Driver, error) {
	endpoint, bucket, prefix, useSSL, err := normalizeS3Config(cfg)
	if err != nil {
		return nil, err
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: useSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, err
	}
	return &S3Driver{
		client: client,
		bucket: bucket,
		prefix: prefix,
	}, nil
}

// normalizeS3Config 清洗 endpoint：去掉协议、路径，并兼容阿里云等虚拟主机风格地址。
func normalizeS3Config(cfg S3Config) (endpoint, bucket, prefix string, useSSL bool, err error) {
	raw := strings.TrimSpace(cfg.Endpoint)
	if raw == "" {
		return "", "", "", cfg.UseSSL, errors.New("S3 endpoint 未配置")
	}

	bucket = strings.TrimSpace(cfg.Bucket)
	prefix = strings.Trim(cfg.Prefix, "/")
	useSSL = cfg.UseSSL

	host := raw
	extraPath := ""

	if strings.Contains(raw, "://") {
		u, parseErr := url.Parse(raw)
		if parseErr != nil {
			return "", "", "", useSSL, fmt.Errorf("endpoint 格式无效: %w", parseErr)
		}
		switch strings.ToLower(u.Scheme) {
		case "https":
			useSSL = true
		case "http":
			useSSL = false
		}
		host = u.Host
		extraPath = strings.Trim(u.Path, "/")
	} else if idx := strings.Index(raw, "/"); idx >= 0 {
		host = raw[:idx]
		extraPath = strings.Trim(raw[idx+1:], "/")
	}

	host = strings.TrimSpace(strings.TrimSuffix(host, "/"))
	if host == "" {
		return "", "", "", useSSL, errors.New("S3 endpoint 未配置")
	}

	if bucket == "" {
		if b, ep := splitVirtualHostBucket(host); b != "" {
			bucket = b
			host = ep
		}
	}

	if extraPath != "" {
		if bucket == "" {
			parts := strings.SplitN(extraPath, "/", 2)
			bucket = parts[0]
			if len(parts) > 1 && prefix == "" {
				prefix = parts[1]
			}
		} else if prefix == "" {
			prefix = extraPath
		}
	}

	if bucket == "" {
		return "", "", "", useSSL, errors.New("S3 bucket 未配置")
	}
	return host, bucket, prefix, useSSL, nil
}

func splitVirtualHostBucket(host string) (bucket, endpoint string) {
	lower := strings.ToLower(host)
	// 阿里云 OSS：<bucket>.oss-<region>.aliyuncs.com
	if i := strings.Index(lower, ".oss-"); i > 0 && strings.HasSuffix(lower, ".aliyuncs.com") {
		return host[:i], host[i+1:]
	}
	// AWS 虚拟主机：<bucket>.s3.<region>.amazonaws.com
	if i := strings.Index(lower, ".s3."); i > 0 {
		return host[:i], host[i+4:]
	}
	return "", host
}

func (d *S3Driver) objectKey(virtualPath string) string {
	rel := strings.TrimPrefix(NormalizeVirtual(virtualPath), "/")
	if d.prefix == "" {
		return rel
	}
	if rel == "" {
		return d.prefix
	}
	return d.prefix + "/" + rel
}

func (d *S3Driver) virtualFromKey(key string) string {
	key = strings.TrimPrefix(key, "/")
	if d.prefix != "" {
		if key == d.prefix {
			return "/"
		}
		p := d.prefix + "/"
		if strings.HasPrefix(key, p) {
			key = strings.TrimPrefix(key, p)
		}
	}
	if key == "" {
		return "/"
	}
	return NormalizeVirtual("/" + key)
}

func (d *S3Driver) List(dirPath string) ([]FileItem, error) {
	prefix := d.objectKey(dirPath)
	if prefix != "" {
		prefix += "/"
	}
	ctx := context.Background()
	ch := d.client.ListObjects(ctx, d.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: false,
	})
	seen := map[string]bool{}
	items := make([]FileItem, 0)
	for obj := range ch {
		if obj.Err != nil {
			return nil, obj.Err
		}
		rel := strings.TrimPrefix(obj.Key, prefix)
		if rel == "" {
			continue
		}
		if strings.Contains(rel, "/") {
			name := strings.SplitN(rel, "/", 2)[0]
			if seen[name] {
				continue
			}
			seen[name] = true
			items = append(items, FileItem{
				Name:  name,
				Path:  JoinVirtual(dirPath, name),
				IsDir: true,
			})
			continue
		}
		name := strings.TrimSuffix(rel, "/")
		if name == "" {
			continue
		}
		isDir := strings.HasSuffix(obj.Key, "/")
		item := FileItem{
			Name:     name,
			Path:     JoinVirtual(dirPath, name),
			Size:     obj.Size,
			IsDir:    isDir,
			Modified: obj.LastModified.UTC(),
		}
		if !isDir {
			item.MimeType = mime.TypeByExtension(filepath.Ext(name))
		}
		items = append(items, item)
	}
	return items, nil
}

func (d *S3Driver) statKey(key string) (*FileItem, error) {
	ctx := context.Background()
	info, err := d.client.StatObject(ctx, d.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" {
			return nil, ErrNotFound
		}
		return nil, err
	}
	vpath := d.virtualFromKey(key)
	name := BaseName(vpath)
	return &FileItem{
		Name:     name,
		Path:     vpath,
		Size:     info.Size,
		IsDir:    false,
		Modified: info.LastModified.UTC(),
		MimeType: info.ContentType,
	}, nil
}

func (d *S3Driver) Mkdir(dirPath string) error {
	key := d.objectKey(dirPath)
	if key == "" {
		return nil
	}
	if !strings.HasSuffix(key, "/") {
		key += "/"
	}
	ctx := context.Background()
	_, err := d.client.PutObject(ctx, d.bucket, key, bytes.NewReader(nil), 0, minio.PutObjectOptions{})
	return err
}

func (d *S3Driver) MkdirAll(dirPath string) error {
	dirPath = NormalizeVirtual(dirPath)
	if dirPath == "/" {
		return nil
	}
	for _, p := range AncestorsVirtual(dirPath) {
		if err := d.Mkdir(p); err != nil {
			return err
		}
	}
	return nil
}

func (d *S3Driver) SaveFile(dirPath, filename string, data []byte) error {
	return d.SaveFileAt(JoinVirtual(dirPath, filename), data)
}

func (d *S3Driver) SaveFileAt(virtualPath string, data []byte) error {
	return d.SaveFileStream(virtualPath, bytes.NewReader(data), int64(len(data)))
}

func (d *S3Driver) SaveFileStream(virtualPath string, r io.Reader, size int64) error {
	key := d.objectKey(virtualPath)
	ctx := context.Background()
	_, err := d.client.PutObject(ctx, d.bucket, key, r, size, minio.PutObjectOptions{
		ContentType: mime.TypeByExtension(filepath.Ext(BaseName(virtualPath))),
	})
	return err
}

func (d *S3Driver) Delete(paths []string) error {
	ctx := context.Background()
	for _, p := range paths {
		key := d.objectKey(p)
		if err := d.client.RemoveObject(ctx, d.bucket, key, minio.RemoveObjectOptions{}); err != nil {
			return err
		}
		dirKey := strings.TrimSuffix(key, "/") + "/"
		ch := d.client.ListObjects(ctx, d.bucket, minio.ListObjectsOptions{Prefix: dirKey, Recursive: true})
		for obj := range ch {
			if obj.Err != nil {
				return obj.Err
			}
			if err := d.client.RemoveObject(ctx, d.bucket, obj.Key, minio.RemoveObjectOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *S3Driver) Rename(oldPath, newPath string) error {
	if err := d.Copy(oldPath, ParentVirtual(newPath)); err != nil {
		return err
	}
	return d.Delete([]string{oldPath})
}

func (d *S3Driver) Copy(srcPath, destDir string) error {
	srcKey := d.objectKey(srcPath)
	destKey := d.objectKey(JoinVirtual(destDir, BaseName(srcPath)))
	ctx := context.Background()
	src := minio.CopySrcOptions{Bucket: d.bucket, Object: srcKey}
	dst := minio.CopyDestOptions{Bucket: d.bucket, Object: destKey}
	_, err := d.client.CopyObject(ctx, dst, src)
	return err
}

func (d *S3Driver) Move(srcPath, destDir string) error {
	if err := d.Copy(srcPath, destDir); err != nil {
		return err
	}
	return d.Delete([]string{srcPath})
}

func (d *S3Driver) Open(filePath string) (string, *FileItem, error) {
	return "", nil, ErrUnsupported
}

func (d *S3Driver) ReadFile(filePath string) (io.ReadCloser, *FileItem, error) {
	key := d.objectKey(filePath)
	item, err := d.statKey(key)
	if err != nil {
		return nil, nil, err
	}
	ctx := context.Background()
	obj, err := d.client.GetObject(ctx, d.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, nil, err
	}
	return obj, item, nil
}

func (d *S3Driver) Test() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ok, err := d.client.BucketExists(ctx, d.bucket)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("bucket 不存在")
	}
	return nil
}
