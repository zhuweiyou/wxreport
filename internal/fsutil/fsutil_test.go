package fsutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomicCreatesDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "out.txt")
	if err := WriteFileAtomic(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "hello" {
		t.Fatalf("内容 = %q，期望 hello", raw)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("临时文件应已重命名，残留状态: %v", err)
	}
}

func TestWriteJSONAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	value := map[string]any{"version": 1, "name": "测试"}
	if err := WriteJSONAtomic(path, value); err != nil {
		t.Fatalf("WriteJSONAtomic: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		t.Fatalf("JSON 文件应以换行结尾: %q", raw)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("解析写出内容: %v", err)
	}
	if got["name"] != "测试" {
		t.Fatalf("name = %v，期望 测试", got["name"])
	}
}
