package config

import "testing"

// 启用内存事件（max_memory_events>0）时应强制停止实例，忽略 stop_instances_on_exit 的配置。
func TestShouldStopInstancesOnExitForcedByMemoryEvents(t *testing.T) {
	boolPtr := func(v bool) *bool { return &v }

	// 内存模式 + 显式 false：仍应返回 true
	if c := (&Config{MaxMemoryEvents: 10, StopInstancesOnExit: boolPtr(false)}); !c.ShouldStopInstancesOnExit() {
		t.Fatalf("内存模式应强制 stop_instances_on_exit=true")
	}
	// 内存模式 + 显式 true：true
	if c := (&Config{MaxMemoryEvents: 10, StopInstancesOnExit: boolPtr(true)}); !c.ShouldStopInstancesOnExit() {
		t.Fatalf("内存模式应为 true")
	}
	// 内存模式 + 未配置：true
	if c := (&Config{MaxMemoryEvents: 10}); !c.ShouldStopInstancesOnExit() {
		t.Fatalf("内存模式应为 true")
	}

	// 非内存模式：尊重配置
	if c := (&Config{StopInstancesOnExit: boolPtr(false)}); c.ShouldStopInstancesOnExit() {
		t.Fatalf("非内存模式显式 false 应为 false")
	}
	if c := (&Config{StopInstancesOnExit: boolPtr(true)}); !c.ShouldStopInstancesOnExit() {
		t.Fatalf("非内存模式显式 true 应为 true")
	}
	if c := (&Config{}); !c.ShouldStopInstancesOnExit() {
		t.Fatalf("非内存模式未配置应默认 true")
	}
}
