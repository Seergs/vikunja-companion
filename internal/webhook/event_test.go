package webhook

import (
	"errors"
	"testing"
	"time"
)

func TestParseReminderFired(t *testing.T) {
	body := []byte(`{
	  "event_name": "task.reminder.fired",
	  "time": "2026-08-29T09:00:00.123456-06:00",
	  "data": {
	    "task": {"id": 12, "title": "Buy milk", "identifier": "HOME-12", "done": false,
	             "due_date": "2026-08-30T18:00:00Z", "project_id": 3},
	    "user": {"id": 1, "username": "sergio"},
	    "project": {"id": 3, "title": "Home"},
	    "reminder": {"reminder": "2026-08-29T09:00:00Z", "relative_period": 0, "relative_to": ""}
	  }
	}`)

	ev, err := Parse(body)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Name != EventReminderFired || ev.ReminderFired == nil {
		t.Fatalf("ev = %+v", ev)
	}
	if ev.TaskOverdue != nil || ev.TasksOverdue != nil {
		t.Error("other payloads should be nil")
	}
	d := ev.ReminderFired
	if d.Task.ID != 12 || d.Task.Title != "Buy milk" || d.Task.Identifier != "HOME-12" {
		t.Errorf("task = %+v", d.Task)
	}
	if d.Task.DueDate.IsZero() {
		t.Error("due date should be set")
	}
	if d.User.ID != 1 || d.Project.Title != "Home" {
		t.Errorf("user/project = %+v / %+v", d.User, d.Project)
	}
	if !d.Reminder.Equal(time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("reminder = %v", d.Reminder)
	}
	if ev.Recipient().ID != 1 {
		t.Errorf("recipient = %+v", ev.Recipient())
	}
}

func TestParseTaskOverdueUnsetDueDate(t *testing.T) {
	body := []byte(`{
	  "event_name": "task.overdue",
	  "time": "2026-08-29T09:00:00Z",
	  "data": {
	    "task": {"id": 5, "title": "Old task", "due_date": "0001-01-01T00:00:00Z", "project_id": 1},
	    "user": {"id": 2, "username": "ada"},
	    "project": {"id": 1, "title": "Inbox"}
	  }
	}`)

	ev, err := Parse(body)
	if err != nil {
		t.Fatal(err)
	}
	if ev.TaskOverdue == nil {
		t.Fatal("TaskOverdue nil")
	}
	if !ev.TaskOverdue.Task.DueDate.IsZero() {
		t.Errorf("zero-year due_date should map to zero time, got %v", ev.TaskOverdue.Task.DueDate)
	}
}

func TestParseTasksOverdueBatch(t *testing.T) {
	body := []byte(`{
	  "event_name": "tasks.overdue",
	  "time": "2026-08-29T09:00:00Z",
	  "data": {
	    "tasks": [
	      {"id": 5, "title": "A", "project_id": 1},
	      {"id": 9, "title": "B", "project_id": 6}
	    ],
	    "user": {"id": 2, "username": "ada"},
	    "projects": {
	      "1": {"id": 1, "title": "Inbox"},
	      "6": {"id": 6, "title": "Work"}
	    }
	  }
	}`)

	ev, err := Parse(body)
	if err != nil {
		t.Fatal(err)
	}
	d := ev.TasksOverdue
	if d == nil || len(d.Tasks) != 2 {
		t.Fatalf("tasks = %+v", d)
	}
	if d.Projects[6].Title != "Work" || d.Projects[1].Title != "Inbox" {
		t.Errorf("projects = %+v", d.Projects)
	}
	if d.User.ID != 2 {
		t.Errorf("user = %+v", d.User)
	}
}

func TestParseUnsupportedEvent(t *testing.T) {
	body := []byte(`{"event_name": "task.created", "time": "2026-08-29T09:00:00Z", "data": {}}`)

	_, err := Parse(body)
	var ue *ErrUnsupportedEvent
	if !errors.As(err, &ue) || ue.Name != "task.created" {
		t.Fatalf("err = %v, want ErrUnsupportedEvent{task.created}", err)
	}
}

func TestParseMalformed(t *testing.T) {
	for _, body := range []string{
		``,
		`not json`,
		`{"event_name": ""}`,
		`{"event_name": "task.overdue", "data": "not an object"}`,
	} {
		if _, err := Parse([]byte(body)); err == nil {
			t.Errorf("Parse(%q) = nil error", body)
		}
	}
}

func TestKnownEvent(t *testing.T) {
	if !KnownEvent(EventTasksOverdue) || KnownEvent("task.created") {
		t.Fatal("KnownEvent wrong")
	}
	if len(KnownEvents) != 3 {
		t.Fatalf("KnownEvents = %v", KnownEvents)
	}
}
