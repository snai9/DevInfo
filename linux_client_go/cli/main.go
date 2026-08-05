package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"hwinfo-client/collector"
	"hwinfo-client/config"
	"hwinfo-client/dialog"
	"hwinfo-client/uploader"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type serverResult struct {
	URL string
	OK  bool
}

var logFile *os.File

func startLog() string {
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".config", "hwinfo")
	os.MkdirAll(logDir, 0755)
	logPath := filepath.Join(logDir, "run.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		logFile = f
	}
	return logPath
}

func logMsg(msg string) {
	ts := time.Now().Format("2006-01-02 15:04:05.000")
	line := fmt.Sprintf("[%s] %s\n", ts, msg)
	fmt.Print(line)
	if logFile != nil {
		logFile.WriteString(line)
		logFile.Sync()
	}
}

func main() {
	if len(os.Args) < 2 {
		guiMain()
		return
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "collect":
		cmdCollect()
	case "config":
		cmdConfig(args)
	case "ping":
		cmdPing(args)
	case "upload":
		cmdUpload(args)
	case "auto":
		guiMain()
	case "-h", "--help", "help":
		printHelp()
	default:
		fmt.Printf("未知命令: %s\n\n", cmd)
		printHelp()
	}
}

func guiMain() {
	if !dialog.Available() {
		fmt.Println("错误：未检测到弹框工具（kdialog 或 zenity）")
		fmt.Println()
		fmt.Println("统信UOS/DDE桌面请安装 kdialog：")
		fmt.Println("  sudo apt install kdialog -y")
		fmt.Println()
		fmt.Println("其它桌面请安装 zenity：")
		fmt.Println("  sudo apt install zenity -y")
		os.Exit(1)
	}

	// 启动日志
	logPath := startLog()
	logMsg("程序启动")

	// 立刻弹出进度框（给用户反馈，不白屏）
	prog := dialog.ShowProgress("资产信息采集系统", "正在初始化...")

	// 并发：采集硬件 + 探测服务器
	type hwResult struct {
		data *collector.HWData
		cost int64
	}
	hwCh := make(chan hwResult, 1)
	serverCh := make(chan serverResult, 1)

	start := time.Now().UnixMilli()
	go func() {
		t0 := time.Now().UnixMilli()
		d := collector.All()
		cost := time.Now().UnixMilli() - t0
		logMsg(fmt.Sprintf("硬件采集完成，耗时 %d ms", cost))
		hwCh <- hwResult{data: d, cost: cost}
	}()

	cfg := config.Load()

	go func() {
		t0 := time.Now().UnixMilli()
		url, ok := uploader.FindActiveServer(cfg.ServerURLs, cfg.ActiveURL)
		cost := time.Now().UnixMilli() - t0
		logMsg(fmt.Sprintf("服务器探测完成，结果: %v (url=%s)，耗时 %d ms", ok, url, cost))
		serverCh <- serverResult{URL: url, OK: ok}
	}()

	// 等硬件采集（快的那个先到）
	prog.SetText("正在采集硬件信息...")
	hw := <-hwCh
	totalHW := time.Now().UnixMilli() - start
	logMsg(fmt.Sprintf("硬件就绪，总耗时 %d ms", totalHW))

	data := hw.data
	data.Used = cfg.Used
	data.Unit = cfg.Unit
	data.Dept = cfg.Dept

	// 等服务器探测（可能比硬件慢）
	prog.SetText("正在连接服务器...")
	var sr serverResult
	select {
	case sr = <-serverCh:
	case <-time.After(10 * time.Second):
		sr = serverResult{URL: "", OK: false}
		logMsg("服务器探测超时（10秒）")
	}
	totalAll := time.Now().UnixMilli() - start
	logMsg(fmt.Sprintf("全部就绪，总耗时 %d ms", totalAll))
	prog.Close()

	if !sr.OK {
		diagnosis := uploader.DiagnoseServers()
		dialog.Error("无法连接服务器",
			fmt.Sprintf("未能探测到可用的服务器地址。\n\n诊断结果：\n%s\n\n请联系管理员：\n  1. 确认服务器程序是否已启动\n  2. 确认防火墙是否开放 1414/1415 端口\n  3. 确认服务器IP和端口是否正确\n\n日志文件：%s", diagnosis, logPath))
		logMsg("服务器探测失败，退出")
		return
	}
	cfg.ActiveURL = sr.URL

	if !guiConfirmEdit(data, cfg) {
		logMsg("用户取消")
		return
	}

	cfg.Used = data.Used
	cfg.Unit = data.Unit
	cfg.Dept = data.Dept
	cfg.Save()

	prog = dialog.ShowProgress("资产信息采集系统", "正在上传到服务器，请稍候...")
	result := uploader.Send(data, cfg.ActiveURL)
	prog.Close()

	uploader.PrintResult(result)
	logMsg(fmt.Sprintf("上传结果: code=%d msg=%s", result.Code, result.Msg))

	switch result.Code {
	case 0:
		dialog.Info("上传成功", "资产信息已成功上传到服务器！")
	case 1:
		dialog.Info("提示", "记录已存在，无需重复上传")
	default:
		dialog.Error("上传失败", result.Msg)
	}
}

