package siwx

import (
	"fmt"
	"strings"
	"unicode"
)

type overviewResponse struct {
	OutputRoot string `json:"output_root"`
}

// meaningfulName 判断昵称里是否至少有一个可见字符。
// 全空白、控制符、零宽/格式字符、组合标记和常见「空白填充字符」都视为不可见。
func meaningfulName(name string) bool {
	for _, r := range name {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			continue
		}
		if unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Mn, r) {
			continue
		}
		// U+115F/U+1160/U+3164/U+FFA0 韩文填充符、U+2800 盲文空白，常被用作「空昵称」。
		switch r {
		case '\u115F', '\u1160', '\u2800', '\u3164', '\uFFA0':
			continue
		}
		return true
	}
	return false
}

func loadNickNames(dbPath string) (map[string]string, error) {
	output, err := queryContacts(dbPath)
	if err != nil {
		return nil, fmt.Errorf("读取联系人库 %s: %w", dbPath, err)
	}

	nickNames := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x1f", 2)
		if len(parts) != 2 {
			continue
		}
		username := strings.TrimSpace(parts[0])
		nickname := strings.TrimSpace(parts[1])
		if username != "" && nickname != "" {
			nickNames[username] = nickname
		}
	}
	return nickNames, nil
}

func queryContacts(dbPath string) (string, error) {
	return querySQLite(dbPath, "\x1f", "SELECT username, nick_name FROM contact;")
}
