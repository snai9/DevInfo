package collector

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

type HWData struct {
	Hostname   string       `json:"hostname"`
	Osver      string       `json:"osver"`
	PcId       string       `json:"pcId"`
	PcBrand    string       `json:"pcBrand"`
	Ips        []string     `json:"ips"`
	Macs       []string     `json:"macs"`
	Office     string       `json:"office"`
	OfficeDate string       `json:"officeDate"`
	Antivirus  string       `json:"antivirus"`
	AvDate     string       `json:"avDate"`
	Memories   []MemoryInfo `json:"memories"`
	Disks      []DiskInfo   `json:"disks"`
	Used       string       `json:"used"`
	Unit       string       `json:"unit"`
	Dept       string       `json:"dept"`
}

type MemoryInfo struct {
	Model string `json:"model"`
	Size  string `json:"size"`
}

type DiskInfo struct {
	Model  string `json:"model"`
	Size   string `json:"size"`
	Serial string `json:"serial"`
}

func runCmd(cmd string, timeout time.Duration) string {
	c := exec.Command("bash", "-c", cmd)
	done := make(chan string, 1)
	go func() {
		out, _ := c.CombinedOutput()
		done <- strings.TrimSpace(string(out))
	}()
	select {
	case out := <-done:
		return out
	case <-time.After(timeout):
		c.Process.Kill()
		return ""
	}
}

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func cleanSerial(s string) string {
	s = strings.TrimSpace(s)
	// 去掉OEM填充串，但只要有内容就保留
	s = strings.ReplaceAll(s, "To be filled by O.E.M.", "")
	s = strings.ReplaceAll(s, "ToBeFilledByO.E.M.", "")
	s = strings.ReplaceAll(s, "OEM", "")
	s = strings.TrimSpace(s)
	if s != "" && len(s) >= 2 {
		return s
	}
	return ""
}

func Hostname() string {
	name, err := os.Hostname()
	if err == nil && name != "" {
		return name
	}
	name = readFile("/proc/sys/kernel/hostname")
	if name != "" {
		return name
	}
	return "未获取"
}

func Network() (ips []string, macs []string) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return []string{"未获取"}, []string{"未获取"}
	}

	for _, iface := range ifaces {
		if iface.Name == "lo" {
			continue
		}
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		mac := iface.HardwareAddr.String()
		if mac != "" && mac != "00:00:00:00:00:00" {
			macs = append(macs, strings.ToUpper(mac))
		}

		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP
			if ip.IsLoopback() || !ip.IsPrivate() && !ip.IsGlobalUnicast() {
				continue
			}
			ipStr := ip.String()
			if ipStr != "0.0.0.0" && strings.Contains(ipStr, ".") {
				ips = append(ips, ipStr)
			}
		}
	}

	if len(ips) == 0 {
		ips = []string{"未获取"}
	}
	if len(macs) == 0 {
		macs = []string{"未获取"}
	}
	return
}

func Osver() string {
	data := readFile("/etc/os-release")
	if data != "" {
		name := ""
		version := ""
		for _, line := range strings.Split(data, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				re := regexp.MustCompile(`PRETTY_NAME="?(.+?)"?$`)
				m := re.FindStringSubmatch(line)
				if len(m) > 1 {
					return m[1]
				}
			}
			if strings.HasPrefix(line, "NAME=") {
				re := regexp.MustCompile(`NAME="?(.+?)"?$`)
				m := re.FindStringSubmatch(line)
				if len(m) > 1 {
					name = m[1]
				}
			}
			if strings.HasPrefix(line, "VERSION=") {
				re := regexp.MustCompile(`VERSION="?(.+?)"?$`)
				m := re.FindStringSubmatch(line)
				if len(m) > 1 {
					version = m[1]
				}
			}
		}
		if name != "" {
			return strings.TrimSpace(name + " " + version)
		}
	}

	ver := runCmd("lsb_release -d 2>/dev/null", 2*time.Second)
	if ver != "" {
		re := regexp.MustCompile(`Description:\s*(.+)`)
		m := re.FindStringSubmatch(ver)
		if len(m) > 1 {
			return m[1]
		}
	}

	procVer := readFile("/proc/version")
	if len(procVer) > 60 {
		return procVer[:60]
	}
	if procVer != "" {
		return procVer
	}
	return "未获取"
}

