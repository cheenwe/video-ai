package storage

import (
	"errors"
	"path"
	"path/filepath"
	"strings"
)

var ErrPathTraversal = errors.New("非法路径")
var ErrNotFound = errors.New("文件不存在")
var ErrExists = errors.New("同名项已存在")
var ErrNotEmpty = errors.New("目录非空")
var ErrUnsupported = errors.New("不支持的存储驱动")
var ErrProtected = errors.New("系统文件受保护，无法操作")

func NormalizeVirtual(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	clean := path.Clean(p)
	if clean == "." {
		return "/"
	}
	return clean
}

func JoinVirtual(base, name string) string {
	name = strings.Trim(name, "/")
	if name == "" {
		return NormalizeVirtual(base)
	}
	return NormalizeVirtual(base + "/" + name)
}

func ParentVirtual(p string) string {
	p = NormalizeVirtual(p)
	if p == "/" {
		return "/"
	}
	return path.Dir(p)
}

func BaseName(p string) string {
	return path.Base(NormalizeVirtual(p))
}

func AncestorsVirtual(p string) []string {
	p = NormalizeVirtual(p)
	if p == "/" {
		return nil
	}
	parts := strings.Split(strings.TrimPrefix(p, "/"), "/")
	dirs := make([]string, 0, len(parts))
	cur := ""
	for _, part := range parts {
		cur += "/" + part
		dirs = append(dirs, cur)
	}
	return dirs
}

// IsUnderVirtual 判断 target 是否位于 root 之下（含 root 自身）。
func IsUnderVirtual(root, target string) bool {
	root = NormalizeVirtual(root)
	target = NormalizeVirtual(target)
	if root == "/" {
		return true
	}
	return target == root || strings.HasPrefix(target, root+"/")
}

// RelVirtual 返回 target 相对 root 的路径；target 不在 root 下时返回 ErrPathTraversal。
func RelVirtual(root, target string) (string, error) {
	root = NormalizeVirtual(root)
	target = NormalizeVirtual(target)
	if !IsUnderVirtual(root, target) {
		return "", ErrPathTraversal
	}
	if target == root {
		return "", nil
	}
	return strings.TrimPrefix(target, root+"/"), nil
}

// ResolveUnderVirtual 将相对路径解析到 root 下的绝对虚拟路径，并拒绝越界。
func ResolveUnderVirtual(root, rel string) (string, error) {
	root = NormalizeVirtual(root)
	rel = strings.TrimSpace(rel)
	if rel == "" || rel == "." || rel == "/" {
		return root, nil
	}
	if strings.HasPrefix(rel, "/") {
		target := NormalizeVirtual(rel)
		if !IsUnderVirtual(root, target) {
			return "", ErrPathTraversal
		}
		return target, nil
	}
	rel = strings.Trim(rel, "/")
	if rel == "" || rel == "." {
		return root, nil
	}
	target := JoinVirtual(root, rel)
	if !IsUnderVirtual(root, target) {
		return "", ErrPathTraversal
	}
	return target, nil
}

// 系统进程数据与旁路文件名：列表中隐藏，并禁止删除/重命名/覆盖。
var protectedExactNames = map[string]bool{
	".nextfile":  true,
	"avatars":    true,
	"versions":   true,
	"uploads":    true,
	"ai_file.db": true,
	".ds_store":  true,
}

// IsProtectedName reports whether a single path segment / filename is system-managed.
func IsProtectedName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return false
	}
	lower := strings.ToLower(name)
	if protectedExactNames[lower] {
		return true
	}
	if strings.HasPrefix(name, "._") {
		return true
	}
	if strings.HasSuffix(lower, ".uploading") {
		return true
	}
	switch filepath.Ext(lower) {
	case ".db", ".sqlite", ".sqlite3":
		return true
	}
	if strings.HasSuffix(lower, ".db-wal") || strings.HasSuffix(lower, ".db-shm") || strings.HasSuffix(lower, ".db-journal") {
		return true
	}
	return false
}

// IsProtectedVirtualPath reports whether any segment of a virtual path is protected.
func IsProtectedVirtualPath(virtPath string) bool {
	virtPath = NormalizeVirtual(virtPath)
	if virtPath == "/" || virtPath == "" {
		return false
	}
	for _, p := range strings.Split(strings.Trim(virtPath, "/"), "/") {
		if IsProtectedName(p) {
			return true
		}
	}
	return false
}

// FilterProtectedEntries removes system / junk entries from a directory listing.
func FilterProtectedEntries(items []FileItem) []FileItem {
	if len(items) == 0 {
		return items
	}
	out := make([]FileItem, 0, len(items))
	for _, item := range items {
		name := item.Name
		if name == "" {
			name = BaseName(item.Path)
		}
		if IsProtectedName(name) {
			continue
		}
		out = append(out, item)
	}
	return out
}

// IsUnderAnyRoot reports whether absPath is equal to or under any of the given absolute roots.
func IsUnderAnyRoot(absPath string, roots []string) bool {
	absPath = filepath.Clean(absPath)
	for _, root := range roots {
		root = filepath.Clean(root)
		if root == "" {
			continue
		}
		if absPath == root {
			return true
		}
		rel, err := filepath.Rel(root, absPath)
		if err != nil {
			continue
		}
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return true
		}
	}
	return false
}
