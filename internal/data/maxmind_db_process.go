package data

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/oschwald/maxminddb-golang/v2"
)

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// OpenMaxMindDB 打开 MaxMind 数据库（自动处理不存在时的解压）
func OpenMaxMindDB(dbPath string) (*maxminddb.Reader, error) {
	if dbPath != "" {
		return openDBWithArch(dbPath)
	}
	outputPath := ResolveDataPath()
	mmdbPath := filepath.Join(outputPath, "GeoLite2-City.mmdb")

	// 如果数据库文件不存在，则解压生成
	if _, err := os.Stat(mmdbPath); os.IsNotExist(err) {
		if err := ensureMMDBFile(outputPath, mmdbPath); err != nil {
			return nil, err
		}
	}

	return openDBWithArch(mmdbPath)
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

	zstdDecoder.Reset(bytes.NewReader(EmbeddedMaxMindDBCity))
	if _, err := io.Copy(file, zstdDecoder); err != nil {
		return fmt.Errorf("maxmind数据库文件解压失败: %w", err)
	}
	return nil
}

// 解析 assets 路径
func ResolveDataPath() string {
	if os.Getenv("TESTING") == "true" {
		return os.TempDir()
	}

	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		if _, err := os.Stat(exeDir); err == nil {
			path := filepath.Join(exeDir, "data")
			if os.MkdirAll(path, 0755) == nil {
				return path
			}
		}
	}

	if cwd, err := os.Getwd(); err == nil {
		path := filepath.Join(cwd, "data")
		if os.MkdirAll(path, 0755) == nil {
			return path
		}
	}

	path := filepath.Join(os.TempDir(), "checkip", "data")
	if os.MkdirAll(path, 0755) == nil {
		return path
	}

	return filepath.Join("data")
}

// 32位系统从内存读取数据库
func openFromBytes(path string) (*maxminddb.Reader, error) {
	runtime.GC() // 主动GC，释放内存

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取文件到内存失败: %w", err)
	}
	reader, err := maxminddb.OpenBytes(data)
	if err != nil {
		return nil, fmt.Errorf("从字节数组创建reader失败: %w", err)
	}
	return reader, nil
}

// UpdateGeoLite2DB 检查并更新 GeoLite2 数据库
func UpdateGeoLite2DB(dbPath string) error {
	GithubProxy := os.Getenv("GITHUB_PROXY")
	if GithubProxy == "" {
		GithubProxy = "https://ghproxy.net/"
	}

	apiURL := "https://api.github.com/repos/mojolabs-id/GeoLite2-Database/releases/latest"

	resp, err := http.Get(apiURL)
	if err != nil {
		return fmt.Errorf("获取 release 信息失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API 状态码: %d", resp.StatusCode)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return fmt.Errorf("解析 release JSON 失败: %w", err)
	}

	var downloadURL string
	for _, asset := range rel.Assets {
		if asset.Name == "GeoLite2-City.mmdb" {
			downloadURL = GithubProxy + asset.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return errors.New("未找到 GeoLite2-Country.mmdb 下载地址")
	}

	// 备份原文件
	bakPath := dbPath + ".bak"
	if _, err := os.Stat(dbPath); err == nil {
		if err := os.Rename(dbPath, bakPath); err != nil {
			return fmt.Errorf("备份原文件失败: %w", err)
		}
	}

	// 下载（重试 3 次）
	success := false
	for i := range 3 {
		if err := downloadFile(downloadURL, dbPath); err != nil {
			fmt.Printf("下载失败 (%d/3): %v\n", i+1, err)
			time.Sleep(1 * time.Second)
			continue
		}
		success = true
		break
	}

	if !success {
		// 回退
		if _, err := os.Stat(bakPath); err == nil {
			_ = os.Rename(bakPath, dbPath)
		}
		return errors.New("下载失败，已回退原文件")
	}

	// 成功则删除备份
	_ = os.Remove(bakPath)
	slog.Info("GeoLite2-City.mmdb 更新完成")
	return nil
}

func downloadFile(url, path string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP 状态码 %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
