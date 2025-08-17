package data

import (
    "bytes"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "runtime"

    "github.com/klauspost/compress/zstd"
    "github.com/oschwald/maxminddb-golang/v2"
)

// OpenMaxMindDB 打开 MaxMind 数据库（自动处理不存在时的解压）
func OpenMaxMindDB() (*maxminddb.Reader, error) {
    outputPath := resolveAssetPath("maxmindDB")
    mmdbPath := filepath.Join(outputPath, "GeoLite2-Country.mmdb")

    // 如果数据库文件不存在，则解压生成
    if _, err := os.Stat(mmdbPath); os.IsNotExist(err) {
        if err := ensureMMDBFile(outputPath, mmdbPath); err != nil {
            return nil, err
        }
    }

    return openDBWithArch(mmdbPath)
}

// OpenGeoDB 打开指定路径的地理数据库（空路径则使用默认）
func OpenGeoDB(path string) (*maxminddb.Reader, error) {
    if path == "" {
        return OpenMaxMindDB()
    }
    return openDBWithArch(path)
}

// 根据架构选择合适的打开方式
func openDBWithArch(path string) (*maxminddb.Reader, error) {
    if runtime.GOARCH == "386" {
        return openFromBytes(path)
    }
    db, err := maxminddb.Open(path)
    if err != nil {
        return nil, fmt.Errorf("maxmind数据库打开失败: %w", err)
    }
    return db, nil
}

// 确保数据库文件存在，不存在则从嵌入数据解压生成
func ensureMMDBFile(outputPath, mmdbPath string) error {
    if err := os.MkdirAll(outputPath, 0755); err != nil {
        return fmt.Errorf("创建数据库目录失败: %w", err)
    }

    zstdDecoder, err := zstd.NewReader(nil)
    if err != nil {
        return fmt.Errorf("zstd解码器创建失败: %w", err)
    }
    defer zstdDecoder.Close()

    file, err := os.OpenFile(mmdbPath, os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        return fmt.Errorf("maxmind数据库文件创建失败: %w", err)
    }
    defer file.Close()

    zstdDecoder.Reset(bytes.NewReader(EmbeddedMaxMindDB))
    if _, err := io.Copy(file, zstdDecoder); err != nil {
        return fmt.Errorf("maxmind数据库文件解压失败: %w", err)
    }
    return nil
}

// 解析 assets 路径
func resolveAssetPath(subDir string) string {
    if os.Getenv("TESTING") == "1" {
        return os.TempDir()
    }

    if exePath, err := os.Executable(); err == nil {
        exeDir := filepath.Dir(exePath)
        if _, err := os.Stat(exeDir); err == nil {
            path := filepath.Join(exeDir, "assets", subDir)
            if os.MkdirAll(path, 0755) == nil {
                return path
            }
        }
    }

    if cwd, err := os.Getwd(); err == nil {
        path := filepath.Join(cwd, "assets", subDir)
        if os.MkdirAll(path, 0755) == nil {
            return path
        }
    }

    path := filepath.Join(os.TempDir(), "checkip", "assets", subDir)
    if os.MkdirAll(path, 0755) == nil {
        return path
    }

    return filepath.Join("assets", subDir)
}

// 32位系统从内存读取数据库
func openFromBytes(path string) (*maxminddb.Reader, error) {
    runtime.GC() // 主动GC，释放内存

    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("读取文件到内存失败: %w", err)
    }
    reader, err := maxminddb.FromBytes(data)
    if err != nil {
        return nil, fmt.Errorf("从字节数组创建reader失败: %w", err)
    }
    return reader, nil
}
