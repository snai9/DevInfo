# 国产系统（统信UOS）客户端开发计划

## 一、项目现状分析

### 1.1 当前架构
```
┌─────────────────┐         HTTP          ┌─────────────────┐
│  aardio 客户端   │ ─────────────────────▶ │  aardio 服务端   │
│  (Windows only)  │  POST /api/upload     │  (保持不变)       │
└─────────────────┘                        └─────────────────┘
```

### 1.2 服务端 API（保持不变）
| 接口 | 方法 | 说明 |
|------|------|------|
| `/api/ping` | POST | 探测服务器是否可达 |
| `/api/upload` | POST | 上传硬件/软件信息数据 |
| `/api/records` | GET | 查询所有记录（管理用） |

### 1.3 客户端核心功能
1. **硬件信息采集**：主机名、IP/MAC、操作系统、计算机编号(BIOS/主板序列号)、品牌、内存详情、硬盘详情
2. **软件信息采集**：办公软件（WPS/Office/LibreOffice）、杀毒软件
3. **使用人信息**：姓名、单位、部门（可配置保存）
4. **服务器连接**：自动探测可用服务器、手动配置服务器地址
5. **数据上传**：将采集到的信息上传到服务端

---

## 二、技术方案建议

### 方案对比

| 方案 | 语言 | GUI | 单文件部署 | 开发难度 | 统信UOS兼容性 | 推荐指数 |
|------|------|-----|-----------|----------|---------------|---------|
| **方案A：Python + Tkinter** | Python | ✅ Tkinter(内置) | ⚠️ PyInstaller打包 | ⭐ 简单 | ✅ 优秀 | ⭐⭐⭐⭐⭐ |
| 方案B：Go + Fyne | Go | ✅ Fyne | ✅ 原生编译 | ⭐⭐ 中等 | ✅ 良好 | ⭐⭐⭐⭐ |
| 方案C：Node.js + Electron | JS | ✅ Electron | ⚠️ 打包体积大 | ⭐⭐ 中等 | ✅ 良好 | ⭐⭐⭐ |
| 方案D：Shell脚本(无GUI) | Bash | ❌ | ✅ | ⭐ 极简 | ✅ 优秀 | ⭐⭐⭐ |

### 推荐方案：Python + Tkinter

**理由：**
1. **统信UOS默认自带Python**，无需额外安装运行时
2. **Tkinter是Python内置GUI库**，零依赖，开发快
3. Linux下获取硬件信息Python非常方便（读`/sys`、`/proc`、调用`dmidecode`等命令）
4. 团队已有 aardio 动态语言经验，Python语法学习成本低
5. PyInstaller可打包成单文件分发，体积约15-20MB
6. 代码量可控，预估500-800行

---

## 三、详细实现方案

### 3.1 目录结构
```
DevInfo-master/
├── linux_client/              # 新增：Linux客户端目录
│   ├── main.py                # 主入口（GUI版本）
│   ├── cli.py                 # 命令行版本（无GUI）
│   ├── collector.py           # 硬件/软件信息采集模块
│   ├── uploader.py            # HTTP上传模块
│   ├── config.py              # 配置管理模块
│   ├── requirements.txt       # Python依赖（极少）
│   ├── build.sh               # 打包脚本
│   └── README.md              # 使用说明
├── ...                        # 原有 aardio 项目保持不变
```

### 3.2 核心模块设计

#### 3.2.1 信息采集模块 (collector.py)
Linux下硬件信息采集方式：

| 信息项 | Windows(aardio) | Linux(Python) |
|--------|-----------------|---------------|
| 主机名 | WMI Win32_ComputerSystem | `socket.gethostname()` 或 `/proc/sys/kernel/hostname` |
| IP地址 | WMI Win32_NetworkAdapterConfiguration | `socket.gethostbyname()` 或 `/sys/class/net/*/address` |
| MAC地址 | 同上 | `/sys/class/net/*/address` |
| 操作系统 | WMI Win32_OperatingSystem | `/etc/os-release` 或 `lsb_release -a` |
| 计算机编号 | WMI Win32_BIOS/BaseBoard SerialNumber | `dmidecode -s system-serial-number` 或 `/sys/class/dmi/id/product_serial` |
| 计算机品牌 | WMI Win32_ComputerSystem Manufacturer+Model | `dmidecode -s system-manufacturer` + `system-product-name` |
| 内存信息 | WMI Win32_PhysicalMemory | `dmidecode --type 17` 或 `/proc/meminfo` |
| 硬盘信息 | WMI Win32_DiskDrive | `lsblk -J` 或 `hdparm -i` 或 `/sys/block/*/` |
| 办公软件 | 注册表 Uninstall | dpkg/apt/rpm 包管理查询 |
| 杀毒软件 | 注册表 + WMI SecurityCenter2 | 包管理查询（奇安信/深信服等Linux版杀软） |

