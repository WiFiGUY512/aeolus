package conditions

import (
	"strings"
	"testing"
)

// What each client moved is read by time, the span's rows alone, and each
// one's last host name by index (0110): not by reading back through every
// row kept, which on the lab manager took the usage page two seconds.
func TestClientTotalsByIndex(t *testing.T) {
	s, _ := open(t)
	rows, err := s.db.Query(`EXPLAIN QUERY PLAN `+clientTotals, "2026-10-24T00:00:00.000000000Z")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	all := strings.Join(plan, "; ")
	if !strings.Contains(all, "SEARCH c USING INDEX client_usage_at") || strings.Count(all, "USING INDEX client_usage_named") != 3 || strings.Contains(all, "SCAN") {
		t.Fatalf("plan: %s", all)
	}
}
