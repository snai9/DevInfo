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

	// 先用 GET 轻量探测（更快更稳）
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(pingURL)
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return true
		}
	}

	// GET 不行再试 POST（兼容旧版本）
	status, _ := postJSON(pingURL, map[string]interface{}{}, timeout)
	return status >= 200 && status < 300
}

var defaultServerIPs = []string{
	"10.44.179.88",
	"10.130.175.88",
	"172.16.0.88",
	"10.27.0.1",
}

const basePort = 1414
const maxRounds = 2

func FindActiveServer(urls []string, cachedURL string) (string, bool) {
	// 1. 先试缓存的地址，通了直接用（最快路径）
	if cachedURL != "" && Ping(cachedURL, 2000) {
		return cachedURL, true
	}

	// 2. 按规则探测：4个固定IP，从 basePort 开始，最多两轮（basePort, basePort+1）
	for round := 0; round < maxRounds; round++ {
		port := basePort + round
		for _, ip := range defaultServerIPs {
			// 先用 TCP 快速测端口通不通（比HTTP快很多）
			if !tcpPing(ip, port, 800) {
				continue
			}
			// 端口通了，再测 HTTP 接口（放宽超时到2秒）
			url := fmt.Sprintf("http://%s:%d", ip, port)
			if Ping(url, 2000) {
				return url, true
			}
		}
	}

	// 3. 都探测不到，返回空和 false
	return "", false
}

// tcpPing 快速测试TCP端口是否可达
func tcpPing(ip string, port int, timeoutMs int) bool {
	addr := fmt.Sprintf("%s:%d", ip, port)
	conn, err := net.DialTimeout("tcp", addr, time.Duration(timeoutMs)*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// DiagnoseServers 诊断各服务器的连通状态，用于错误提示
func DiagnoseServers() string {
	var lines []string
	for round := 0; round < maxRounds; round++ {
		port := basePort + round
		for _, ip := range defaultServerIPs {
			tcpOk := tcpPing(ip, port, 400)
			status := "  ✗ 端口不通"
			if tcpOk {
				// TCP通了再测HTTP
				url := fmt.Sprintf("http://%s:%d", ip, port)
				if Ping(url, 600) {
					status = "  ✓ 服务正常"
				} else {
					status = "  △ 端口通但服务未响应"
				}
			}
			lines = append(lines, fmt.Sprintf("  %s:%d%s", ip, port, status))
		}
	}
	return strings.Join(lines, "\n")
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
