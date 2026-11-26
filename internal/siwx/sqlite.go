package siwx

import (
	"os"
	"os/exec"
)

// querySQLite 执行只读查询；数据库被占用时复制一份再查。
func querySQLite(dbPath, separator, query string) (string, error) {
	args := []string{"-readonly"}
	if separator != "" {
		args = append(args, "-separator", separator)
	}
	args = append(args, dbPath, query)
	output, err := exec.Command("/usr/bin/sqlite3", args...).Output()
	if err == nil {
		return string(output), nil
	}

	data, readErr := os.ReadFile(dbPath)
	if readErr != nil {
		return "", err
	}
	temp, tempErr := os.CreateTemp("", "siwx-sqlite-*.db")
	if tempErr != nil {
		return "", err
	}
	defer func() { _ = os.Remove(temp.Name()) }()
	if _, writeErr := temp.Write(data); writeErr != nil {
		_ = temp.Close()
		return "", err
	}
	if closeErr := temp.Close(); closeErr != nil {
		return "", err
	}

	args = nil
	if separator != "" {
		args = append(args, "-separator", separator)
	}
	args = append(args, temp.Name(), query)
	output, err = exec.Command("/usr/bin/sqlite3", args...).Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}
