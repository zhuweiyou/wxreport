package shot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOSSConfigFromLocalEnv(t *testing.T) {
	for _, key := range []string{
		"OSS_BUCKET", "OSS_ACCESS_KEY_ID", "OSS_ACCESS_KEY_SECRET", "OSS_PATH",
		"OSS_REGION", "OSS_ENDPOINT", "OSS_DOMAIN", "OSS_PUBLIC_BASE",
		"ALIBABA_CLOUD_ACCESS_KEY_ID", "ALIBABA_CLOUD_ACCESS_KEY_SECRET",
	} {
		t.Setenv(key, "")
	}

	root := t.TempDir()
	content := "OSS_BUCKET=bucket\nOSS_ACCESS_KEY_ID=key\nOSS_ACCESS_KEY_SECRET=secret\nOSS_PATH=latest-report.png\n"
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadOSSConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bucket != "bucket" || cfg.Path != "latest-report.png" {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestPublicURL(t *testing.T) {
	cfg := OSSConfig{Bucket: "bucket", Endpoint: "https://oss-cn-shanghai.aliyuncs.com"}
	if got := cfg.PublicURL("latest-report.png"); got != "https://bucket.oss-cn-shanghai.aliyuncs.com/latest-report.png" {
		t.Fatalf("url = %s", got)
	}

	cfg.PublicBase = "https://cdn.example.com/"
	if got := cfg.PublicURL("/latest-report.png"); got != "https://cdn.example.com/latest-report.png" {
		t.Fatalf("custom domain url = %s", got)
	}
}
