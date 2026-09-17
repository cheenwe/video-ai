package services

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"baseplatform/internal/config"

	"gorm.io/gorm"
)

// dbDumpName 是数据库导出文件在 tar 归档中的路径名。
const dbDumpName = "db-dump.sql"

type BackupManifest struct {
	FormatVersion int       `json:"format_version"`
	SHA256        string    `json:"sha256"`
	ID            string    `json:"id"`
	CreatedAt     time.Time `json:"created_at"`
	ArchivePath   string    `json:"archive_path"`
	SizeBytes     int64     `json:"size_bytes"`
	DataHome      string    `json:"data_home"`
	Note          string    `json:"note"`
	DBDriver      string    `json:"db_driver"`               // sqlite | mysql | postgres
	DBDumpName    string    `json:"db_dump_name"`            // 归档内 DB 导出文件名（空=未导出）
	DBDumpBytes   int64     `json:"db_dump_bytes"`           // DB 导出文件大小
	DBDumpError   string    `json:"db_dump_error,omitempty"` // 导出失败原因
}

type BackupStatus struct {
	RestorePath string `json:"restore_path,omitempty"`
	State       string `json:"state"`
	Phase       string `json:"phase"`
	Percent     int    `json:"percent"`
	Message     string `json:"message"`
	Error       string `json:"error"`
	BackupID    string `json:"backup_id"`
}

type OpsBackupService struct {
	Cfg    *config.Config
	DB     *gorm.DB
	mu     sync.Mutex
	status BackupStatus
}

func NewOpsBackupService(cfg *config.Config, db *gorm.DB) *OpsBackupService {
	return &OpsBackupService{Cfg: cfg, DB: db, status: BackupStatus{State: "idle"}}
}

func (s *OpsBackupService) BackupDir() string {
	if v := strings.TrimSpace(os.Getenv("OPS_BACKUP_DIR")); v != "" {
		return v
	}
	return filepath.Join(s.Cfg.DataHome, "backups")
}

func (s *OpsBackupService) keepCount() int {
	n := 7
	if v := strings.TrimSpace(os.Getenv("OPS_BACKUP_KEEP")); v != "" {
		var parsed int
		if _, err := fmt.Sscanf(v, "%d", &parsed); err == nil && parsed > 0 {
			n = parsed
		}
	}
	return n
}

func (s *OpsBackupService) Status() BackupStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *OpsBackupService) setStatus(st BackupStatus) {
	s.mu.Lock()
	s.status = st
	s.mu.Unlock()
}

func (s *OpsBackupService) fail(err error) {
	s.setStatus(BackupStatus{State: "failed", Phase: "error", Percent: 0, Message: "失败", Error: err.Error()})
}

// dumpBinary 查找 mysqldump / pg_dump 的可执行路径。
func (s *OpsBackupService) dumpBinary(name string) (string, error) {
	if s.Cfg.DBDumpBinPath != "" {
		if _, err := os.Stat(s.Cfg.DBDumpBinPath); err == nil {
			return s.Cfg.DBDumpBinPath, nil
		}
	}
	return exec.LookPath(name)
}

// writeDBDump 将数据库导出内容写入 tar writer，返回导出信息。
func (s *OpsBackupService) writeDBDump(ctx context.Context, tw *tar.Writer) (int64, string, error) {
	driver := s.Cfg.DBDriver
	if driver != "sqlite" && driver != "" && s.Cfg.DBDSN != "" {
		return 0, "", errors.New("外部数据库备份需使用拆分连接配置，以确保导出连接与服务连接一致")
	}
	if driver == "" {
		driver = "sqlite"
	}
	switch driver {
	case "sqlite":
		return s.dumpSQLite(ctx, tw)
	case "mysql":
		return s.dumpMySQL(ctx, tw)
	case "postgres":
		return s.dumpPostgres(ctx, tw)
	}
	return 0, "", fmt.Errorf("不支持的驱动：%s", driver)
}

