package dao

import (
	"testing"
)

func TestResourceSetActiveAndContent(t *testing.T) {
	dm := newTestDaoManager(t)

	// 初始无记录时返回 fallback
	got, _ := dm.GetActiveResourceSetID("linux-iptables")
	if got != "linux-iptables" {
		t.Fatalf("初始 active set = %q, 期望 fallback linux-iptables", got)
	}

	// 写入并读回
	if err := dm.SetActiveResourceSetID("openwrt-iptables"); err != nil {
		t.Fatalf("SetActiveResourceSetID: %v", err)
	}
	got, err := dm.GetActiveResourceSetID("linux-iptables")
	if err != nil {
		t.Fatalf("GetActiveResourceSetID: %v", err)
	}
	if got != "openwrt-iptables" {
		t.Fatalf("active set = %q, 期望 openwrt-iptables", got)
	}

	// 再次切换应覆盖同一行，不新增
	if err := dm.SetActiveResourceSetID("linux-nftables"); err != nil {
		t.Fatalf("SetActiveResourceSetID 2: %v", err)
	}
	got, _ = dm.GetActiveResourceSetID("")
	if got != "linux-nftables" {
		t.Fatalf("active set = %q, 期望 linux-nftables", got)
	}
}

func TestResourceSetContentIsolation(t *testing.T) {
	dm := newTestDaoManager(t)

	// 同一 resourceID 在两个资源集内互不影响
	if err := dm.WriteResourceBySet("linux-iptables", "client_online.sh", "A"); err != nil {
		t.Fatalf("write A: %v", err)
	}
	if err := dm.WriteResourceBySet("openwrt-iptables", "client_online.sh", "B"); err != nil {
		t.Fatalf("write B: %v", err)
	}

	a, err := dm.GetResourceBySet("linux-iptables", "client_online.sh")
	if err != nil || a.Content != "A" {
		t.Fatalf("linux-iptables = %v, %v", a, err)
	}
	b, err := dm.GetResourceBySet("openwrt-iptables", "client_online.sh")
	if err != nil || b.Content != "B" {
		t.Fatalf("openwrt-iptables = %v, %v", b, err)
	}

	// 统计
	if n, _ := dm.CountResourceBySet("linux-iptables"); n != 1 {
		t.Fatalf("linux-iptables count = %d, 期望 1", n)
	}

	// 覆盖写（delete+create）不应产生重复
	if err := dm.WriteResourceBySet("linux-iptables", "client_online.sh", "A2"); err != nil {
		t.Fatalf("write A2: %v", err)
	}
	a, _ = dm.GetResourceBySet("linux-iptables", "client_online.sh")
	if a.Content != "A2" {
		t.Fatalf("覆盖后 = %q, 期望 A2", a.Content)
	}
	if n, _ := dm.CountResourceBySet("linux-iptables"); n != 1 {
		t.Fatalf("覆盖后 count = %d, 期望 1", n)
	}

	// 删除只影响指定资源集
	if err := dm.DeleteResourceBySet("linux-iptables", "client_online.sh"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := dm.GetResourceBySet("linux-iptables", "client_online.sh"); err == nil {
		t.Fatalf("删除后不应再读到")
	}
	if _, err := dm.GetResourceBySet("openwrt-iptables", "client_online.sh"); err != nil {
		t.Fatalf("删除 linux-iptables 不应影响 openwrt-iptables: %v", err)
	}
}

func TestResourceByIDListInSet(t *testing.T) {
	dm := newTestDaoManager(t)
	_ = dm.WriteResourceBySet("linux-iptables", "config", "C1")
	_ = dm.WriteResourceBySet("linux-iptables", "auth.sh", "C2")
	_ = dm.WriteResourceBySet("openwrt-iptables", "config", "X")

	list, err := dm.GetResourceByIDListInSet("linux-iptables", []string{"config", "auth.sh", "misc"})
	if err != nil {
		t.Fatalf("GetResourceByIDListInSet: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("返回 %d 条, 期望 2", len(list))
	}
}
