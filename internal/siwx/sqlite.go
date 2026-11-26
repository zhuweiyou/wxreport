package siwx

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

// querySQLite 执行只读查询；数据库被占用时复制一份再查。
func querySQLite(dbPath, separator, query string) (string, error) {
	output, err := querySQLiteFile(dbPath, separator, query)
	if err == nil {
		return output, nil
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
	return querySQLiteFile(temp.Name(), separator, query)
}

func querySQLiteFile(dbPath, separator, query string) (string, error) {
	path := filepath.ToSlash(dbPath)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	dsn := (&url.URL{
		Scheme:   "file",
		Path:     path,
		RawQuery: "mode=ro",
	}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return "", fmt.Errorf("打开 SQLite %s: %w", filepath.Base(dbPath), err)
	}
	defer func() { _ = db.Close() }()

	rows, err := db.Query(query)
	if err != nil {
		return "", fmt.Errorf("查询 SQLite %s: %w", filepath.Base(dbPath), err)
	}
	defer func() { _ = rows.Close() }()

	columns, err := rows.Columns()
	if err != nil {
		return "", fmt.Errorf("读取 SQLite 列: %w", err)
	}
	values := make([]any, len(columns))
	pointers := make([]any, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}

	var builder strings.Builder
	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			return "", fmt.Errorf("读取 SQLite 行: %w", err)
		}
		for i, value := range values {
			if i > 0 {
				builder.WriteString(separator)
			}
			builder.WriteString(formatSQLiteValue(value))
		}
		builder.WriteString("\n")
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("遍历 SQLite 结果: %w", err)
	}
	return builder.String(), nil
}

func formatSQLiteValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case []byte:
		return string(v)
	case string:
		return v
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}