// dumpSQLite 用 VACUUM INTO 生成一致性快照，写入 tar（以 db-dump.sql 命名但实际为 SQLite DB 二进制）。
func (s *OpsBackupService) dumpSQLite(ctx context.Context, tw *tar.Writer) (int64, string, error) {
	if s.DB == nil {
		return 0, "", errors.New("DB 未初始化")
	}
	snapshot := filepath.Join(s.BackupDir(), ".tmp-snapshot-"+time.Now().UTC().Format("20060102-150405")+".db")
	// SQLite VACUUM INTO 不支持参数绑定，路径用单引号包裹（单引号转义为双单引号）
	escaped := strings.ReplaceAll(snapshot, "'", "''")
	sql := fmt.Sprintf("VACUUM INTO '%s'", escaped)
	if err := s.DB.WithContext(ctx).Exec(sql).Error; err != nil {
		_ = os.Remove(snapshot)
		return 0, "", fmt.Errorf("VACUUM INTO 失败: %w", err)
	}
	defer os.Remove(snapshot)
	info, err := os.Stat(snapshot)
	if err != nil {
		return 0, "", err
	}
	f, err := os.Open(snapshot)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	hdr := &tar.Header{
		Name:     "baseplatform.db",
		Size:     info.Size(),
		Mode:     0o600,
		Typeflag: tar.TypeReg,
		Format:   tar.FormatGNU,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return 0, "", err
	}
	n, err := io.Copy(tw, f)
	return n, "baseplatform.db", err
}

// dumpMySQL 用 mysqldump 导出到临时文件，再写入 tar。
func (s *OpsBackupService) dumpMySQL(ctx context.Context, tw *tar.Writer) (int64, string, error) {
	bin, err := s.dumpBinary("mysqldump")
	if err != nil {
		return 0, "", fmt.Errorf("未找到 mysqldump: %w", err)
	}
	tmpFile := filepath.Join(s.BackupDir(), ".tmp-mysqldump-"+time.Now().UTC().Format("20060102-150405")+".sql")
	args := []string{
		"--host=" + s.Cfg.DBHost,
		"--port=" + s.Cfg.DBPort,
		"--user=" + s.Cfg.DBUser,
		"--single-transaction",
		"--routines",
		"--triggers",
		"--set-gtid-purged=OFF",
		"--result-file=" + tmpFile,
		s.Cfg.DBName,
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), "MYSQL_PWD="+s.Cfg.DBPassword)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(tmpFile)
		return 0, "", fmt.Errorf("mysqldump 失败: %w, stderr: %s", err, stderr.String())
	}
	defer os.Remove(tmpFile)
	return writeFileToTar(tw, tmpFile, dbDumpName)
}

// dumpPostgres 用 pg_dump 导出到临时文件，再写入 tar。
func (s *OpsBackupService) dumpPostgres(ctx context.Context, tw *tar.Writer) (int64, string, error) {
	bin, err := s.dumpBinary("pg_dump")
	if err != nil {
		return 0, "", fmt.Errorf("未找到 pg_dump: %w", err)
	}
	tmpFile := filepath.Join(s.BackupDir(), ".tmp-pgdump-"+time.Now().UTC().Format("20060102-150405")+".sql")
	args := []string{
		"--host=" + s.Cfg.DBHost,
		"--port=" + s.Cfg.DBPort,
		"--username=" + s.Cfg.DBUser,
		"--dbname=" + s.Cfg.DBName,
		"--file=" + tmpFile,
		"--format=plain",
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+s.Cfg.DBPassword)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(tmpFile)
		return 0, "", fmt.Errorf("pg_dump 失败: %w, stderr: %s", err, stderr.String())
	}
	defer os.Remove(tmpFile)
	return writeFileToTar(tw, tmpFile, dbDumpName)
}

// writeFileToTar 把一个文件写入 tar 中指定名称。
func writeFileToTar(tw *tar.Writer, path, name string) (int64, string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	hdr := &tar.Header{
		Name:     name,
		Size:     info.Size(),
		Mode:     0o600,
		Typeflag: tar.TypeReg,
		Format:   tar.FormatGNU,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return 0, "", err
	}
	n, err := io.Copy(tw, f)
	return n, name, err
}

