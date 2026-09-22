package ovpnserver

import (
	"strings"
	"testing"
)

// 确保内置的 ACL 动态放行/回收脚本存在且包含关键占位符与逻辑。
func TestDefaultACLScripts(t *testing.T) {
	for _, id := range []string{RESOURCE_ID_ACL_ADD_SCRIPT, RESOURCE_ID_ACL_DEL_SCRIPT} {
		content := GetDefaultResource(id)
		if content == "" {
			t.Fatalf("默认资源 %s 为空", id)
		}
		for _, want := range []string{"__WORKING_DIR__", "__SERVER_ID__", "__SERVER_INTERFACE__", "virtual_ip:", "flock"} {
			if !strings.Contains(content, want) {
				t.Errorf("资源 %s 缺少 %q", id, want)
			}
		}
	}
	// 回收脚本必须清空该客户端的连接跟踪(状态表)，否则已建立的连接在登出后仍会放行。
	del := GetDefaultResource(RESOURCE_ID_ACL_DEL_SCRIPT)
	for _, want := range []string{"flush_conntrack", "conntrack"} {
		if !strings.Contains(del, want) {
			t.Errorf("acl_del.sh 缺少 %q", want)
		}
	}
}
