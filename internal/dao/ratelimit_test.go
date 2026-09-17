package dao

import (
	"path/filepath"
	"testing"

	"openvpn-pannel/internal/config"
	"openvpn-pannel/internal/models"
)

func newTestDaoManager(t *testing.T) *DaoManager {
	t.Helper()
	cfg := &config.Config{
		SQLiteDB:     filepath.Join(t.TempDir(), "test.db"),
		PasswordSalt: "test-salt",
	}
	dm, err := NewDaoManager(cfg)
	if err != nil {
		t.Fatalf("NewDaoManager: %v", err)
	}
	if err := models.MigrateDB(dm.DB); err != nil {
		t.Fatalf("MigrateDB: %v", err)
	}
	return dm
}

func mustUser(t *testing.T, dm *DaoManager, username string) *models.User {
	t.Helper()
	u, err := dm.GetUserByUsername(username)
	if err != nil {
		t.Fatalf("GetUserByUsername(%s): %v", username, err)
	}
	return u
}

func mustGroup(t *testing.T, dm *DaoManager, name string) *models.Group {
	t.Helper()
	g, err := dm.GetGroupByName(name)
	if err != nil {
		t.Fatalf("GetGroupByName(%s): %v", name, err)
	}
	return g
}

func TestMinRateIgnoresZero(t *testing.T) {
	// 0 视为不限速，取最低时忽略；没有有限值时返回 0
	if got := minRate([]uint64{100, 0, 50}); got != 50 {
		t.Fatalf("minRate = %d, want 50", got)
	}
	if got := minRate([]uint64{0, 0}); got != 0 {
		t.Fatalf("minRate all-zero = %d, want 0", got)
	}
	if got := minRate(nil); got != 0 {
		t.Fatalf("minRate nil = %d, want 0", got)
	}
}

func TestMaxRateZeroMeansUnlimited(t *testing.T) {
	// 0 视为不限速，取最高时任一为 0 即整体不限速
	if got := maxRate([]uint64{100, 0, 50}); got != 0 {
		t.Fatalf("maxRate with zero = %d, want 0", got)
	}
	if got := maxRate([]uint64{100, 50, 200}); got != 200 {
		t.Fatalf("maxRate = %d, want 200", got)
	}
	if got := maxRate(nil); got != 0 {
		t.Fatalf("maxRate nil = %d, want 0", got)
	}
}

// 验证活跃用户组判定与限速计算：用户同属 A/B 两组，服务器只允许 A 连接时，
// 活跃组只有 A（对应需求里的例子）。
func TestResolveRateLimitActiveGroupRestriction(t *testing.T) {
	dm := newTestDaoManager(t)

	if err := dm.CreateGroup("groupA", "", 100, 200); err != nil {
		t.Fatalf("CreateGroup A: %v", err)
	}
	if err := dm.CreateGroup("groupB", "", 300, 400); err != nil {
		t.Fatalf("CreateGroup B: %v", err)
	}
	if err := dm.CreateUser("user1", "pass", "", models.RATE_LIMIT_TYPE_ACTIVE_GROUP_MIN, 0, 0); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	u := mustUser(t, dm, "user1")
	gA := mustGroup(t, dm, "groupA")
	gB := mustGroup(t, dm, "groupB")
	if err := dm.AddUserToGroup(u.ID, gA.ID); err != nil {
		t.Fatalf("AddUserToGroup A: %v", err)
	}
	if err := dm.AddUserToGroup(u.ID, gB.ID); err != nil {
		t.Fatalf("AddUserToGroup B: %v", err)
	}

	const serverID = 1
	// 服务器只允许 groupA
	if err := dm.AddServerPermission(&models.ServerPermission{
		ServerID: serverID, ObjType: models.SERVER_PERM_OBJ_TYPE_GROUP,
		ObjID: gA.ID, Action: models.SERVER_PERM_ACTION_PERMIT,
	}); err != nil {
		t.Fatalf("AddServerPermission: %v", err)
	}

	active, err := dm.ListActiveGroupsForUser(u.ID, serverID)
	if err != nil {
		t.Fatalf("ListActiveGroupsForUser: %v", err)
	}
	if len(active) != 1 || active[0].ID != gA.ID {
		t.Fatalf("active groups = %+v, want only groupA", active)
	}

	// 默认策略(=1)取活跃组最低速率
	up, down, err := dm.ResolveUserRateLimit(u.ID, serverID)
	if err != nil {
		t.Fatalf("ResolveUserRateLimit: %v", err)
	}
	if up != 100 || down != 200 {
		t.Fatalf("rate = (%d,%d), want (100,200) from groupA", up, down)
	}
}