func guiConfirmEdit(data *collector.HWData, cfg *config.Config) bool {

	var errMsg string
	for {
		networkLines := formatNetwork(data.Ips, data.Macs)

		summary := fmt.Sprintf(
			"请核对以下信息，确认无误后点击【是】上传。\n\n"+

				"━━━ 使用人信息 ━━━\n"+
				"  使用人：%s\n"+
				"  单  位：%s\n"+
				"  部  门：%s\n\n"+

				"━━━ 基本信息 ━━━\n"+
				"  计算机名：%s\n"+
				"  操作系统：%s\n"+
				"  计算机品牌：%s\n"+
				"  计算机编号：%s\n\n"+

				"━━━ 网络信息 ━━━\n%s\n\n"+

				"━━━ 软件信息 ━━━\n"+
				"  办公软件：%s\n"+
				"  安装日期：%s\n"+
				"  杀毒软件：%s\n"+
				"  安装日期：%s\n\n"+

				"━━━ 内存信息 ━━━\n%s\n\n"+

				"━━━ 硬盘信息 ━━━\n%s\n\n"+

				"━━━ 服务器 ━━━\n"+
				"  当前地址：%s\n\n"+

				"需要修改请点击【否】，选择要修改的项目。",

			show(data.Used, "未设置"),
			show(data.Unit, "未设置"),
			show(data.Dept, "未设置"),

			show(data.Hostname, ""),
			show(data.Osver, ""),
			show(data.PcBrand, ""),
			show(data.PcId, ""),

			networkLines,

			show(data.Office, ""),
			show(data.OfficeDate, ""),
			show(data.Antivirus, ""),
			show(data.AvDate, ""),

			formatMemories(data.Memories),
			formatDisks(data.Disks),

			show(cfg.ActiveURL, "未配置"),
		)

		if errMsg != "" {
			summary = "⚠️  " + errMsg + "\n\n" + summary
			errMsg = ""
		}

		if dialog.YesNo("资产信息采集系统 - 信息确认", summary) {
			if data.Used == "" {
				errMsg = "请填写使用人"
				continue
			}
			if data.Unit == "" {
				errMsg = "请填写单位"
				continue
			}
			if data.Dept == "" {
				errMsg = "请填写部门"
				continue
			}
			return true
		}

		if !guiEditOne(data, cfg) {
			return false
		}
	}
}

