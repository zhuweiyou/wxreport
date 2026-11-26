package siwx

import (
	"fmt"
	"strings"
	"unicode"
)

type overviewResponse struct {
	OutputRoot string `json:"output_root"`
}

func meaningfulName(name string) bool {
	for _, r := range name {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			continue
		}
		if r == 'ㅤ' || unicode.Is(unicode.Cf, r) {
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
