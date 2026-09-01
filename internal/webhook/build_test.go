package webhook

import (
	"testing"
	"time"

	"github.com/seergs/vikunja-companion/internal/notify"
)

func TestBuildReminder(t *testing.T) {
	ev := &Event{
		Name: EventReminderFired,
		Time: time.Now(),
		ReminderFired: &ReminderFiredData{
			Task:     Task{ID: 12, Title: "Buy milk"},
			User:     User{ID: 1},
			Reminder: time.Unix(1_756_468_800, 0),
		},
	}
	ns := Build(ev)
	if len(ns) != 1 {
		t.Fatalf("got %d notifications", len(ns))
	}
	if ns[0].Title != "Buy milk" || ns[0].Deeplink != "task/12" {
		t.Errorf("notif = %+v", ns[0])
	}
	if ns[0].DedupeKey != "reminder:12:1756468800" {
		t.Errorf("dedupe = %q", ns[0].DedupeKey)
	}
	if ns[0].Level != "" {
		t.Errorf("reminder should be default level, got %q", ns[0].Level)
	}
}

func TestBuildOverdueIsWarning(t *testing.T) {
	at := time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC)
	single := Build(&Event{TaskOverdue: &TaskOverdueData{Task: Task{ID: 5, Title: "x"}, User: User{ID: 1}}, Time: at})
	batch := Build(&Event{TasksOverdue: &TasksOverdueData{User: User{ID: 7}, Tasks: []Task{{ID: 1}, {ID: 2}}}, Time: at})
	if single[0].Level != notify.LevelWarning || batch[0].Level != notify.LevelWarning {
		t.Errorf("overdue notifications must be warnings: single=%q batch=%q", single[0].Level, batch[0].Level)
	}
}

func TestBuildTasksOverdueBatch(t *testing.T) {
	at := time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC)
	ev := &Event{
		Name: EventTasksOverdue,
		Time: at,
		TasksOverdue: &TasksOverdueData{
			User:  User{ID: 7},
			Tasks: []Task{{ID: 1, Title: "A"}, {ID: 2, Title: "B"}, {ID: 3, Title: "C"}},
		},
	}
	ns := Build(ev)
	if len(ns) != 1 || ns[0].Body != "You have 3 overdue tasks" || ns[0].Deeplink != "today" {
		t.Fatalf("notif = %+v", ns)
	}
	if ns[0].DedupeKey != "overdue-batch:7:2026-08-29" {
		t.Errorf("dedupe = %q", ns[0].DedupeKey)
	}
}

func TestBuildTasksOverdueSingle(t *testing.T) {
	at := time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC)
	ev := &Event{
		Name:         EventTasksOverdue,
		Time:         at,
		TasksOverdue: &TasksOverdueData{User: User{ID: 7}, Tasks: []Task{{ID: 9, Title: "Solo"}}},
	}
	ns := Build(ev)
	if len(ns) != 1 || ns[0].Title != "Solo" || ns[0].Deeplink != "task/9" {
		t.Fatalf("notif = %+v", ns)
	}
	// Same dedupe key family as the batch, so a batch + a 1-task delivery on the
	// same day collapse.
	if ns[0].DedupeKey != "overdue-batch:7:2026-08-29" {
		t.Errorf("dedupe = %q", ns[0].DedupeKey)
	}
}

func TestBuildTaskLabelFallback(t *testing.T) {
	ev := &Event{
		TaskOverdue: &TaskOverdueData{Task: Task{ID: 5}, User: User{ID: 1}},
		Time:        time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC),
	}
	ns := Build(ev)
	if ns[0].Title != "Task 5" {
		t.Errorf("title = %q, want 'Task 5'", ns[0].Title)
	}
}

func TestBuildEmptyBatch(t *testing.T) {
	ev := &Event{TasksOverdue: &TasksOverdueData{User: User{ID: 1}}, Time: time.Now()}
	if Build(ev) != nil {
		t.Error("empty batch should build nothing")
	}
}