func guiEditOne(data *collector.HWData, cfg *config.Config) bool {
	menu := []string{
		"使用人",
		"单位",
		"部门",
		"服务器地址",
		"取消返回",
	}

	for {
		cmd := "kdialog"
		if dialog.Engine() == "zenity" {
			cmd = "zenity"
		}

		var choice string
		var ok bool

		if cmd == "kdialog" {
			args := []string{"--title", "选择要修改的项目", "--menu", "请选择要修改的信息："}
			for i, m := range menu {
				args = append(args, fmt.Sprintf("%d", i+1), m)
			}
			out, err := kdialogCmd(args...)
			if err != nil {
				return true
			}
			choice = strings.TrimSpace(out)
			ok = true
		} else {
			choice, ok = dialog.Entry("选择要修改的项目",
				"请输入要修改的项目编号：\n  1. 使用人\n  2. 单位\n  3. 部门\n  4. 服务器地址\n  5. 取消返回",
				"")
			if !ok {
				return true
			}
		}

		switch choice {
		case "1", "使用人":
			val, ok := dialog.Entry("修改使用人", "请输入使用人姓名：", data.Used)
			if ok {
				data.Used = val
			}
		case "2", "单位":
			val, ok := dialog.Entry("修改单位", "请输入所在单位：", data.Unit)
			if ok {
				data.Unit = val
			}
		case "3", "部门":
			val, ok := dialog.Entry("修改部门", "请输入部门及位置：", data.Dept)
			if ok {
				data.Dept = val
			}
		case "4", "服务器地址":
			urls := strings.Join(cfg.ServerURLs, ", ")
			val, ok := dialog.Entry("修改服务器地址",
				"请输入服务器地址（多个用逗号分隔）：", urls)
			if ok && val != "" {
				arr := []string{}
				for _, u := range strings.Split(val, ",") {
					u = strings.TrimSpace(u)
					if u != "" {
						arr = append(arr, u)
					}
				}
				if len(arr) > 0 {
					cfg.ServerURLs = arr
					cfg.ActiveURL = arr[0]
				}
			}
		case "5", "取消返回", "":
			return true
		default:
			continue
		}
		return true
	}
}

func kdialogCmd(args ...string) (string, error) {
	cmd := exec.Command("kdialog", args...)
	out, err := cmd.Output()
	return string(out), err
}