func PcId() string {
	// 优先级：出厂编号 > 主板BIOS编号 > 机箱编号
	paths := []string{
		"/sys/class/dmi/id/product_serial",
		"/sys/class/dmi/id/board_serial",
		"/sys/class/dmi/id/chassis_serial",
	}
	for _, p := range paths {
		sn := readFile(p)
		clean := cleanSerial(sn)
		if clean != "" {
			return clean
		}
		// 如果cleanSerial过滤掉了，但原文有内容，就直接返回原文
		sn = strings.TrimSpace(sn)
		if sn != "" && len(sn) >= 2 && strings.ToLower(sn) != "tobefilledbyo.e.m." {
			return sn
		}
	}

	// dmidecode 备用
	for _, dmiKey := range []string{"system-serial-number", "baseboard-serial-number", "chassis-serial-number"} {
		sn := runCmd("dmidecode -s "+dmiKey+" 2>/dev/null", 2*time.Second)
		clean := cleanSerial(sn)
		if clean != "" {
			return clean
		}
		sn = strings.TrimSpace(sn)
		if sn != "" && len(sn) >= 2 && strings.ToLower(sn) != "tobefilledbyo.e.m." {
			return sn
		}
	}

	return "未获取"
}

func PcBrand() string {
	mfr := readFile("/sys/class/dmi/id/sys_vendor")
	model := readFile("/sys/class/dmi/id/product_name")

	if mfr == "" {
		mfr = runCmd("dmidecode -s system-manufacturer 2>/dev/null", 2*time.Second)
	}
	if model == "" {
		model = runCmd("dmidecode -s system-product-name 2>/dev/null", 2*time.Second)
	}

	mfr = strings.TrimSpace(mfr)
	model = strings.TrimSpace(model)

	if mfr == "" && model == "" {
		return "未获取"
	}
	if mfr == "" {
		return model
	}
	if model == "" {
		return mfr
	}
	if strings.Contains(strings.ToLower(model), strings.ToLower(mfr)) {
		return model
	}
	return mfr + " " + model
}

func Memory() []MemoryInfo {
	var result []MemoryInfo

	dmiOut := runCmd("dmidecode --type 17 2>/dev/null", 3*time.Second)
	if dmiOut != "" {
		current := MemoryInfo{}
		hasData := false
		mfr := ""
		speed := ""
		part := ""

		for _, line := range strings.Split(dmiOut, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Size:") {
				if hasData {
					model := mfr
					if model == "" {
						model = "未知"
					}
					if speed != "" {
						model = model + " DDR" + speed[:1] + " " + speed
					} else if part != "" {
						model = model + " " + part
					}
					current.Model = model
					result = append(result, current)
				}
				current = MemoryInfo{}
				hasData = false
				mfr = ""
				speed = ""
				part = ""

				sizeStr := strings.TrimSpace(strings.TrimPrefix(line, "Size:"))
				if strings.Contains(sizeStr, "MB") {
					re := regexp.MustCompile(`(\d+)`)
					m := re.FindStringSubmatch(sizeStr)
					if len(m) > 1 {
						mb := 0
						fmt.Sscanf(m[1], "%d", &mb)
						if mb >= 1024 {
							current.Size = fmt.Sprintf("%d GB", mb/1024)
						} else {
							current.Size = fmt.Sprintf("%d MB", mb)
						}
						hasData = true
					}
				} else if strings.Contains(sizeStr, "GB") {
					current.Size = sizeStr
					hasData = true
				}
			} else if strings.HasPrefix(line, "Manufacturer:") && hasData {
				mfr = strings.TrimSpace(strings.TrimPrefix(line, "Manufacturer:"))
			} else if strings.HasPrefix(line, "Speed:") && hasData {
				sp := strings.TrimSpace(strings.TrimPrefix(line, "Speed:"))
				re := regexp.MustCompile(`(\d+)`)
				m := re.FindStringSubmatch(sp)
				if len(m) > 1 {
					speed = m[1]
				}
			} else if strings.HasPrefix(line, "Part Number:") && hasData {
				part = strings.TrimSpace(strings.TrimPrefix(line, "Part Number:"))
			}
		}

		if hasData {
			model := mfr
			if model == "" {
				model = "未知"
			}
			if speed != "" {
				model = model + " DDR" + speed[:1] + " " + speed
			} else if part != "" {
				model = model + " " + part
			}
			current.Model = model
			result = append(result, current)
		}
	}

	if len(result) == 0 {
		meminfo := readFile("/proc/meminfo")
		if meminfo != "" {
			re := regexp.MustCompile(`MemTotal:\s*(\d+)`)
			m := re.FindStringSubmatch(meminfo)
			if len(m) > 1 {
				kb := 0
				fmt.Sscanf(m[1], "%d", &kb)
				gb := kb / 1024 / 1024
				size := ""
				if gb > 0 {
					size = fmt.Sprintf("%d GB", gb)
				} else {
					size = fmt.Sprintf("%d MB", kb/1024)
				}
				result = append(result, MemoryInfo{Model: "系统内存", Size: size})
			}
		}
	}

	if len(result) == 0 {
		result = append(result, MemoryInfo{Model: "未获取", Size: "未获取"})
	}
	return result
}

