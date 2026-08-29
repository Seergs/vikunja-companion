package webhook

import (
	"fmt"
	"strconv"
	"time"

	"github.com/seergs/vikunja-companion/internal/notify"
)

// Build maps a parsed event to the notifications to deliver to its recipient.
// It returns nil for an event that warrants no push.
//
// Deep links are relative (e.g. "task/12", "today"); the app turns them into
// its own URL scheme.
func Build(ev *Event) []notify.Notification {
	switch {
	case ev.ReminderFired != nil:
		return buildReminder(ev.ReminderFired)
	case ev.TaskOverdue != nil:
		return buildTaskOverdue(ev.TaskOverdue, ev.Time)
	case ev.TasksOverdue != nil:
		return buildTasksOverdue(ev.TasksOverdue, ev.Time)
	default:
		return nil
	}
}

func buildReminder(d *ReminderFiredData) []notify.Notification {
	return []notify.Notification{{
		Title:     taskLabel(d.Task),
		Body:      "Reminder",
		Deeplink:  taskLink(d.Task.ID),
		DedupeKey: fmt.Sprintf("reminder:%d:%d", d.Task.ID, d.Reminder.Unix()),
	}}
}

func buildTaskOverdue(d *TaskOverdueData, at time.Time) []notify.Notification {
	return []notify.Notification{{
		Title:     taskLabel(d.Task),
		Body:      "Overdue",
		Deeplink:  taskLink(d.Task.ID),
		DedupeKey: fmt.Sprintf("overdue:%d:%s", d.Task.ID, at.Format("2006-01-02")),
	}}
}

func buildTasksOverdue(d *TasksOverdueData, at time.Time) []notify.Notification {
	n := len(d.Tasks)
	if n == 0 {
		return nil
	}
	if n == 1 {
		t := d.Tasks[0]
		return []notify.Notification{{
			Title:     taskLabel(t),
			Body:      "Overdue",
			Deeplink:  taskLink(t.ID),
			DedupeKey: fmt.Sprintf("overdue-batch:%d:%s", d.User.ID, at.Format("2006-01-02")),
		}}
	}
	return []notify.Notification{{
		Title:     "Overdue tasks",
		Body:      fmt.Sprintf("You have %d overdue tasks", n),
		Deeplink:  "today",
		DedupeKey: fmt.Sprintf("overdue-batch:%d:%s", d.User.ID, at.Format("2006-01-02")),
	}}
}

// taskLabel is a non-empty display string for a task.
func taskLabel(t Task) string {
	switch {
	case t.Title != "":
		return t.Title
	case t.Identifier != "":
		return t.Identifier
	default:
		return "Task " + strconv.FormatInt(t.ID, 10)
	}
}

func taskLink(id int64) string { return "task/" + strconv.FormatInt(id, 10) }
