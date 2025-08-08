// internal/assets/maxmind_db_process.go
package assets

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/klauspost/compress/zstd"
	"github.com/oschwald/maxminddb-golang/v2"
)

// OpenMaxMindDB 打开 MaxMind 数据库。
func OpenMaxMindDB() (*maxminddb.Reader, error) {
	OutputPath := resolveAssetPath("maxmindDB")
	mmdbPath := filepath.Join(OutputPath, "GeoLite2-Country.mmdb")

	// TODO: 应定期更新数据库文件
	if _, err := os.Stat(mmdbPath); err == nil {
		db, err := maxminddb.Open(mmdbPath)
		if err != nil {
			return nil, fmt.Errorf("maxmind数据库打开失败: %w", err)
		}
		return db, nil
	}

	zstdDecoder, err := zstd.NewReader(nil)
	if err != nil {
		return nil, fmt.Errorf("zstd解码器创建失败: %w", err)
	}
	defer zstdDecoder.Close()

	mmdbFile, err := os.OpenFile(mmdbPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("maxmind数据库文件创建失败: %w", err)
	}
	defer mmdbFile.Close()

	zstdDecoder.Reset(bytes.NewReader(EmbeddedMaxMindDB))
	if _, err := io.Copy(mmdbFile, zstdDecoder); err != nil {
		return nil, fmt.Errorf("maxmind数据库文件解压失败: %w", err)
	}

	db, err := maxminddb.Open(mmdbPath)
	if err != nil {
		return nil, fmt.Errorf("maxmind数据库打开失败: %w", err)
	}
	return db, nil
}

func resolveAssetPath(subDir string) string {
	// 在测试环境中，使用临时目录
	if os.Getenv("TESTING") == "1" {
		return os.TempDir()
	}
	
	exePath, err := os.Executable()
	if err != nil {
		// fallback to current working directory
		return filepath.Join("assets", subDir)
	}
	exeDir := filepath.Dir(exePath)
	return filepath.Join(exeDir, "assets", subDir)
}

// OpenGeoDB 打开地理数据库
func OpenGeoDB(path string) (*maxminddb.Reader, error) {
	if path == "" {
		// 使用嵌入的数据库
		return OpenMaxMindDB()
	}
	
	// 使用指定的路径
	db, err := maxminddb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open geo database: %w", err)
	}
	return db, nil
}
