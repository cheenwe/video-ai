package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const maxOutputBytes = 64 * 1024

var defaultBinAllowlist = []string{"rsync", "tar", "gzip", "zstd", "cp", "mv"}

type ExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

func binAllowlist() []string {
	raw := strings.TrimSpace(os.Getenv("SCHEDULE_BIN_ALLOWLIST"))
	if raw == "" {
		return append([]string{}, defaultBinAllowlist...)
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return append([]string{}, defaultBinAllowlist...)
	}
	return out
}

func pathRoots(dataHome string) []string {
	roots := []string{filepath.Clean(dataHome)}
	raw := strings.TrimSpace(os.Getenv("SCHEDULE_PATH_ROOTS"))
	if raw == "" {
		return roots
	}
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if abs, err := filepath.Abs(p); err == nil {
			roots = append(roots, filepath.Clean(abs))
		}
	}
	return roots
}

func resolveBin(bin string) (string, error) {
	bin = strings.TrimSpace(bin)
	if bin == "" {
		return "", errors.New("可执行文件不能为空")
	}
	allow := binAllowlist()
	base := filepath.Base(bin)
	allowed := false
	for _, a := range allow {
		if a == bin || a == base {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", fmt.Errorf("二进制不在允许列表中: %s", bin)
	}
	if filepath.IsAbs(bin) {
		st, err := os.Stat(bin)
		if err != nil {
			return "", fmt.Errorf("可执行文件不存在: %s", bin)
		}
		if st.IsDir() || st.Mode()&0o111 == 0 {
			return "", fmt.Errorf("路径不可执行: %s", bin)
		}
		return bin, nil
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return "", fmt.Errorf("找不到可执行文件 %s: %w", bin, err)
	}
	return path, nil
}

func expandArgs(templateJSON string, vars map[string]string) ([]string, error) {
	var tmpl []string
	if err := json.Unmarshal([]byte(templateJSON), &tmpl); err != nil {
		return nil, errors.New("参数模板必须是 JSON 字符串数组")
	}
	out := make([]string, 0, len(tmpl))
	for _, part := range tmpl {
		expanded := part
		for k, v := range vars {
			expanded = strings.ReplaceAll(expanded, "{{"+k+"}}", v)
		}
		if strings.Contains(expanded, "{{") && strings.Contains(expanded, "}}") {
			return nil, fmt.Errorf("未替换的占位符: %s", expanded)
		}
		out = append(out, expanded)
	}
	return out, nil
}

func validatePathArg(p, dataHome string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", nil
	}
	if strings.Contains(p, "..") {
		return "", errors.New("路径不允许包含 ..")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	for _, root := range pathRoots(dataHome) {
		rel, err := filepath.Rel(root, abs)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return abs, nil
		}
	}
	return "", fmt.Errorf("路径不在允许根目录内: %s", abs)
}

func sanitizeVars(vars map[string]string, dataHome string) (map[string]string, error) {
	out := make(map[string]string, len(vars))
	for k, v := range vars {
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		if key == "src" || key == "dst" || key == "workdir" {
			cleaned, err := validatePathArg(val, dataHome)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}
			out[key] = cleaned
			continue
		}
		if strings.ContainsAny(val, "\n\r\x00") {
			return nil, fmt.Errorf("%s 含非法字符", key)
		}
		out[key] = val
	}
	return out, nil
}

func runCommand(bin string, args []string, workdir string, timeoutSec int, dataHome string) (*ExecResult, error) {
	resolved, err := resolveBin(bin)
	if err != nil {
		return nil, err
	}
	if workdir != "" {
		wd, err := validatePathArg(workdir, dataHome)
		if err != nil {
			return nil, err
		}
		workdir = wd
	}
	if timeoutSec <= 0 {
		timeoutSec = 3600
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, resolved, args...)
	if workdir != "" {
		cmd.Dir = workdir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	start := time.Now()
	err = cmd.Run()
	res := &ExecResult{
		Stdout:   truncateOutput(stdout.String()),
		Stderr:   truncateOutput(stderr.String()),
		Duration: time.Since(start),
	}
	if ctx.Err() == context.DeadlineExceeded {
		res.ExitCode = -1
		return res, errors.New("命令执行超时")
	}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			res.ExitCode = ee.ExitCode()
			return res, fmt.Errorf("命令退出码 %d", res.ExitCode)
		}
		res.ExitCode = -1
		return res, err
	}
	res.ExitCode = 0
	return res, nil
}

func truncateOutput(s string) string {
	if len(s) <= maxOutputBytes {
		return s
	}
	return s[:maxOutputBytes] + "\n...[truncated]"
}
