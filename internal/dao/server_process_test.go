package dao

import "testing"

func TestServerProcessRecordCRUD(t *testing.T) {
	dm := newTestDaoManager(t)

	if err := dm.SaveServerProcess(1, 1234); err != nil {
		t.Fatalf("save: %v", err)
	}
	// 同一服务器再次保存应更新而不是新增。
	if err := dm.SaveServerProcess(1, 5678); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	if err := dm.SaveServerProcess(2, 4321); err != nil {
		t.Fatalf("save 2: %v", err)
	}

	records, err := dm.ListServerProcessRecords()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
	for _, r := range records {
		if r.ServerID == 1 && r.PID != 5678 {
			t.Fatalf("server 1 PID = %d, want 5678", r.PID)
		}
	}

	if err := dm.DeleteServerProcess(1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	records, _ = dm.ListServerProcessRecords()
	if len(records) != 1 || records[0].ServerID != 2 {
		t.Fatalf("after delete: %+v", records)
	}

	if err := dm.DeleteAllServerProcess(); err != nil {
		t.Fatalf("delete all: %v", err)
	}
	records, _ = dm.ListServerProcessRecords()
	if len(records) != 0 {
		t.Fatalf("after delete all: %+v", records)
	}
}
