// Package nvr talks to nvr-agent instances registered by administrators.
package nvr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// maxJSONBody bounds agent JSON responses; config and channel lists are small.
const maxJSONBody = 4 << 20

var (
	idPattern   = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	filePattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,200}$`)
)

// ValidCameraID reports whether id is a camera id accepted by nvr-agent.
func ValidCameraID(id string) bool { return idPattern.MatchString(id) }

// ValidFileName reports whether name is a plain media file name (no path parts).
func ValidFileName(name string) bool {
	return filePattern.MatchString(name) && !strings.Contains(name, "..")
}

// NormalizeBaseURL validates an agent address such as http://192.168.1.20:8090.
func NormalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("地址格式应为 http://IP:端口，例如 http://127.0.0.1:8090")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", errors.New("地址只能包含协议、主机和端口")
	}
	if u.Hostname() == "" {
		return "", errors.New("地址缺少主机")
	}
	return u.Scheme + "://" + u.Host, nil
}

// Error is a failed agent call. Status is 0 when the agent was unreachable.
type Error struct {
	Status int
	Msg    string
}

func (e *Error) Error() string { return e.Msg }

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

var (
	apiTransport = &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 40 * time.Second,
	}
	apiClient   = &http.Client{Transport: apiTransport, Timeout: 45 * time.Second}
	mediaClient = &http.Client{Transport: apiTransport}
)

func New(baseURL, token string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, HTTP: apiClient}
}

func (c *Client) newRequest(ctx context.Context, method, path string, query url.Values, body io.Reader) (*http.Request, error) {
	target := c.BaseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// JSON performs an API call and returns the raw JSON body of a 2xx response.
func (c *Client) JSON(ctx context.Context, method, path string, query url.Values, payload []byte) (json.RawMessage, error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := c.newRequest(ctx, method, path, query, body)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, &Error{Msg: "无法连接 nvr-agent：" + unreachableReason(err)}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxJSONBody+1))
	if err != nil {
		return nil, &Error{Status: resp.StatusCode, Msg: "读取 nvr-agent 响应失败：" + err.Error()}
	}
	if len(data) > maxJSONBody {
		return nil, &Error{Status: resp.StatusCode, Msg: "nvr-agent 响应过大"}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &Error{Status: resp.StatusCode, Msg: AgentErrorMessage(resp.StatusCode, data)}
	}
	if !json.Valid(data) {
		return nil, &Error{Status: resp.StatusCode, Msg: "nvr-agent 返回了非 JSON 内容，请确认地址指向 nvr-agent"}
	}
	return data, nil
}

// Stream opens a media request; the caller must close the response body.
func (c *Client) Stream(ctx context.Context, path string, query url.Values, header http.Header) (*http.Response, error) {
	req, err := c.newRequest(ctx, http.MethodGet, path, query, nil)
	if err != nil {
		return nil, err
	}
	for _, k := range []string{"Range", "If-Range", "If-Modified-Since", "If-None-Match"} {
		if v := header.Get(k); v != "" {
			req.Header.Set(k, v)
		}
	}
	resp, err := mediaClient.Do(req)
	if err != nil {
		return nil, &Error{Msg: "无法连接 nvr-agent：" + unreachableReason(err)}
	}
	return resp, nil
}

// AgentErrorMessage turns an agent error body ({"error": "..."}) into a message.
func AgentErrorMessage(status int, body []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error != "" {
		if status == http.StatusUnauthorized {
			return "nvr-agent 拒绝访问：Token 不正确"
		}
		return e.Error
	}
	if status == http.StatusUnauthorized {
		return "nvr-agent 拒绝访问：Token 不正确"
	}
	return fmt.Sprintf("nvr-agent 返回 HTTP %d", status)
}

func unreachableReason(err error) string {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "连接超时"
	}
	msg := err.Error()
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "actively refused") {
		return "连接被拒绝（nvr-agent 未运行，或监听地址不是 0.0.0.0）"
	}
	if i := strings.LastIndex(msg, ": "); i >= 0 {
		return msg[i+2:]
	}
	return msg
}
