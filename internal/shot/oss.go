// Package shot 的 OSS 上传部分。配置沿用项目旧约定：环境变量优先，
// 其次读取 config/oss.env（该文件已被 .gitignore 忽略）。
package shot

import (
	"bufio"
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// OSSConfig 是阿里云 OSS 的上传配置。
type OSSConfig struct {
	Bucket          string
	Region          string
	Endpoint        string
	AccessKeyID     string
	AccessKeySecret string
	// PublicBase 是可选的自定义域名/CDN 前缀，例如 https://cdn.example.com。
	PublicBase string
	// Path 是 config/oss.env 里配置的默认上传路径；为空时由调用方回退到 latest-report.png。
	Path string
}

// LoadOSSConfig 读取 OSS 配置：环境变量优先，其次 config/oss.env。
func LoadOSSConfig(root string) (OSSConfig, error) {
	values := readEnvFile(filepath.Join(root, "config", "oss.env"))

	get := func(keys ...string) string {
		for _, key := range keys {
			if value := strings.TrimSpace(os.Getenv(key)); value != "" {
				return value
			}
			if value := strings.TrimSpace(values[key]); value != "" {
				return value
			}
		}
		return ""
	}

	region := get("OSS_REGION")
	if region == "" {
		region = "oss-cn-shanghai"
	}
	if !strings.HasPrefix(region, "oss-") {
		region = "oss-" + region
	}

	endpoint := get("OSS_ENDPOINT")
	if endpoint == "" {
		endpoint = "https://" + region + ".aliyuncs.com"
	}
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}

	cfg := OSSConfig{
		Bucket:          get("OSS_BUCKET"),
		Region:          region,
		Endpoint:        endpoint,
		AccessKeyID:     get("OSS_ACCESS_KEY_ID", "ALIBABA_CLOUD_ACCESS_KEY_ID"),
		AccessKeySecret: get("OSS_ACCESS_KEY_SECRET", "ALIBABA_CLOUD_ACCESS_KEY_SECRET"),
		PublicBase:      get("OSS_PUBLIC_BASE", "OSS_DOMAIN"),
		Path:            get("OSS_PATH"),
	}

	var missing []string
	if cfg.Bucket == "" {
		missing = append(missing, "OSS_BUCKET")
	}
	if cfg.AccessKeyID == "" {
		missing = append(missing, "OSS_ACCESS_KEY_ID")
	}
	if cfg.AccessKeySecret == "" {
		missing = append(missing, "OSS_ACCESS_KEY_SECRET")
	}
	if len(missing) > 0 {
		return OSSConfig{}, fmt.Errorf(
			"缺少 OSS 配置 %s；可设置环境变量，或写入 %s",
			strings.Join(missing, " / "),
			filepath.Join(root, "config", "oss.env"),
		)
	}
	return cfg, nil
}

// PublicURL 返回对象的公网访问地址。
func (cfg OSSConfig) PublicURL(objectPath string) string {
	objectPath = strings.TrimLeft(objectPath, "/")
	if cfg.PublicBase != "" {
		base := strings.TrimRight(cfg.PublicBase, "/")
		if !strings.Contains(base, "://") {
			base = "https://" + base
		}
		return base + "/" + escapeKey(objectPath)
	}
	endpoint := cfg.Endpoint
	for _, prefix := range []string{"https://", "http://"} {
		endpoint = strings.TrimPrefix(endpoint, prefix)
	}
	return "https://" + cfg.Bucket + "." + strings.TrimRight(endpoint, "/") + "/" + escapeKey(objectPath)
}

// Upload 上传 PNG 到指定路径；同名对象直接覆盖。
func Upload(cfg OSSConfig, objectPath string, data []byte) (string, error) {
	objectPath = strings.TrimLeft(strings.TrimSpace(objectPath), "/")
	if objectPath == "" {
		return "", fmt.Errorf("OSS 上传路径不能为空")
	}

	client, err := oss.New(cfg.Endpoint, cfg.AccessKeyID, cfg.AccessKeySecret)
	if err != nil {
		return "", fmt.Errorf("创建 OSS 客户端: %w", err)
	}
	bucket, err := client.Bucket(cfg.Bucket)
	if err != nil {
		return "", fmt.Errorf("打开 OSS Bucket %s: %w", cfg.Bucket, err)
	}

	err = bucket.PutObject(objectPath, bytes.NewReader(data),
		oss.ContentType("image/png"),
		// 固定地址覆盖后必须让客户端重新拉取：no-store 连缓存副本都不留。
		oss.CacheControl("no-store, no-cache, must-revalidate, max-age=0"),
		oss.Expires(time.Unix(0, 0).UTC()),
		oss.ContentLength(int64(len(data))),
	)
	if err != nil {
		return "", fmt.Errorf("上传 OSS %s: %w", objectPath, err)
	}
	return cfg.PublicURL(objectPath), nil
}

func readEnvFile(path string) map[string]string {
	values := make(map[string]string)
	file, err := os.Open(path)
	if err != nil {
		return values
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		values[strings.TrimSpace(key)] = value
	}
	return values
}

func escapeKey(key string) string {
	parts := strings.Split(key, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}