type lsblkBlock struct {
	Name   string `json:"name"`
	Model  string `json:"model"`
	Size   string `json:"size"`
	Serial string `json:"serial"`
	Type   string `json:"type"`
}

type lsblkOutput struct {
	BlockDevices []lsblkBlock `json:"blockdevices"`
}

func Disk() []DiskInfo {
	var result []DiskInfo

	lsblkOut := runCmd("lsblk -J -d -o NAME,MODEL,SIZE,SERIAL,TYPE 2>/dev/null", 3*time.Second)
	if lsblkOut != "" {
		var out lsblkOutput
		if json.Unmarshal([]byte(lsblkOut), &out) == nil {
			for _, dev := range out.BlockDevices {
				if strings.HasPrefix(dev.Name, "loop") || strings.HasPrefix(dev.Name, "sr") {
					continue
				}
				if dev.Type != "" && dev.Type != "disk" {
					continue
				}
				model := dev.Model
				if model == "" {
					model = "未知"
				}
				size := dev.Size
				if size == "" {
					size = "未知"
				}
				serial := dev.Serial
				clean := cleanSerial(serial)
				if clean != "" {
					serial = clean
				}
				if serial == "" {
					serial = "未获取"
				}
				result = append(result, DiskInfo{
					Model:  strings.TrimSpace(model),
					Size:   strings.TrimSpace(size),
					Serial: strings.TrimSpace(serial),
				})
			}
		}
	}

	if len(result) == 0 {
		entries, _ := os.ReadDir("/sys/block")
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "sr") {
				continue
			}
			model := readFile("/sys/block/" + name + "/device/model")
			serial := readFile("/sys/block/" + name + "/device/serial")
			sizeStr := readFile("/sys/block/" + name + "/size")

			size := "未知"
			if sizeStr != "" {
				sectors := 0
				fmt.Sscanf(sizeStr, "%d", &sectors)
				bytes := float64(sectors) * 512
				gb := bytes / 1024 / 1024 / 1024
				if gb >= 1000 {
					size = fmt.Sprintf("%.1f TB", gb/1024)
				} else {
					size = fmt.Sprintf("%.0f GB", gb)
				}
			}

			if model == "" {
				model = "未知"
			}
			clean := cleanSerial(serial)
			if clean != "" {
				serial = clean
			}
			if serial == "" {
				serial = "未获取"
			}

			result = append(result, DiskInfo{
				Model:  strings.TrimSpace(model),
				Size:   size,
				Serial: strings.TrimSpace(serial),
			})
		}
	}

	if len(result) == 0 {
		result = append(result, DiskInfo{Model: "未获取", Size: "未获取", Serial: "未获取"})
	}
	return result
}

var avKeywords = []string{
	// 国产 - 关键词匹配
	"360safe", "360-safeguard", "360安全卫士", "360杀毒", "360安全",
	"huorong", "火绒安全", "火绒",
	"qax", "奇安信", "天擎", "tianqing",
	"sangfor", "edr-sangfor", "深信服",
	"rising", "瑞星杀毒", "瑞星",
	"腾讯电脑管家", "qqpcmgr", "qq电脑管家", "电脑管家",
	"uos-pc-manager", "uos安全中心", "uos-security", "uossecurity", "统信安全",
	"pc-manager", "pcmanager",
	"2345安全卫士", "2345杀毒",
	"jiangmin", "江民",
	"kingsoft", "金山毒霸",
	// 国外
	"mcafee", "norton", "symantec",
	"kaspersky", "卡巴斯基",
	"eset", "nod32",
	"avast", "avg", "bitdefender", "sophos",
	"trendmicro", "趋势科技",
	"drweb", "大蜘蛛",
	"avira", "小红伞",
	"panda", "熊猫",
	"comodo", "科摩多",
	"windefend",
	// microsoft defender 精确匹配
	"microsoft-defender", "microsoft defender",
}

