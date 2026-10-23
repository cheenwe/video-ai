package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Host, Port, DataHome, SecretKey, JWTAlgorithm, CORSOrigins string
	StorageLocalRoot                                           string
	JWTExpireDays                                              int
	// Database
	DBDriver string // sqlite | mysql | postgres
	DBDSN    string // 完整 DSN（优先于拆分字段）
	DBHost, DBPort, DBUser, DBPassword, DBName, DBSSLMode string
	DBDumpBinPath string // mysqldump / pg_dump 可执行路径，空则 PATH 查找
}

func Load() (*Config, error) {
	home := os.Getenv("BASE_DATA_DIR")
	if home == "" {
		home = "./data"
	}
	absolute, err := filepath.Abs(home)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(absolute, 0700); err != nil {
		return nil, err
	}
	key := os.Getenv("BASE_SECRET_KEY")
	if key == "" {
		path := filepath.Join(absolute, "session.key")
		b, e := os.ReadFile(path)
		if os.IsNotExist(e) {
			raw := make([]byte, 32)
			if _, err = rand.Read(raw); err != nil {
				return nil, err
			}
			b = []byte(hex.EncodeToString(raw))
			f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if e != nil {
				return nil, e
			}
			_, err = f.Write(b)
			closeErr := f.Close()
			if err != nil {
				return nil, err
			}
			if closeErr != nil {
				return nil, closeErr
			}
		} else if e != nil {
			return nil, e
		}
		key = string(b)
	}
	if len(key) < 32 {
		return nil, fmt.Errorf("BASE_SECRET_KEY must contain at least 32 bytes")
	}
	host := os.Getenv("BASE_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := os.Getenv("BASE_PORT")
	if port == "" {
		port = "8181"
	}
	// 数据库配置
	driver := strings.ToLower(strings.TrimSpace(os.Getenv("BASE_DB_DRIVER")))
	if driver == "" {
		driver = "sqlite"
	}
	if driver != "sqlite" && driver != "mysql" && driver != "postgres" {
		return nil, fmt.Errorf("BASE_DB_DRIVER 仅支持 sqlite / mysql / postgres，当前值：%s", driver)
	}
	dbName := strings.TrimSpace(os.Getenv("BASE_DB_NAME"))
	if driver != "sqlite" && dbName == "" {
		dbName = "baseplatform"
	}
	dbPort := strings.TrimSpace(os.Getenv("BASE_DB_PORT"))
	if driver == "mysql" && dbPort == "" {
		dbPort = "3306"
	}
	if driver == "postgres" && dbPort == "" {
		dbPort = "5432"
	}
	return &Config{
		Host: host, Port: port, DataHome: absolute, SecretKey: key,
		JWTAlgorithm: "HS256", JWTExpireDays: 1,
		StorageLocalRoot: filepath.Join(absolute, "storage"),
		DBDriver:         driver,
		DBDSN:            strings.TrimSpace(os.Getenv("BASE_DB_DSN")),
		DBHost:           strings.TrimSpace(os.Getenv("BASE_DB_HOST")),
		DBPort:           dbPort,
		DBUser:           strings.TrimSpace(os.Getenv("BASE_DB_USER")),
		DBPassword:       os.Getenv("BASE_DB_PASSWORD"),
		DBName:           dbName,
	DBSSLMode:           strings.TrimSpace(os.Getenv("BASE_DB_SSLMODE")),
	DBDumpBinPath:    strings.TrimSpace(os.Getenv("BASE_DB_DUMP_BIN")),
	}, nil
}

// DSN 返回 mysql/postgres 的连接串。优先使用显式 DSN，否则用拆分字段构造。
// sqlite 不走此函数（在 database.Open 里直接拼文件路径）。
func (c *Config) DSN() string {
	if c.DBDriver == "sqlite" {
		return ""
	}
	if c.DBDSN != "" {
		return c.DBDSN
	}
	switch c.DBDriver {
	case "mysql":
		// 参考 gorm.io/driver/mysql 的 DSN：user:pass@tcp(host:port)/dbname?...
		var sslParam string
		if c.DBSSLMode == "skip-verify" {
			sslParam = "&tls=skip-verify"
		}
		return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local%s",
			c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, sslParam)
	case "postgres":
		sslmode := c.DBSSLMode
		if sslmode == "" {
			sslmode = "require"
		}
		return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=Asia/Shanghai",
			c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, sslmode)
	}
	return ""
}

// ProtectedAbsRoots 返回禁止通过文件接口删除/改写的绝对路径（系统数据区）。
func (c *Config) ProtectedAbsRoots() []string {
	if c == nil {
		return nil
	}
	raw := []string{
		c.DataHome,
		filepath.Join(c.DataHome, "avatars"),
		filepath.Join(c.DataHome, "uploads"),
		filepath.Join(c.DataHome, "versions"),
	}
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = filepath.Clean(p)
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		out = append(out, abs)
	}
	return out
}
