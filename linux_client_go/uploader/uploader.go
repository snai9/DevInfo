package uploader

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type Result struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func postJSON(url string, data interface{}, timeout time.Duration) (int, string) {
	payload, err := json.Marshal(data)
	if err != nil {
		return 0, err.Error()
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return 0, err.Error()
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "hwinfo-client-linux-go")

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func extractMsg(body string, defaultMsg string) string {
	if body == "" {
		return defaultMsg
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(body), &result); err == nil {
		if msg, ok := result["msg"].(string); ok && msg != "" {
			return msg
		}
	}
	re := regexp.MustCompile(`<[^>]+>`)
	plain := re.ReplaceAllString(body, " ")
	re2 := regexp.MustCompile(`\s+`)
	plain = strings.TrimSpace(re2.ReplaceAllString(plain, " "))
	if plain != "" && len(plain) < 300 {
		return plain
	}
	return defaultMsg
}

func Send(data interface{}, serverURL string) *Result {
	if serverURL == "" {
		return &Result{Code: -1, Msg: "未配置服务器地址"}
	}

	uploadURL := strings.TrimRight(serverURL, "/") + "/api/upload"
	status, body := postJSON(uploadURL, data, 10*time.Second)

	if status == 0 {
		return &Result{Code: -1, Msg: "无法连接服务器，请检查网络"}
	}

	if body == "" {
		if status >= 500 {
			return &Result{Code: -1, Msg: "服务器存储失败"}
		}
		if status >= 400 {
			return &Result{Code: -1, Msg: "请求格式错误"}
		}
		return &Result{Code: -1, Msg: "请求失败"}
	}

	var result Result
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		if status >= 500 {
			return &Result{Code: -1, Msg: extractMsg(body, "服务器存储失败")}
		}
		if status >= 400 {
			return &Result{Code: -1, Msg: extractMsg(body, "请求格式错误")}
		}
		return &Result{Code: -1, Msg: extractMsg(body, "请求失败")}
	}

	return &result
}

func Ping(serverURL string, timeoutMs int) bool {
	if serverURL == "" {
		return false
	}

	pingURL := strings.TrimRight(serverURL, "/") + "/api/ping"
	timeout := time.Duration(timeoutMs) * time.Millisecond
	status, _ := postJSON(pingURL, map[string]interface{}{}, timeout)
	return status >= 200 && status < 300
}

func FindActiveServer(urls []string) string {
	if len(urls) == 0 {
		return ""
	}

	localPrefixes := getLocalIPPrefixes()

	// 第一优先级：同网段且可达
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if isSameSubnet(u, localPrefixes) && Ping(u, 1500) {
			return u
		}
	}

	// 第二优先级：可达即可
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u != "" && Ping(u, 1500) {
			return u
		}
	}

	// 都不可达，返回第一个
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u != "" {
			return u
		}
	}
	return ""
}

func getLocalIPPrefixes() []string {
	var prefixes []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return prefixes
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil {
				continue
			}
			ones, _ := ipNet.Mask.Size()
			if ones >= 16 {
				// 取前两段作为网段标识，例如 10.44
				prefix := fmt.Sprintf("%d.%d.", ip[0], ip[1])
				prefixes = append(prefixes, prefix)
			}
		}
	}
	return prefixes
}

func isSameSubnet(serverURL string, localPrefixes []string) bool {
	// 从URL中提取IP或主机名
	host := serverURL
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")
	if idx := strings.Index(host, ":"); idx >= 0 {
		host = host[:idx]
	}
	if idx := strings.Index(host, "/"); idx >= 0 {
		host = host[:idx]
	}

	for _, prefix := range localPrefixes {
		if strings.HasPrefix(host, prefix) {
			return true
		}
	}
	return false
}

func PrintResult(r *Result) {
	if r.Code == 0 {
		fmt.Println("✓ 上传成功")
	} else if r.Code == 1 {
		fmt.Println("✓ 记录已存在，无需重复上传")
	} else {
		fmt.Printf("✗ 上传失败: %s\n", r.Msg)
	}
}
