package digest

import (
	"testing"

	"github.com/seergs/vikunja-companion/internal/vikunja"
)

func tasks(priorities ...int64) []vikunja.Task {
	out := make([]vikunja.Task, len(priorities))
	for i, p := range priorities {
		out[i] = vikunja.Task{ID: int64(i + 1), Title: "t", Priority: p}
	}
	return out
}

func TestBuild(t *testing.T) {
	cases := []struct {
		name  string
		tasks []vikunja.Task
		want  string // "" means no notification
	}{
		{"none", nil, ""},
		{"one, not urgent", tasks(2), "You have 1 task due in Vikunja today."},
		{"several, none urgent", tasks(0, 1, 3), "You have 3 tasks due in Vikunja today."},
		{"urgent priority 4", tasks(1, 4), "You have 2 tasks due in Vikunja today. 1 is urgent."},
		{"do-now counts as urgent", tasks(5, 5, 2), "You have 3 tasks due in Vikunja today. 2 are urgent."},
		{"priority 3 is not urgent", tasks(3, 3), "You have 2 tasks due in Vikunja today."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Build(tc.tasks, "digest:1:2026-08-29")
			if tc.want == "" {
				if got != nil {
					t.Fatalf("want nil, got %+v", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("want 1 notification, got %d", len(got))
			}
			n := got[0]
			if n.Body != tc.want {
				t.Errorf("body = %q, want %q", n.Body, tc.want)
			}
			if n.Title != "Daily briefing" || n.Deeplink != "today" {
				t.Errorf("title/deeplink = %q/%q", n.Title, n.Deeplink)
			}
			if n.DedupeKey != "digest:1:2026-08-29" {
				t.Errorf("dedupe key = %q", n.DedupeKey)
			}
		})
	}
}

func TestParseHHMM(t *testing.T) {
	ok := map[string][2]int{"08:00": {8, 0}, "0:05": {0, 5}, "23:59": {23, 59}}
	for in, want := range ok {
		h, m, err := ParseHHMM(in)
		if err != nil || h != want[0] || m != want[1] {
			t.Errorf("ParseHHMM(%q) = %d,%d,%v", in, h, m, err)
		}
	}
	for _, bad := range []string{"", "8", "24:00", "08:60", "-1:00", "aa:bb", "08:00:00"} {
		if _, _, err := ParseHHMM(bad); err == nil {
			t.Errorf("ParseHHMM(%q) should error", bad)
		}
	}
}
