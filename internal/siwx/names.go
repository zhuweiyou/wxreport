package siwx

import (
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

type overviewResponse struct {
	OutputRoot string `json:"output_root"`
}

type nameRow struct {
	Preferred string
	Wxid      string
}

// Names 输出 sender -> 群昵称/微信昵称 的映射。
// 优先群昵称，其次 contact.nick_name，绝不使用 remark。
func Names(account, chat string) error {
	baseURL := strings.TrimRight(envOr("SIWX_BASE_URL", defaultBaseURL), "/")
	client := &http.Client{Timeout: 30 * time.Second}

	messages, err := fetchChatSince(client, baseURL, account, chat, time.Now().Add(-24*time.Hour).Unix())
	if err != nil {
		return err
	}

	var overview overviewResponse
	if err := getJSON(client, baseURL+"/api/settings/overview", &overview); err != nil {
		return fmt.Errorf("读取解密输出目录失败: %w", err)
	}
	outputRoot, err := resolveOutputRoot(overview.OutputRoot)
	if err != nil {
		return err
	}

	contactDB := filepath.Join(outputRoot, account, "contact", "contact.db")
	nickNames, err := loadNickNames(contactDB)
	if err != nil {
		return err
	}
	groupNames, err := loadGroupNickNames(contactDB, chat)
	if err != nil {
		return err
	}

	seen := make(map[string]bool)
	rows := make([]nameRow, 0, len(messages))
	for _, message := range messages {
		if message.SenderName == "" && message.SenderWxid == "" {
			continue
		}
		preferred := preferredName(message, groupNames, nickNames)
		key := preferred + "\x00" + message.SenderWxid
		if seen[key] {
			continue
		}
		seen[key] = true
		rows = append(rows, nameRow{
			Preferred: preferred,
			Wxid:      message.SenderWxid,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Preferred == rows[j].Preferred {
			return rows[i].Wxid < rows[j].Wxid
		}
		return rows[i].Preferred < rows[j].Preferred
	})

	fmt.Println("preferred\twxid")
	for _, row := range rows {
		fmt.Printf("%s\t%s\n", row.Preferred, row.Wxid)
	}
	return nil
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
