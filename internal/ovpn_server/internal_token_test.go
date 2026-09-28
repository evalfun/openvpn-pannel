package ovpnserver

import (
	"strings"
	"testing"
)

// 带令牌占位符的新脚本：替换占位符即可，不应额外注入 curl 包装。
func TestInjectInternalAPIAccessPlaceholder(t *testing.T) {
	script := "#!/bin/bash\n" +
		"INTERNAL_API=\"__INTERNAL_API__\"\n" +
		"INTERNAL_API_TOKEN=\"__INTERNAL_API_TOKEN__\"\n" +
		"curl -s -H \"X-Internal-Token: $INTERNAL_API_TOKEN\" http://$INTERNAL_API/user/auth\n"
	out := InjectInternalAPIAccess(script, "127.0.0.1:59003", "tok-123")
	if strings.Contains(out, "__INTERNAL_API__") || strings.Contains(out, "__INTERNAL_API_TOKEN__") {
		t.Fatalf("占位符未被替换:\n%s", out)
	}
	if !strings.Contains(out, "127.0.0.1:59003") || !strings.Contains(out, "tok-123") {
		t.Fatalf("注入内容缺失:\n%s", out)
	}
	if strings.Contains(out, "curl() {") {
		t.Fatalf("含占位符的新脚本不应注入 curl 包装:\n%s", out)
	}
}

// 升级前已种子化的旧脚本（无占位符）：应注入 curl 包装函数，且 shebang 仍在首行。
func TestInjectInternalAPIAccessLegacyWrapper(t *testing.T) {
	script := "#!/bin/bash\n" +
		"INTERNAL_API=\"__INTERNAL_API__\"\n" +
		"curl -s -X POST -d \"$body\" http://$INTERNAL_API/user/auth\n"
	out := InjectInternalAPIAccess(script, "127.0.0.1:59003", "tok-abc")
	if !strings.Contains(out, "X-Internal-Token: tok-abc") {
		t.Fatalf("旧脚本应通过包装函数携带令牌:\n%s", out)
	}
	if !strings.HasPrefix(out, "#!/bin/bash\n") {
		t.Fatalf("shebang 必须仍在首行:\n%s", out)
	}
	if strings.Contains(out, "__INTERNAL_API__") {
		t.Fatalf("内部 API 地址占位符未替换:\n%s", out)
	}
}

// 不调用 curl 的脚本无需注入。
func TestInjectInternalAPIAccessNoCurl(t *testing.T) {
	script := "#!/bin/bash\necho hi\n"
	out := InjectInternalAPIAccess(script, "127.0.0.1:1", "tok")
	if strings.Contains(out, "curl()") {
		t.Fatalf("无 curl 的脚本不应注入包装:\n%s", out)
	}
}

// token 为空时保持原样（向后兼容）。
func TestInjectInternalAPIAccessEmptyToken(t *testing.T) {
	script := "curl -s http://__INTERNAL_API__/x\n"
	out := InjectInternalAPIAccess(script, "h:1", "")
	if strings.Contains(out, "__INTERNAL_API_TOKEN__") {
		t.Fatalf("token 为空时不应引入令牌占位符:\n%s", out)
	}
	if !strings.Contains(out, "h:1") {
		t.Fatalf("地址应被替换:\n%s", out)
	}
}

// 内置资源集的认证/上下线脚本都应携带令牌占位符与请求头，确保新部署即具备令牌校验。
func TestDefaultScriptsCarryInternalToken(t *testing.T) {
	for _, set := range ListResourceSets() {
		for _, id := range []string{RESOURCE_ID_AUTH_SCRIPT, RESOURCE_ID_CLIENT_ONLINE_SCRIPT, RESOURCE_ID_CLIENT_OFFLINE_SCRIPT} {
			content := GetSetDefaultResource(set.ID, id)
			if !strings.Contains(content, "__INTERNAL_API_TOKEN__") {
				t.Errorf("资源集 %s 的 %s 缺少 __INTERNAL_API_TOKEN__ 占位符", set.ID, id)
			}
			if !strings.Contains(content, "X-Internal-Token") {
				t.Errorf("资源集 %s 的 %s 缺少 X-Internal-Token 请求头", set.ID, id)
			}
		}
	}
}