#### 3.2.2 上传模块 (uploader.py)
```python
# 与 aardio 客户端保持完全一致的协议
POST http://server:8080/api/upload
Content-Type: application/json

{
    "hostname": "PC-001",
    "osver": "uos 20 1050d",
    "pcId": "ABC123",
    "pcBrand": "Lenovo 启天M435",
    "ips": ["192.168.1.100"],
    "macs": ["AA:BB:CC:DD:EE:FF"],
    "office": "WPS Office 11.1",
    "officeDate": "2024-01-15",
    "antivirus": "奇安信天擎",
    "avDate": "2024-02-20",
    "memories": [{"model": "Samsung DDR4 3200", "size": "8 GB"}],
    "disks": [{"model": "WD PC SN530", "size": "512 GB", "serial": "ABC123"}],
    "used": "张三",
    "unit": "XX单位",
    "dept": "XX部门"
}
```

#### 3.2.3 配置管理 (config.py)
```
配置文件路径：~/.config/hwinfo/config.json
内容：
{
    "used": "张三",
    "unit": "XX单位",
    "dept": "XX部门",
    "serverUrls": ["http://10.44.179.88:8080", "http://10.130.175.88:8080"],
    "activeUrl": ""
}
```

### 3.3 GUI 设计（Tkinter）

与 aardio 客户端界面保持一致：
```
┌─────────────────────────────────────────────┐
│ 使用人: 张三 | XX单位 | XX部门    [修改信息]│
│                                         [服务器设置]
├─────────────────────────────────────────────┤
│ 基本信息                                      │
│   计算机名：PC-001                            │
│   操作系统：uos 20 1050d                      │
│   计算机编号：ABC123                           │
│   计算机品牌：Lenovo 启天M435                  │
├─────────────────────────────────────────────┤
│ 网络信息                                      │
│   IP地址：192.168.1.100                       │
│   MAC地址：AA:BB:CC:DD:EE:FF                  │
├─────────────────────────────────────────────┤
│ 软件信息                                      │
│   办公软件：WPS Office 11.1                    │
│   安装日期：2024-01-15                         │
│   杀毒软件：奇安信天擎                          │
│   安装日期：2024-02-20                         │
├─────────────────────────────────────────────┤
│ 内存信息                                      │
│   Samsung DDR4 3200 | 8 GB                    │
├─────────────────────────────────────────────┤
│ 硬盘信息                                      │
│   WD PC SN530 | 512 GB | ABC123              │
├─────────────────────────────────────────────┤
│ [刷新信息]  [上传信息]  [退出]                 │
│ ✓就绪 | ✓已连接 10.44.179.88 | 14:30:25    │
└─────────────────────────────────────────────┘
```

### 3.4 命令行版本（无GUI，备选方案）

如果GUI开发复杂，可先提供命令行版本：

```bash
# 采集并显示信息
./hwinfo-cli --collect

# 配置使用人信息
./hwinfo-cli --config --used 张三 --unit XX单位 --dept XX部门

# 指定服务器地址
./hwinfo-cli --server http://10.44.179.88:8080 --upload

# 一键采集+上传
./hwinfo-cli --auto
```

---

## 四、实施步骤

### 阶段一：基础框架（信息采集 + 上传）
1. 创建 `linux_client/` 目录结构
2. 实现 `collector.py` - 硬件信息采集（Linux版）
3. 实现 `uploader.py` - HTTP上传（与服务端API对接）
4. 实现 `config.py` - 配置管理
5. 实现 `cli.py` - 命令行版本（可先验证功能）

### 阶段二：GUI 版本
6. 实现 `main.py` - Tkinter GUI界面
7. 界面样式美化（配色与aardio版保持一致）
8. 集成测试

### 阶段三：打包部署
9. 编写 `build.sh` - PyInstaller打包脚本
10. 统信UOS真机测试
11. 编写使用说明

---

## 五、风险与注意事项

### 5.1 硬件信息采集风险
- **dmidecode需要root权限**：普通用户可能读不到完整的BIOS/主板序列号 → 降级方案：读 `/sys/class/dmi/id/*`（无需root）
- **不同Linux发行版差异**：统信UOS基于Debian，但个别路径可能有差异 → 多路径探测，取首个有效结果
- **虚拟机环境**：虚拟机很多硬件信息为空 → 返回"未获取"即可

### 5.2 软件信息采集
- Linux下杀软不如Windows普及 → 重点检测奇安信、深信服、火绒等国内厂商的Linux版本
- 包管理查询：dpkg(Debian/UOS)、rpm(CentOS) → 同时支持两种

### 5.3 打包部署
- PyInstaller在统信UOS上打包需在UOS系统上进行（不能在Windows上交叉编译）
- 目标系统需有对应架构的Python环境

---

## 六、可选优化方向

1. **Qt界面版本**：如对Tkinter外观不满意，可改用PyQt（依赖较重）
2. **Go语言版本**：如追求单文件+极小体积，可用Go重写（约10MB单文件）
3. **系统托盘**：开机自启、后台静默采集
4. **自动升级**：检测新版本自动更新

---

## 七、开发资源需求

- **开发周期**：3-5个工作日
- **测试环境**：一台统信UOS物理机或虚拟机
- **依赖**：Python 3.7+（统信UOS默认自带），requests库（可选，可用内置urllib）