func (s *OpsBackupService) Create() (*BackupManifest, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	return s.CreateContext(ctx)
}
func (s *OpsBackupService) CreateContext(ctx context.Context) (*BackupManifest, error) {
	s.mu.Lock()
	if s.status.State == "running" {
		s.mu.Unlock()
		return nil, errors.New("已有备份任务在执行")
	}
	s.status = BackupStatus{State: "running", Phase: "prepare", Percent: 5, Message: "准备备份"}
	s.mu.Unlock()

	id := time.Now().UTC().Format("20060102-150405") + "-" + uuid.NewString()
	dir := s.BackupDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		s.fail(err)
		return nil, err
	}
	archive := filepath.Join(dir, "baseplatform-"+id+".tar.gz")
	manifestPath := filepath.Join(dir, "baseplatform-"+id+".manifest.json")

	s.setStatus(BackupStatus{State: "running", Phase: "pack", Percent: 20, Message: "打包数据", BackupID: id})

	f, err := os.OpenFile(archive, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		s.fail(err)
		return nil, err
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	dataHome := filepath.Clean(s.Cfg.DataHome)
	backupAbs := filepath.Clean(dir)
	err = filepath.Walk(dataHome, func(path string, info os.FileInfo, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if walkErr != nil {
			return walkErr
		}
		if info == nil {
			return nil
		}
		if path == dataHome {
			return nil
		}
		if path == backupAbs || strings.HasPrefix(path, backupAbs+string(os.PathSeparator)) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dataHome, path)
		if err != nil {
			return err
		}
		base := filepath.Base(path)
		if base == "baseplatform.db" || base == "baseplatform.db-wal" || base == "baseplatform.db-shm" || base == "db-dump.sql" || base == "restore-staging" || strings.HasPrefix(base, ".tmp-") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("备份不支持符号链接: %s", rel)
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() || !info.Mode().IsRegular() {
			return nil
		}
		rf, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, contextReader{ctx: ctx, r: rf})
		_ = rf.Close()
		return copyErr
	})
	if err != nil {
		_ = tw.Close()
		_ = gw.Close()
		_ = f.Close()
		_ = os.Remove(archive)
		s.fail(err)
		return nil, err
	}

	// 数据库导出
	driver := s.Cfg.DBDriver
	if driver == "" {
		driver = "sqlite"
	}
	mDumpName := ""
	mDumpBytes := int64(0)
	mDumpError := ""
	if driver != "sqlite" || s.DB != nil {
		s.setStatus(BackupStatus{State: "running", Phase: "db_dump", Percent: 60, Message: "导出数据库", BackupID: id})
		n, name, derr := s.writeDBDump(ctx, tw)
		if derr != nil {
			_ = tw.Close()
			_ = gw.Close()
			_ = f.Close()
			_ = os.Remove(archive)
			s.fail(derr)
			return nil, derr
		} else {
			mDumpName = name
			mDumpBytes = n
		}
	}

	for _, e := range []error{tw.Close(), gw.Close(), f.Close()} {
		if e != nil {
			_ = os.Remove(archive)
			s.fail(e)
			return nil, e
		}
	}
	digest, err := fileDigest(archive)
	if err != nil {
		s.fail(err)
		return nil, err
	}
	st, err := os.Stat(archive)
	if err != nil {
		s.fail(err)
		return nil, err
	}
	m := &BackupManifest{
		FormatVersion: 2, SHA256: digest,
		ID:          id,
		CreatedAt:   time.Now().UTC(),
		ArchivePath: archive,
		SizeBytes:   st.Size(),
		DataHome:    dataHome,
		Note:        "baseplatform backup",
		DBDriver:    driver,
		DBDumpName:  mDumpName,
		DBDumpBytes: mDumpBytes,
		DBDumpError: mDumpError,
	}
	raw, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(manifestPath, raw, 0o600); err != nil {
		s.fail(err)
		return nil, err
	}
	_ = s.Prune()
	s.setStatus(BackupStatus{State: "success", Phase: "done", Percent: 100, Message: "备份完成", BackupID: id})
	return m, nil
}

