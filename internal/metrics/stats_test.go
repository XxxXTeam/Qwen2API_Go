package metrics

import "testing"

func TestRecordRequestExcludesAdminFromBusinessTotals(t *testing.T) {
	stats := NewDashboardStats()

	stats.RecordRequest("admin", 200)
	stats.RecordRequest("chat", 200)

	snapshot := stats.Snapshot()
	totals := snapshot["totals"].(map[string]int)

	if totals["requests"] != 1 {
		t.Fatalf("requests = %d, want 1", totals["requests"])
	}
	if totals["admin"] != 1 {
		t.Fatalf("admin = %d, want 1", totals["admin"])
	}
}