// 必须排除的关键词（这些包虽然匹配了AV关键词，但不是真正的杀软）
var avExcludeKeywords = []string{
	"vdbup",       // 病毒库更新包
	"vdb",         // 病毒库
	"locale",      // 语言包
	"l10n",        // 本地化
	"i18n",        // 国际化
	"documentation", // 文档
	"-dev",        // 开发包
	"-dbg",        // 调试包
	"-dbggsym",    // 调试符号
	"-headers",    // 头文件
	"-sources",    // 源码
	"-examples",   // 示例
	"-tests",      // 测试
	"-data",       // 数据文件（除非明确是杀软数据）
	"-common",     // 通用组件
	"-libs",       // 库文件
	"-plugin",     // 插件
	"-icons",      // 图标
	"-themes",     // 主题
	"-translations", // 翻译
	"-lang",       // 语言
	"-help",       // 帮助
	"-manual",     // 手册
	"deepin-defender", // 统信自带的系统安全组件（不是独立杀软）
}

var officeKeywords = []string{
	"wps-office", "wps-office-pro", "kingsoft",
	"libreoffice", "libreoffice-base", "libreoffice-calc", "libreoffice-draw",
	"libreoffice-impress", "libreoffice-math", "libreoffice-writer",
	"永中office",
}

type pkgInfo struct {
	Version string
	Date    string
}

func getInstalledPackages() map[string]pkgInfo {
	pkgs := make(map[string]pkgInfo)

	// 一次性批量获取所有 dpkg 包的安装日期（避免每个包单独跑命令）
	dateMap := make(map[string]string)
	go func() {
		// 方法1: 用 stat 批量获取 mtime（更可靠）
		statOut := runCmd("stat -c '%n|%Y' /var/lib/dpkg/info/*.list 2>/dev/null", 2*time.Second)
		if statOut != "" {
			for _, line := range strings.Split(statOut, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				parts := strings.SplitN(line, "|", 2)
				if len(parts) == 2 {
					fileName := parts[0]
					tsStr := strings.TrimSpace(parts[1])
					pkgName := strings.TrimSuffix(filepath.Base(fileName), ".list")
					sec := int64(0)
					fmt.Sscanf(tsStr, "%d", &sec)
					if sec > 0 {
						dateMap[pkgName] = time.Unix(sec, 0).Format("2006-01-02")
					}
				}
			}
		}

		// 方法2: 如果 stat 失败，用 ls 兜底
		if len(dateMap) == 0 {
			lsOut := runCmd("ls -lt --time-style=+%Y-%m-%d /var/lib/dpkg/info/*.list 2>/dev/null", 2*time.Second)
			if lsOut != "" {
				for _, line := range strings.Split(lsOut, "\n") {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					fields := strings.Fields(line)
					var dateStr string
					var fileName string
					for _, f := range fields {
						if matched, _ := regexp.MatchString(`^\d{4}-\d{2}-\d{2}$`, f); matched {
							dateStr = f
						}
						if strings.HasSuffix(f, ".list") {
							fileName = f
						}
					}
					if dateStr != "" && fileName != "" {
						pkgName := strings.TrimSuffix(filepath.Base(fileName), ".list")
						dateMap[pkgName] = dateStr
					}
				}
			}
		}
	}()

	// dpkg 同时获取版本
	dpkgOut := runCmd("dpkg-query -W -f='${Package}|${Version}\\n' 2>/dev/null", 3*time.Second)
	if dpkgOut != "" {
		scanner := bufio.NewScanner(strings.NewReader(dpkgOut))
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.SplitN(line, "|", 2)
			if len(parts) >= 2 {
				name := strings.TrimSpace(parts[0])
				ver := strings.TrimSpace(parts[1])
				pkgs[name] = pkgInfo{Version: ver, Date: ""}
			}
		}
	}

	// rpm 同时获取版本和安装日期
	rpmOut := runCmd("rpm -qa --queryformat '%{NAME}|%{VERSION}|%{INSTALLTIME}\\n' 2>/dev/null", 3*time.Second)
	if rpmOut != "" {
		scanner := bufio.NewScanner(strings.NewReader(rpmOut))
		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.SplitN(line, "|", 3)
			if len(parts) >= 2 {
				name := strings.TrimSpace(parts[0])
				ver := strings.TrimSpace(parts[1])
				date := ""
				if len(parts) >= 3 {
					ts := strings.TrimSpace(parts[2])
					if ts != "" {
						sec := int64(0)
						fmt.Sscanf(ts, "%d", &sec)
						if sec > 0 {
							date = time.Unix(sec, 0).Format("2006-01-02")
						}
					}
				}
				if _, exists := pkgs[name]; !exists {
					pkgs[name] = pkgInfo{Version: ver, Date: date}
				}
			}
		}
	}

	// 把日期填进去（给上面的 goroutine 一点时间）
	time.Sleep(300 * time.Millisecond)
	for name, info := range pkgs {
		if d, ok := dateMap[name]; ok && d != "" {
			info.Date = d
			pkgs[name] = info
		}
	}

	return pkgs
}

