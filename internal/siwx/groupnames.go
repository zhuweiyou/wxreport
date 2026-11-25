package siwx

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// loadGroupNickNames 读取群成员在群里的昵称（群昵称）。
//
// 微信把群成员列表序列化在 contact.db 的 chat_room.ext_buffer 里，
// 结构是 protobuf：顶层重复 field 1，每个成员 submessage 的
// field 1 是成员 wxid，field 2 是群昵称（没设置时省略）。
func loadGroupNickNames(dbPath, chat string) (map[string]string, error) {
	escaped := strings.ReplaceAll(chat, "'", "''")
	query := fmt.Sprintf("SELECT hex(ext_buffer) FROM chat_room WHERE username='%s' LIMIT 1;", escaped)
	output, err := querySQLite(dbPath, "", query)
	if err != nil {
		return nil, fmt.Errorf("读取群成员昵称: %w", err)
	}
	text := strings.TrimSpace(output)
	if text == "" {
		return map[string]string{}, nil
	}
	blob, err := hex.DecodeString(text)
	if err != nil {
		return nil, fmt.Errorf("解析群成员昵称 hex: %w", err)
	}
	return parseGroupMembers(blob), nil
}

func parseGroupMembers(blob []byte) map[string]string {
	members := make(map[string]string)
	for len(blob) > 0 {
		key, n := readVarint(blob)
		if n <= 0 {
			break
		}
		blob = blob[n:]
		field, wire := key>>3, key&7

		switch wire {
		case 0: // varint
			_, n = readVarint(blob)
			if n <= 0 {
				return members
			}
			blob = blob[n:]
		case 1: // 64-bit
			if len(blob) < 8 {
				return members
			}
			blob = blob[8:]
		case 2: // length-delimited
			length, n := readVarint(blob)
			if n <= 0 || n > len(blob) || length > uint64(len(blob)-n) {
				return members
			}
			value := blob[n : n+int(length)]
			blob = blob[n+int(length):]
			if field == 1 {
				if id, nickname := parseGroupMember(value); id != "" && nickname != "" {
					members[id] = nickname
				}
			}
		case 5: // 32-bit
			if len(blob) < 4 {
				return members
			}
			blob = blob[4:]
		default:
			return members
		}
	}
	return members
}

func parseGroupMember(data []byte) (string, string) {
	var id, nickname string
	for len(data) > 0 {
		key, n := readVarint(data)
		if n <= 0 {
			return id, nickname
		}
		data = data[n:]
		field, wire := key>>3, key&7

		switch wire {
		case 0:
			_, n = readVarint(data)
			if n <= 0 {
				return id, nickname
			}
			data = data[n:]
		case 1:
			if len(data) < 8 {
				return id, nickname
			}
			data = data[8:]
		case 2:
			length, n := readVarint(data)
			if n <= 0 || n > len(data) || length > uint64(len(data)-n) {
				return id, nickname
			}
			value := string(data[n : n+int(length)])
			data = data[n+int(length):]
			switch field {
			case 1:
				id = value
			case 2:
				nickname = value
			}
		case 5:
			if len(data) < 4 {
				return id, nickname
			}
			data = data[4:]
		default:
			return id, nickname
		}
	}
	return id, nickname
}

func readVarint(data []byte) (uint64, int) {
	var value uint64
	for i := 0; i < len(data) && i < 10; i++ {
		b := data[i]
		value |= uint64(b&0x7f) << (7 * i)
		if b < 0x80 {
			return value, i + 1
		}
	}
	return 0, 0
}
