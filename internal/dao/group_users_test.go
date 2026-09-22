package dao

import "testing"

func TestListUsersInGroupPaged(t *testing.T) {
	dm := newTestDaoManager(t)
	if err := dm.CreateGroup("g1", "", 0, 0); err != nil {
		t.Fatalf("create group: %v", err)
	}
	group, err := dm.GetGroupByName("g1")
	if err != nil {
		t.Fatalf("get group: %v", err)
	}

	for _, name := range []string{"u1", "u2", "u3", "u4", "u5"} {
		if err := dm.CreateUserWithGroup(name, "pw", "", ""); err != nil {
			t.Fatalf("create user %s: %v", name, err)
		}
	}
	// u6 不入组，用于确认分页只统计组内用户
	if err := dm.CreateUserWithGroup("u6", "pw", "", ""); err != nil {
		t.Fatalf("create u6: %v", err)
	}

	// 直接构造组内关系（CreateUserWithGroup 也可，但这里用 AddUserToGroup 更直观）
	for _, name := range []string{"u1", "u2", "u3", "u4", "u5"} {
		u, err := dm.GetUserByUsername(name)
		if err != nil {
			t.Fatalf("get %s: %v", name, err)
		}
		if err := dm.AddUserToGroup(u.ID, group.ID); err != nil {
			t.Fatalf("add %s: %v", name, err)
		}
	}

	// 第 1 页，每页 2 个
	page1, count, err := dm.ListUsersInGroupPaged(group.ID, "", 1, 2)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if count != 5 {
		t.Fatalf("count = %d, want 5", count)
	}
	if len(page1) != 2 || page1[0].Username != "u1" || page1[1].Username != "u2" {
		t.Fatalf("page1 = %+v", page1)
	}

	// 第 3 页只剩 1 个
	page3, _, err := dm.ListUsersInGroupPaged(group.ID, "", 3, 2)
	if err != nil {
		t.Fatalf("page3: %v", err)
	}
	if len(page3) != 1 || page3[0].Username != "u5" {
		t.Fatalf("page3 = %+v", page3)
	}

	// 按用户名搜索
	filtered, count, err := dm.ListUsersInGroupPaged(group.ID, "u4", 1, 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if count != 1 || len(filtered) != 1 || filtered[0].Username != "u4" {
		t.Fatalf("search result = %+v count=%d", filtered, count)
	}
}

func TestBatchRemoveUsersFromGroup(t *testing.T) {
	dm := newTestDaoManager(t)
	if err := dm.CreateGroup("g1", "", 0, 0); err != nil {
		t.Fatalf("create group: %v", err)
	}
	for _, name := range []string{"u1", "u2", "u3"} {
		if err := dm.CreateUserWithGroup(name, "pw", "", "g1"); err != nil {
			t.Fatalf("create user %s: %v", name, err)
		}
	}
	group, _ := dm.GetGroupByName("g1")

	// 批量移除 u1/u2，并混入一个不存在的用户（应被忽略）
	if err := dm.BatchRemoveUsersFromGroup([]string{"u1", "u2", "not-exist"}, "g1"); err != nil {
		t.Fatalf("BatchRemoveUsersFromGroup: %v", err)
	}
	_, count, err := dm.ListUsersInGroupPaged(group.ID, "", 1, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if count != 1 {
		t.Fatalf("移除后组内用户数 = %d, want 1", count)
	}
	if u, err := dm.GetUserByUsername("u3"); err != nil {
		t.Fatalf("u3: %v", err)
	} else if in, _ := dm.UserInGroup(u.ID, group.ID); !in {
		t.Fatalf("u3 应仍在组内")
	}

	// 组不存在时报错
	if err := dm.BatchRemoveUsersFromGroup([]string{"u3"}, "no-such-group"); err == nil {
		t.Fatalf("组不存在时应报错")
	}
}