func matchAV(name string) bool {
	lower := strings.ToLower(name)

	// 先检查排除关键词（只要匹配到排除关键词，就不是杀软）
	for _, kw := range avExcludeKeywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return false
		}
	}

	// 再检查匹配关键词
	for _, kw := range avKeywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

func matchOffice(name string) bool {
	lower := strings.ToLower(name)
	for _, kw := range officeKeywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

func Software() (office, officeDate, antivirus, avDate string) {
	pkgs := getInstalledPackages()

	var officeList []string
	var avList []string
	var officeDates []string
	var avDates []string

	for name, info := range pkgs {
		if matchOffice(name) {
			display := strings.ReplaceAll(strings.ReplaceAll(name, "-", " "), "_", " ")
			if info.Version != "" {
				display = display + " " + info.Version
			}
			officeList = append(officeList, display)
			if info.Date != "" {
				officeDates = append(officeDates, info.Date)
			}
		}
		if matchAV(name) {
			display := strings.ReplaceAll(strings.ReplaceAll(name, "-", " "), "_", " ")
			if info.Version != "" {
				display = display + " " + info.Version
			}
			avList = append(avList, display)
			if info.Date != "" {
				avDates = append(avDates, info.Date)
			}
		}
	}

	office = strings.Join(officeList, ";")
	antivirus = strings.Join(avList, ";")

	if office == "" {
		office = "未获取"
	}
	if antivirus == "" {
		antivirus = "未获取"
	}
	if len(officeDates) > 0 {
		officeDate = officeDates[len(officeDates)-1]
	} else {
		officeDate = "未获取"
	}
	if len(avDates) > 0 {
		avDate = avDates[len(avDates)-1]
	} else {
		avDate = "未获取"
	}

	return
}

func All() *HWData {
	if runtime.GOOS != "linux" {
		return &HWData{
			Hostname:   Hostname(),
			Osver:      runtime.GOOS,
			PcId:       "未获取",
			PcBrand:    "未获取",
			Ips:        []string{"未获取"},
			Macs:       []string{"未获取"},
			Office:     "未获取",
			OfficeDate: "未获取",
			Antivirus:  "未获取",
			AvDate:     "未获取",
			Memories:   []MemoryInfo{{Model: "未获取", Size: "未获取"}},
			Disks:      []DiskInfo{{Model: "未获取", Size: "未获取", Serial: "未获取"}},
		}
	}

	data := &HWData{}
	var wg sync.WaitGroup

	// 所有采集并发执行
	wg.Add(7)

	go func() {
		defer wg.Done()
		data.Hostname = Hostname()
	}()

	go func() {
		defer wg.Done()
		ips, macs := Network()
		data.Ips = ips
		data.Macs = macs
	}()

	go func() {
		defer wg.Done()
		data.Osver = Osver()
	}()

	go func() {
		defer wg.Done()
		data.PcId = PcId()
	}()

	go func() {
		defer wg.Done()
		data.PcBrand = PcBrand()
	}()

	go func() {
		defer wg.Done()
		data.Office, data.OfficeDate, data.Antivirus, data.AvDate = Software()
	}()

	go func() {
		defer wg.Done()
		data.Memories = Memory()
	}()

	// Disk 单独采集
	disksDone := make(chan struct{})
	go func() {
		data.Disks = Disk()
		close(disksDone)
	}()

	wg.Wait()
	<-disksDone

	return data
}