func TestResolveRateLimitPolicies(t *testing.T) {
	dm := newTestDaoManager(t)
	if err := dm.CreateGroup("g1", "", 100, 0); err != nil {
		t.Fatalf("CreateGroup g1: %v", err)
	}
	if err := dm.CreateGroup("g2", "", 0, 50); err != nil {
		t.Fatalf("CreateGroup g2: %v", err)
	}
	g1 := mustGroup(t, dm, "g1")
	g2 := mustGroup(t, dm, "g2")
	const serverID = 2
	for _, g := range []*models.Group{g1, g2} {
		if err := dm.AddServerPermission(&models.ServerPermission{
			ServerID: serverID, ObjType: models.SERVER_PERM_OBJ_TYPE_GROUP,
			ObjID: g.ID, Action: models.SERVER_PERM_ACTION_PERMIT,
		}); err != nil {
			t.Fatalf("AddServerPermission: %v", err)
		}
	}

	cases := []struct {
		name             string
		rateLimitType    uint
		userUp, userDown uint64
		wantUp, wantDown uint64
	}{
		// 活跃组 g1=(100,0) g2=(0,50)
		{"active_group_min", models.RATE_LIMIT_TYPE_ACTIVE_GROUP_MIN, 0, 0, 100, 50},
		// 最高速率：任一活跃组为 0 则整体不限速
		{"active_group_max", models.RATE_LIMIT_TYPE_ACTIVE_GROUP_MAX, 0, 0, 0, 0},
		{"fixed", models.RATE_LIMIT_TYPE_FIXED, 10, 20, 10, 20},
		{"none", models.RATE_LIMIT_TYPE_NONE, 10, 20, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := dm.CreateUser("u_"+tc.name, "p", "", tc.rateLimitType, tc.userUp, tc.userDown); err != nil {
				t.Fatalf("CreateUser: %v", err)
			}
			u := mustUser(t, dm, "u_"+tc.name)
			if err := dm.AddUserToGroup(u.ID, g1.ID); err != nil {
				t.Fatalf("AddUserToGroup g1: %v", err)
			}
			if err := dm.AddUserToGroup(u.ID, g2.ID); err != nil {
				t.Fatalf("AddUserToGroup g2: %v", err)
			}
			up, down, err := dm.ResolveUserRateLimit(u.ID, serverID)
			if err != nil {
				t.Fatalf("ResolveUserRateLimit: %v", err)
			}
			if up != tc.wantUp || down != tc.wantDown {
				t.Fatalf("rate = (%d,%d), want (%d,%d)", up, down, tc.wantUp, tc.wantDown)
			}
		})
	}
}

func TestResolveRateLimitUserPermitAllGroupsAndDeny(t *testing.T) {
	dm := newTestDaoManager(t)
	if err := dm.CreateGroup("ga", "", 111, 222); err != nil {
		t.Fatalf("CreateGroup ga: %v", err)
	}
	if err := dm.CreateGroup("gb", "", 333, 444); err != nil {
		t.Fatalf("CreateGroup gb: %v", err)
	}
	ga := mustGroup(t, dm, "ga")
	gb := mustGroup(t, dm, "gb")

	const serverID = 3
	// 用户级放行：即使组权限未配置，所有所属组都活跃
	if err := dm.CreateUser("permit_user", "p", "", models.RATE_LIMIT_TYPE_ACTIVE_GROUP_MIN, 0, 0); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	pu := mustUser(t, dm, "permit_user")
	_ = dm.AddUserToGroup(pu.ID, ga.ID)
	_ = dm.AddUserToGroup(pu.ID, gb.ID)
	if err := dm.AddServerPermission(&models.ServerPermission{
		ServerID: serverID, ObjType: models.SERVER_PERM_OBJ_TYPE_USER,
		ObjID: pu.ID, Action: models.SERVER_PERM_ACTION_PERMIT,
	}); err != nil {
		t.Fatalf("AddServerPermission: %v", err)
	}
	active, err := dm.ListActiveGroupsForUser(pu.ID, serverID)
	if err != nil {
		t.Fatalf("ListActiveGroupsForUser: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("user permit: active groups = %d, want 2", len(active))
	}
	// 最低速率：两组的这两个值都非 0，取最小
	if up, down, _ := dm.ResolveUserRateLimit(pu.ID, serverID); up != 111 || down != 222 {
		t.Fatalf("user permit min rate = (%d,%d), want (111,222)", up, down)
	}

	// 用户级拒绝：无活跃组，默认策略返回不限速
	if err := dm.CreateUser("deny_user", "p", "", models.RATE_LIMIT_TYPE_ACTIVE_GROUP_MIN, 0, 0); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	du := mustUser(t, dm, "deny_user")
	_ = dm.AddUserToGroup(du.ID, ga.ID)
	if err := dm.AddServerPermission(&models.ServerPermission{
		ServerID: serverID, ObjType: models.SERVER_PERM_OBJ_TYPE_USER,
		ObjID: du.ID, Action: models.SERVER_PERM_ACTION_DENY,
	}); err != nil {
		t.Fatalf("AddServerPermission: %v", err)
	}
	active, err = dm.ListActiveGroupsForUser(du.ID, serverID)
	if err != nil {
		t.Fatalf("ListActiveGroupsForUser: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("deny: active groups = %d, want 0", len(active))
	}
	if up, down, _ := dm.ResolveUserRateLimit(du.ID, serverID); up != 0 || down != 0 {
		t.Fatalf("deny min rate = (%d,%d), want (0,0)", up, down)
	}
}
