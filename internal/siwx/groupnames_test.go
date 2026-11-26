package siwx

import (
	"encoding/hex"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"
)

func appendTestVarint(dst []byte, value uint64) []byte {
	for value >= 0x80 {
		dst = append(dst, byte(value)|0x80)
		value >>= 7
	}
	return append(dst, byte(value))
}

func appendTestTag(dst []byte, field, wire uint64) []byte {
	return appendTestVarint(dst, field<<3|wire)
}

func appendTestBytes(dst []byte, field uint64, value []byte) []byte {
	dst = appendTestTag(dst, field, 2)
	dst = appendTestVarint(dst, uint64(len(value)))
	return append(dst, value...)
}

func appendTestVarintField(dst []byte, field, value uint64) []byte {
	dst = appendTestTag(dst, field, 0)
	return appendTestVarint(dst, value)
}

func testMember(id, nickname string) []byte {
	var member []byte
	member = appendTestBytes(member, 1, []byte(id))
	if nickname != "" {
		member = appendTestBytes(member, 2, []byte(nickname))
	}
	member = appendTestVarintField(member, 3, 17)
	member = appendTestBytes(member, 4, []byte("wxid_owner"))
	return member
}

func testGroupBlob(members ...[2]string) []byte {
	var blob []byte
	for _, item := range members {
		blob = appendTestBytes(blob, 1, testMember(item[0], item[1]))
	}
	// 顶层未知字段应被跳过。
	blob = appendTestVarintField(blob, 99, 1)
	blob = appendTestBytes(blob, 100, []byte("ignored"))
	return blob
}

func TestParseGroupMembers(t *testing.T) {
	blob := testGroupBlob(
		[2]string{"wxid_test_member_a", "测试成员A"},
		[2]string{"wxid_test_member_b", "测试成员B"},
		[2]string{"wxid_without_nick", ""},
	)
	members := parseGroupMembers(blob)
	if members["wxid_test_member_a"] != "测试成员A" {
		t.Errorf("zhu = %q", members["wxid_test_member_a"])
	}
	if members["wxid_test_member_b"] != "测试成员B" {
		t.Errorf("yuan = %q", members["wxid_test_member_b"])
	}
	if _, ok := members["wxid_without_nick"]; ok {
		t.Error("member without group nickname should be omitted")
	}
}

func TestLoadGroupNickNamesFromSQLite(t *testing.T) {
	sqlite3, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("sqlite3 不可用")
	}
	dbPath := filepath.Join(t.TempDir(), "contact.db")
	blob := testGroupBlob(
		[2]string{"wxid_test_member_a", "测试成员A"},
		[2]string{"wxid_test_member_b", "测试成员B"},
	)
	sql := fmt.Sprintf(
		"CREATE TABLE chat_room(username TEXT, ext_buffer BLOB); "+
			"INSERT INTO chat_room VALUES('12345678901@chatroom', X'%s');",
		hex.EncodeToString(blob),
	)
	if output, err := exec.Command(sqlite3, dbPath, sql).CombinedOutput(); err != nil {
		t.Fatalf("创建测试库: %v\n%s", err, output)
	}

	members, err := loadGroupNickNames(dbPath, "12345678901@chatroom")
	if err != nil {
		t.Fatalf("loadGroupNickNames: %v", err)
	}
	if members["wxid_test_member_a"] != "测试成员A" || members["wxid_test_member_b"] != "测试成员B" {
		t.Errorf("members = %+v", members)
	}

	empty, err := loadGroupNickNames(dbPath, "not-exist@chatroom")
	if err != nil {
		t.Fatalf("missing chat: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("missing chat members = %+v", empty)
	}
}
