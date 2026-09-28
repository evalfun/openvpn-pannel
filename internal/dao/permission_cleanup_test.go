package dao

import (
	"testing"

	"openvpn-pannel/internal/models"
)

// 删除用户时应按 obj_id 清理该用户自己的服务器权限：
// 既不能留下被删用户的悬挂权限，也不能误删其它用户的权限。
func TestBlukDeleteUserCleansOwnServerPermissions(t *testing.T) {
	dm := newTestDaoManager(t)
	for _, name := range []string{"u1", "u2", "u3"} {
		if err := dm.CreateUserWithGroup(name, "pw", "", ""); err != nil {
			t.Fatalf("create user %s: %v", name, err)
		}
	}
	u1 := mustUser(t, dm, "u1")
	u2 := mustUser(t, dm, "u2")
	u3 := mustUser(t, dm, "u3")

	// 仅给 u2、u3 各一条权限：权限行主键(1,2)与用户 id(2,3)错位，
	// 若按权限主键 id 删除就会误删/漏删。
	mustAddPerm := func(objID uint) {
		t.Helper()
		if err := dm.AddServerPermission(&models.ServerPermission{
			ServerID: 1, ObjType: models.SERVER_PERM_OBJ_TYPE_USER, ObjID: objID, Action: models.SERVER_PERM_ACTION_PERMIT,
		}); err != nil {
			t.Fatalf("add permission for %d: %v", objID, err)
		}
	}
	mustAddPerm(u2.ID)
	mustAddPerm(u3.ID)

	if err := dm.BlukDeleteUser([]uint{u3.ID}); err != nil {
		t.Fatalf("BlukDeleteUser: %v", err)
	}

	var perms []models.ServerPermission
	if err := dm.DB.Where("obj_type = ?", models.SERVER_PERM_OBJ_TYPE_USER).Find(&perms).Error; err != nil {
		t.Fatalf("query permissions: %v", err)
	}
	if len(perms) != 1 {
		t.Fatalf("剩余用户权限数 = %d, want 1: %+v", len(perms), perms)
	}
	if perms[0].ObjID != u2.ID {
		t.Fatalf("剩余权限应属于 u2(id=%d)，实际 obj_id=%d", u2.ID, perms[0].ObjID)
	}
	_ = u1
}

// 删除用户组时应按 obj_id 清理该组自己的服务器权限。
func TestDeleteGroupCleansOwnServerPermissions(t *testing.T) {
	dm := newTestDaoManager(t)
	for _, name := range []string{"g1", "g2", "g3"} {
		if err := dm.CreateGroup(name, "", 0, 0); err != nil {
			t.Fatalf("create group %s: %v", name, err)
		}
	}
	g1 := mustGroup(t, dm, "g1")
	g2 := mustGroup(t, dm, "g2")
	g3 := mustGroup(t, dm, "g3")

	mustAddPerm := func(objID uint) {
		t.Helper()
		if err := dm.AddServerPermission(&models.ServerPermission{
			ServerID: 1, ObjType: models.SERVER_PERM_OBJ_TYPE_GROUP, ObjID: objID, Action: models.SERVER_PERM_ACTION_PERMIT,
		}); err != nil {
			t.Fatalf("add permission for group %d: %v", objID, err)
		}
	}
	mustAddPerm(g2.ID)
	mustAddPerm(g3.ID)

	if err := dm.DeleteGroup(g3.ID); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}

	var perms []models.ServerPermission
	if err := dm.DB.Where("obj_type = ?", models.SERVER_PERM_OBJ_TYPE_GROUP).Find(&perms).Error; err != nil {
		t.Fatalf("query permissions: %v", err)
	}
	if len(perms) != 1 {
		t.Fatalf("剩余用户组权限数 = %d, want 1: %+v", len(perms), perms)
	}
	if perms[0].ObjID != g2.ID {
		t.Fatalf("剩余权限应属于 g2(id=%d)，实际 obj_id=%d", g2.ID, perms[0].ObjID)
	}
	_ = g1
}
