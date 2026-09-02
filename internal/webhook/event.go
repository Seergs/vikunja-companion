package webhook

import (
	"encoding/json"
	"fmt"
	"time"
)

// Event names — the complete v1 surface. See docs/webhooks-verified.md.
const (
	EventReminderFired = "task.reminder.fired"
	EventTaskOverdue   = "task.overdue"
	EventTasksOverdue  = "tasks.overdue"
)

// KnownEvents is the set the companion understands, in a stable order.
var KnownEvents = []string{EventReminderFired, EventTaskOverdue, EventTasksOverdue}

// KnownEvent reports whether name is one of the three events the companion
// handles.
func KnownEvent(name string) bool {
	switch name {
	case EventReminderFired, EventTaskOverdue, EventTasksOverdue:
		return true
	}
	return false
}

// ErrUnsupportedEvent is returned by Parse for a well-formed envelope whose
// event_name is not one of the three the companion handles.
type ErrUnsupportedEvent struct{ Name string }

func (e *ErrUnsupportedEvent) Error() string {
	return fmt.Sprintf("webhook: unsupported event %q", e.Name)
}

// Task is the subset of a Vikunja task the companion needs to build a
// notification and a deep link.
type Task struct {
	ID         int64
	Title      string
	Identifier string
	Done       bool
	DueDate    time.Time // zero when unset
	ProjectID  int64
}

// Project is the subset of a Vikunja project the companion needs.
type Project struct {
	ID    int64
	Title string
}

// User identifies the recipient of an event.
type User struct {
	ID       int64
	Username string
}

// Event is a parsed, verified webhook delivery. Exactly one of the payload
// pointers is non-nil, matching Name.
type Event struct {
	Name string
	Time time.Time

	ReminderFired *ReminderFiredData
	TaskOverdue   *TaskOverdueData
	TasksOverdue  *TasksOverdueData
}

// Recipient returns the user the event is directed at.
func (e *Event) Recipient() User {
	switch {
	case e.ReminderFired != nil:
		return e.ReminderFired.User
	case e.TaskOverdue != nil:
		return e.TaskOverdue.User
	case e.TasksOverdue != nil:
		return e.TasksOverdue.User
	default:
		return User{}
	}
}

// ReminderFiredData is data for task.reminder.fired.
type ReminderFiredData struct {
	Task     Task
	User     User
	Project  Project
	Reminder time.Time
}

// TaskOverdueData is data for task.overdue.
type TaskOverdueData struct {
	Task    Task
	User    User
	Project Project
}

// TasksOverdueData is data for the batch tasks.overdue.
type TasksOverdueData struct {
	Tasks    []Task
	User     User
	Projects map[int64]Project
}

// ---- wire shapes (Vikunja JSON) ----

type envelope struct {
	EventName string          `json:"event_name"`
	Time      time.Time       `json:"time"`
	Data      json.RawMessage `json:"data"`
}

type wireTask struct {
	ID         int64       `json:"id"`
	Title      string      `json:"title"`
	Identifier string      `json:"identifier"`
	Done       bool        `json:"done"`
	DueDate    vikunjaTime `json:"due_date"`
	ProjectID  int64       `json:"project_id"`
}

func (w wireTask) to() Task {
	return Task{
		ID: w.ID, Title: w.Title, Identifier: w.Identifier,
		Done: w.Done, DueDate: w.DueDate.Time, ProjectID: w.ProjectID,
	}
}

type wireProject struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

func (w wireProject) to() Project { return Project(w) }

type wireUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

func (w wireUser) to() User { return User(w) }

// vikunjaTime unmarshals an RFC3339 string, mapping Go's zero year (Vikunja's
// "unset" marker, "0001-01-01T00:00:00Z") to the zero time.Time.
type vikunjaTime struct{ time.Time }

func (v *vikunjaTime) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return err
	}
	if t.Year() > 1 {
		v.Time = t
	}
	return nil
}

// Parse decodes a raw webhook body (already signature-verified) into an Event.
// It returns *ErrUnsupportedEvent for a valid envelope carrying an event the
// companion does not handle.
func Parse(body []byte) (*Event, error) {
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("webhook: decoding envelope: %w", err)
	}
	if env.EventName == "" {
		return nil, fmt.Errorf("webhook: envelope has no event_name")
	}

	ev := &Event{Name: env.EventName, Time: env.Time}

	switch env.EventName {
	case EventReminderFired:
		var d struct {
			Task     wireTask    `json:"task"`
			User     wireUser    `json:"user"`
			Project  wireProject `json:"project"`
			Reminder struct {
				Reminder vikunjaTime `json:"reminder"`
			} `json:"reminder"`
		}
		if err := json.Unmarshal(env.Data, &d); err != nil {
			return nil, fmt.Errorf("webhook: decoding %s data: %w", env.EventName, err)
		}
		ev.ReminderFired = &ReminderFiredData{
			Task: d.Task.to(), User: d.User.to(), Project: d.Project.to(),
			Reminder: d.Reminder.Reminder.Time,
		}

	case EventTaskOverdue:
		var d struct {
			Task    wireTask    `json:"task"`
			User    wireUser    `json:"user"`
			Project wireProject `json:"project"`
		}
		if err := json.Unmarshal(env.Data, &d); err != nil {
			return nil, fmt.Errorf("webhook: decoding %s data: %w", env.EventName, err)
		}
		ev.TaskOverdue = &TaskOverdueData{Task: d.Task.to(), User: d.User.to(), Project: d.Project.to()}

	case EventTasksOverdue:
		var d struct {
			Tasks    []wireTask             `json:"tasks"`
			User     wireUser               `json:"user"`
			Projects map[string]wireProject `json:"projects"`
		}
		if err := json.Unmarshal(env.Data, &d); err != nil {
			return nil, fmt.Errorf("webhook: decoding %s data: %w", env.EventName, err)
		}
		out := &TasksOverdueData{User: d.User.to(), Projects: make(map[int64]Project, len(d.Projects))}
		for _, t := range d.Tasks {
			out.Tasks = append(out.Tasks, t.to())
		}
		for _, p := range d.Projects {
			out.Projects[p.ID] = p.to()
		}
		ev.TasksOverdue = out

	default:
		return nil, &ErrUnsupportedEvent{Name: env.EventName}
	}

	return ev, nil
}
