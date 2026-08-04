GO ?= go
GOFLAGS ?= -ldflags "-s -w"
TRIMFLAGS ?= -trimpath

BIN_DIR ?= bin
CLI_BIN ?= $(BIN_DIR)/hwinfo-cli
GUI_SCRIPT ?= $(BIN_DIR)/hwinfo-gui.sh

.PHONY: all cli gui clean dir upx help

all: cli gui

dir:
	@mkdir -p $(BIN_DIR)

cli: dir
	@echo "==> 编译 CLI 版本..."
	$(GO) build $(GOFLAGS) $(TRIMFLAGS) -o $(CLI_BIN) ./cli
	@echo "==> CLI 版本编译完成: $(CLI_BIN)"
	@echo "    体积: $$(du -h $(CLI_BIN) | cut -f1)"

gui: dir cli
	@echo "==> 复制 GUI 启动脚本..."
	@cp hwinfo-gui.sh $(GUI_SCRIPT)
	@chmod +x $(GUI_SCRIPT)
	@echo "==> GUI 脚本已准备: $(GUI_SCRIPT)"

upx: cli
	@if command -v upx &> /dev/null; then \
		echo "==> UPX 压缩中..."; \
		upx --best --lzma $(CLI_BIN); \
		echo "==> UPX 压缩完成"; \
		echo "    体积: $$(du -h $(CLI_BIN) | cut -f1)"; \
	else \
		echo "==> 未安装 upx，跳过压缩"; \
		echo "    安装: sudo apt install upx-ucl"; \
	fi

linux-amd64: dir
	@echo "==> 交叉编译 Linux amd64..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) $(TRIMFLAGS) -o $(BIN_DIR)/hwinfo-cli-linux-amd64 ./cli
	@echo "==> 编译完成: $(BIN_DIR)/hwinfo-cli-linux-amd64"
	@echo "    体积: $$(du -h $(BIN_DIR)/hwinfo-cli-linux-amd64 | cut -f1)"

linux-arm64: dir
	@echo "==> 交叉编译 Linux arm64..."
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) $(TRIMFLAGS) -o $(BIN_DIR)/hwinfo-cli-linux-arm64 ./cli
	@echo "==> 编译完成: $(BIN_DIR)/hwinfo-cli-linux-arm64"
	@echo "    体积: $$(du -h $(BIN_DIR)/hwinfo-cli-linux-arm64 | cut -f1)"

windows-amd64: dir
	@echo "==> 交叉编译 Windows amd64..."
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) $(TRIMFLAGS) -o $(BIN_DIR)/hwinfo-cli-windows-amd64.exe ./cli
	@echo "==> 编译完成: $(BIN_DIR)/hwinfo-cli-windows-amd64.exe"

clean:
	@echo "==> 清理编译产物..."
	@rm -rf $(BIN_DIR)
	@echo "==> 清理完成"

help:
	@echo "资产信息采集客户端 (Linux版) - 编译脚本"
	@echo ""
	@echo "用法:"
	@echo "  make               编译 CLI + GUI"
	@echo "  make cli           仅编译 CLI 版本"
	@echo "  make gui           准备 GUI 启动脚本"
	@echo "  make upx           用 UPX 压缩 (需先安装 upx-ucl)"
	@echo "  make linux-amd64   交叉编译 Linux amd64 (在 Windows 上也可运行)"
	@echo "  make linux-arm64   交叉编译 Linux arm64"
	@echo "  make clean         清理编译产物"
	@echo ""
	@echo "体积优化说明:"
	@echo "  默认编译: ~5MB"
	@echo "  + UPX 压缩: ~2MB"
	@echo ""
	@echo "使用方法:"
	@echo "  CLI: ./bin/hwinfo-cli auto"
	@echo "  GUI: ./bin/hwinfo-gui.sh"