func show(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func join(arr []string) string {
	if len(arr) == 0 {
		return ""
	}
	return strings.Join(arr, ", ")
}

func formatMemories(mems []collector.MemoryInfo) string {
	if len(mems) == 0 {
		return ""
	}
	parts := []string{}
	for _, m := range mems {
		parts = append(parts, m.Model+" "+m.Size)
	}
	return strings.Join(parts, "; ")
}

func formatDisks(disks []collector.DiskInfo) string {
	if len(disks) == 0 {
		return "  （未获取）"
	}
	lines := []string{}
	for _, d := range disks {
		line := fmt.Sprintf("  %s | %s | SN:%s", d.Model, d.Size, d.Serial)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func formatNetwork(ips, macs []string) string {
	maxLen := len(ips)
	if len(macs) > maxLen {
		maxLen = len(macs)
	}
	lines := []string{}
	for i := 0; i < maxLen; i++ {
		ip := "（无）"
		mac := "（无）"
		if i < len(ips) {
			ip = ips[i]
		}
		if i < len(macs) {
			mac = macs[i]
		}
		lines = append(lines, fmt.Sprintf("  IP地址：%s\n  MAC地址：%s", ip, mac))
	}
	if len(lines) == 0 {
		return "  （未获取）"
	}
	return strings.Join(lines, "\n")
}

func printHelp() {
	fmt.Println("资产信息采集客户端 (Linux版)")
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  hwinfo-cli              # 图形界面模式（双击默认）")
	fmt.Println("  hwinfo-cli <命令> [选项]")
	fmt.Println()
	fmt.Println("命令:")
	fmt.Println("  collect    采集并显示硬件信息")
	fmt.Println("  config     查看或修改配置")
	fmt.Println("  ping       探测服务器是否可达")
	fmt.Println("  upload     采集并上传到服务器")
	fmt.Println("  auto       图形界面模式")
	fmt.Println()
	fmt.Println("config 选项:")
	fmt.Println("  --used 姓名        使用人姓名")
	fmt.Println("  --unit 单位        所在单位")
	fmt.Println("  --dept 部门        部门及位置")
	fmt.Println("  --server 地址      服务器地址(多个用逗号分隔)")
	fmt.Println()
	fmt.Println("ping/upload 选项:")
	fmt.Println("  --server 地址      指定服务器地址")
	fmt.Println()
	fmt.Println("示例:")
	fmt.Println("  hwinfo-cli config --used 张三 --unit XX单位 --dept XX部门")
	fmt.Println("  hwinfo-cli")
}

func cmdCollect() {
	data := collector.All()
	j, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(j))
}

func cmdConfig(args []string) {
	fs := flag.NewFlagSet("config", flag.ExitOnError)
	used := fs.String("used", "", "使用人姓名")
	unit := fs.String("unit", "", "所在单位")
	dept := fs.String("dept", "", "部门及位置")
	server := fs.String("server", "", "服务器地址(多个用逗号分隔)")
	fs.Parse(args)

	cfg := config.Load()
	changed := false

	if *used != "" {
		cfg.Used = *used
		changed = true
	}
	if *unit != "" {
		cfg.Unit = *unit
		changed = true
	}
	if *dept != "" {
		cfg.Dept = *dept
		changed = true
	}
	if *server != "" {
		urls := []string{}
		for _, u := range strings.Split(*server, ",") {
			u = strings.TrimSpace(u)
			if u != "" {
				urls = append(urls, u)
			}
		}
		if len(urls) > 0 {
			cfg.ServerURLs = urls
			cfg.ActiveURL = urls[0]
			changed = true
		}
	}

	if changed {
		cfg.Save()
		fmt.Println("配置已保存")
	} else {
		j, _ := json.MarshalIndent(cfg, "", "  ")
		fmt.Println(string(j))
	}
}

func cmdPing(args []string) {
	fs := flag.NewFlagSet("ping", flag.ExitOnError)
	server := fs.String("server", "", "服务器地址")
	fs.Parse(args)

	cfg := config.Load()
	url := *server
	if url == "" {
		url = cfg.ActiveURL
		if url == "" && len(cfg.ServerURLs) > 0 {
			url = cfg.ServerURLs[0]
		}
	}
	if url == "" {
		fmt.Println("未配置服务器地址")
		os.Exit(1)
	}

	ok := uploader.Ping(url, 3000)
	if ok {
		fmt.Printf("服务器 %s: 可达\n", url)
	} else {
		fmt.Printf("服务器 %s: 不可达\n", url)
		os.Exit(1)
	}
}

func cmdUpload(args []string) {
	fs := flag.NewFlagSet("upload", flag.ExitOnError)
	server := fs.String("server", "", "服务器地址")
	fs.Parse(args)

	cfg := config.Load()

	if cfg.Used == "" || cfg.Unit == "" || cfg.Dept == "" {
		fmt.Println("请先配置：hwinfo-cli config --used 张三 --unit XX单位 --dept XX部门")
		os.Exit(1)
	}

	url := *server
	if url == "" {
		url = cfg.ActiveURL
		if url == "" && len(cfg.ServerURLs) > 0 {
			url = cfg.ServerURLs[0]
		}
	}
	if url == "" {
		fmt.Println("未配置服务器地址")
		os.Exit(1)
	}

	fmt.Println("正在采集硬件信息...")
	data := collector.All()
	data.Used = cfg.Used
	data.Unit = cfg.Unit
	data.Dept = cfg.Dept

	fmt.Printf("正在上传到 %s ...\n", url)
	result := uploader.Send(data, url)
	uploader.PrintResult(result)

	if result.Code != 0 && result.Code != 1 {
		os.Exit(1)
	}
}
