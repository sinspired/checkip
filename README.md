# CheckIP

一个用于检查 IP 地址信息的 Go 项目，支持地理位置查询、CDN 检测和代理信息获取。

## 功能特性

- 🌍 **地理位置查询**: 使用 MaxMind GeoLite2 数据库查询 IP 地理位置
- 🚀 **CDN 检测**: 检测 IP 是否属于 Cloudflare CDN
- 🔍 **代理检测**: 检测代理服务器和出口 IP
- 📊 **多 API 支持**: 支持多个 IP 查询 API
- 🛡️ **错误处理**: 完善的错误处理和重试机制

## 项目结构

```
checkip/
├── api/                    # API 层
│   └── checkip.go         # HTTP 处理器
├── cmd/                   # 命令行入口
│   └── api/
│       └── main.go        # API 服务器主程序
├── internal/              # 内部包
│   ├── assets/           # 资源文件
│   │   ├── cloudflare_cdn_ip.go
│   │   ├── cloudflare_cdn_ip_process.go
│   │   ├── maxmind_db.go
│   │   └── maxmind_db_process.go
│   └── checkip/          # 核心逻辑
│       ├── checkip.go    # 主要功能
│       ├── cloudflare.go # Cloudflare 相关功能
│       └── checkip_test.go
├── go.mod
├── go.sum
└── README.md
```

## 安装和运行

### 编译

```bash
go build ./cmd/api
```

### 运行

```bash
# 使用默认配置
./api

# 指定端口
ADDR=:8080 ./api

# 指定 MaxMind 数据库路径
MAXMIND_DB_PATH=/path/to/GeoLite2-Country.mmdb ./api
```

### API 使用

```bash
# 获取当前 IP 的完整信息
curl "http://localhost:8099/api"

# 仅获取当前 IP 地址
curl "http://localhost:8099/api/ip"

# 检查指定 IP 地址（查询参数方式）
curl "http://localhost:8099/api?ip=8.8.8.8"

# 检查指定 IP 地址（路径参数方式）
curl "http://localhost:8099/api/8.8.8.8"
```

响应示例：

**获取当前 IP 信息** (`/api`):
```json
{
  "ip": "34.201.45.67",
  "country_code": "US",
  "is_cdn": false,
  "location": "US",
  "tag": "US²"
}
```

**仅获取 IP 地址** (`/api/ip`):
```json
{
  "ip": "34.201.45.67"
}
```

**检查指定 IP** (`/api?ip=8.8.8.8` 或 `/api/8.8.8.8`):
```json
{
  "ip": "8.8.8.8",
  "country_code": "US",
  "is_cdn": false,
  "location": "US",
  "tag": "US²"
}
```

## 环境变量

### 运行测试

```bash
### 代码格式化

```bash
go fmt ./...
```

## 许可证

MIT License
