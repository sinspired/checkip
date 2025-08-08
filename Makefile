# Makefile for CheckIP project

.PHONY: build test clean run help

# 默认目标
.DEFAULT_GOAL := help

# 构建变量
BINARY_NAME := checkip-api
BUILD_DIR := build
MAIN_PATH := ./cmd/api

# 构建应用
build:
	@echo "Building CheckIP API..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Build completed: $(BUILD_DIR)/$(BINARY_NAME)"

# 运行测试
test:
	@echo "Running tests..."
	go test -v ./internal/checkip
	go test -v ./internal/assets

# 运行测试并生成覆盖率报告
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./internal/checkip
	go test -v -coverprofile=coverage.out ./internal/assets
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# 清理构建文件
clean:
	@echo "Cleaning build files..."
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html
	@echo "Clean completed"

# 运行应用
run: build
	@echo "Running CheckIP API..."
	./$(BUILD_DIR)/$(BINARY_NAME)

# 开发模式运行（直接运行，不构建）
dev:
	@echo "Running in development mode..."
	go run $(MAIN_PATH)

# 格式化代码
fmt:
	@echo "Formatting code..."
	go fmt ./...

# 代码检查
lint:
	@echo "Running linter..."
	golangci-lint run

# 安装依赖
deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# 显示帮助信息
help:
	@echo "CheckIP Makefile Commands:"
	@echo "  build        - Build the application"
	@echo "  test         - Run tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  clean        - Clean build files"
	@echo "  run          - Build and run the application"
	@echo "  dev          - Run in development mode"
	@echo "  fmt          - Format code"
	@echo "  lint         - Run linter"
	@echo "  deps         - Install dependencies"
	@echo "  help         - Show this help message" 