func (s *OpsBackupService) List() ([]BackupManifest, error) {
	dir := s.BackupDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []BackupManifest{}, nil
		}
		return nil, err
	}
	var out []BackupManifest
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".manifest.json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var m BackupManifest
		if json.Unmarshal(raw, &m) == nil {
			out = append(out, m)
		}
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].CreatedAt.After(out[i].CreatedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

func (s *OpsBackupService) Get(id string) (*BackupManifest, error) {
	id = filepath.Base(strings.TrimSpace(id))
	path := filepath.Join(s.BackupDir(), "baseplatform-"+id+".manifest.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m BackupManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *OpsBackupService) Delete(id string) error {
	id = filepath.Base(strings.TrimSpace(id))
	dir := s.BackupDir()
	_ = os.Remove(filepath.Join(dir, "baseplatform-"+id+".tar.gz"))
	return os.Remove(filepath.Join(dir, "baseplatform-"+id+".manifest.json"))
}

func (s *OpsBackupService) Prune() error {
	list, err := s.List()
	if err != nil {
		return err
	}
	keep := s.keepCount()
	if len(list) <= keep {
		return nil
	}
	for _, m := range list[keep:] {
		_ = s.Delete(m.ID)
	}
	return nil
}

func (s *OpsBackupService) ArchivePath(id string) (string, error) {
	if !regexp.MustCompile(`^[a-zA-Z0-9-]+$`).MatchString(id) {
		return "", errors.New("备份ID无效")
	}
	p := filepath.Join(s.BackupDir(), "baseplatform-"+id+".tar.gz")
	if _, err := os.Stat(p); err != nil {
		return "", err
	}
	return p, nil
}

func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Restore creates an independently verified recovery directory. Live data is never overwritten.
func (s *OpsBackupService) Restore(id string) error {
	s.mu.Lock()
	if s.status.State == "running" {
		s.mu.Unlock()
		return errors.New("已有维护任务运行中")
	}
	s.status = BackupStatus{State: "running", Phase: "restore_verify", Message: "校验恢复包"}
	s.mu.Unlock()
	target := filepath.Join(s.Cfg.DataHome, "restore-staging", id+"-"+uuid.NewString())
	if err := s.RestoreTo(id, target); err != nil {
		s.fail(err)
		return err
	}
	s.setStatus(BackupStatus{State: "prepared", Phase: "restore_prepared", Percent: 100, BackupID: id, RestorePath: target, Message: "恢复目录已验证。停止服务后将 BASE_DATA_DIR 指向此目录并重新启动；当前数据未覆盖。"})
	return nil
}

// RestoreTo validates and extracts into a new directory, then atomically publishes it.
// The destination must not exist. This also provides an offline recovery entry point.
func (s *OpsBackupService) RestoreTo(id, target string) error {
	m, err := s.Get(id)
	if err != nil {
		return err
	}
	if m.FormatVersion != 2 || m.SHA256 == "" || m.DBDumpError != "" {
		return errors.New("旧版或不完整备份不可自动恢复，请创建新版备份")
	}
	driver := s.Cfg.DBDriver
	if driver == "" {
		driver = "sqlite"
	}
	if m.DBDriver != driver {
		return errors.New("备份数据库类型与当前配置不一致")
	}
	if driver != "sqlite" {
		return errors.New("外部数据库恢复需在独立数据库使用原生工具导入备份内 db-dump.sql；禁止覆盖在线数据库")
	}
	archive, err := s.ArchivePath(id)
	if err != nil {
		return err
	}
	digest, err := fileDigest(archive)
	if err != nil {
		return err
	}
	if digest != m.SHA256 {
		return errors.New("备份校验和不匹配")
	}
	if _, err = os.Lstat(target); !os.IsNotExist(err) {
		return errors.New("恢复目标必须不存在")
	}
	parent := filepath.Dir(target)
	if err = os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, ".restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	seen := map[string]bool{}
	var total int64
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
		name := filepath.FromSlash(h.Name)
		clean := filepath.Clean(name)
		if name == "" || filepath.IsAbs(name) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) || strings.Contains(h.Name, "\\") || strings.Contains(h.Name, ":") || seen[clean] {
			return errors.New("归档路径无效或重复")
		}
		seen[clean] = true
		out := filepath.Join(tmp, clean)
		if h.Typeflag == tar.TypeDir {
			if e = os.MkdirAll(out, 0700); e != nil {
				return e
			}
			continue
		}
		if h.Typeflag != tar.TypeReg || h.Size < 0 {
			return errors.New("归档包含不支持的文件类型")
		}
		total += h.Size
		if total > 20<<30 {
			return errors.New("恢复数据超过20GiB限制")
		}
		if e = os.MkdirAll(filepath.Dir(out), 0700); e != nil {
			return e
		}
		file, e := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		_, copyErr := io.CopyN(file, tr, h.Size)
		syncErr := file.Sync()
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if syncErr != nil {
			return syncErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	if !seen["baseplatform.db"] {
		return errors.New("备份缺少数据库快照")
	}
	check, err := gorm.Open(sqlite.Open(filepath.Join(tmp, "baseplatform.db")), &gorm.Config{})
	if err != nil {
		return err
	}
	sqlDB, err := check.DB()
	if err != nil {
		return err
	}
	var result string
	err = check.Raw("PRAGMA integrity_check").Scan(&result).Error
	if e := sqlDB.Close(); err == nil {
		err = e
	}
	if err != nil {
		return err
	}
	if result != "ok" {
		return errors.New("数据库完整性检查失败")
	}
	return os.Rename(tmp, target)
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(b)
}
