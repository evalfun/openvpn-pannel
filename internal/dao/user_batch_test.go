package dao

import (
	"strings"
	"testing"
)

func TestCreateUserWithGroup(t *testing.T) {
	dm := newTestDaoManager(t)

	if err := dm.CreateGroup("vip", "", 0, 0); err != nil {
		t.Fatalf("create group: %v", err)
	}
	vip, err := dm.GetGroupByName("vip")
	if err != nil {
		t.Fatalf("get group: %v", err)
	}

	// 1) 指定用户组：创建成功且已入组
	if err := dm.CreateUserWithGroup("alice", "pw1", "", "vip"); err != nil {
		t.Fatalf("create alice: %v", err)
	}
	alice, err := dm.GetUserByUsername("alice")
	if err != nil {
		t.Fatalf("get alice: %v", err)
	}
	inGroup, err := dm.UserInGroup(alice.ID, vip.ID)
	if err != nil || !inGroup {
		t.Fatalf("alice should be in vip group: in=%v err=%v", inGroup, err)
	}

	// 2) 用户组为空：创建成功但不入任何组
	if err := dm.CreateUserWithGroup("bob", "pw2", "", ""); err != nil {
		t.Fatalf("create bob: %v", err)
	}
	bob, err := dm.GetUserByUsername("bob")
	if err != nil {
		t.Fatalf("get bob: %v", err)
	}
	groups, err := dm.ListGroupsForUser(bob.ID)
	if err != nil {
		t.Fatalf("list groups for bob: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("bob should not belong to any group, got %v", groups)
	}

	// 3) 用户名重复：失败
	if err := dm.CreateUserWithGroup("alice", "pw", "", ""); err == nil || !strings.Contains(err.Error(), "已存在") {
		t.Fatalf("expected duplicate username error, got %v", err)
	}

	// 4) 用户组不存在：失败，且不应创建用户
	if err := dm.CreateUserWithGroup("carol", "pw", "", "no-such-group"); err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("expected group not found error, got %v", err)
	}
	if _, err := dm.GetUserByUsername("carol"); err == nil {
		t.Fatal("carol should not be created when group does not exist")
	}
}
