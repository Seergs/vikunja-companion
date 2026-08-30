package vikunja

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// UserSettings is the subset of GET /api/v1/user's settings block the digest
// cron needs: the timezone that "08:00" is interpreted in.
type UserSettings struct {
	Timezone string // IANA name, e.g. "America/Mexico_City"; "" if the user never set one
}

// UserSettings fetches GET /api/v1/user and returns the caller's settings.
func (c *Client) UserSettings(ctx context.Context, token string) (*UserSettings, error) {
	var resp struct {
		Settings struct {
			Timezone string `json:"timezone"`
		} `json:"settings"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/user", token, &resp); err != nil {
		return nil, err
	}
	return &UserSettings{Timezone: resp.Settings.Timezone}, nil
}

// Task is the subset of a Vikunja task the digest needs to count and classify.
type Task struct {
	ID       int64
	Title    string
	Priority int64 // 0 unset .. 4 urgent, 5 "DO NOW"
	DueDate  time.Time
}

// tasksPerPage is the request page size; Vikunja's max_items_per_page is 50.
const tasksPerPage = 50

// tasksMaxPages caps a runaway fetch; 50 * 10 = 500 undone tasks due today is
// already far past any real personal backlog.
const tasksMaxPages = 10

// dueTodayFilter selects undone tasks that have a due date, due at or before
// the end of the caller's local day. "now+1d/d" is start-of-tomorrow (the
// documented "now-1M/M" idiom); "> 0001-01-01" drops the zero-date rows that a
// null due date can show up as. Dates are unquoted date-math — Vikunja rejects
// quoted RFC3339 here.
const dueTodayFilter = "done = false && due_date > 0001-01-01 && due_date < now+1d/d"

// TasksDueToday returns the caller's undone tasks due through the end of today
// in tz (the IANA name Vikunja resolves "now" against).
func (c *Client) TasksDueToday(ctx context.Context, token, tz string) ([]Task, error) {
	var out []Task
	for page := 1; page <= tasksMaxPages; page++ {
		q := url.Values{}
		q.Set("filter", dueTodayFilter)
		q.Set("filter_timezone", tz)
		q.Set("filter_include_nulls", "false")
		q.Set("per_page", strconv.Itoa(tasksPerPage))
		q.Set("page", strconv.Itoa(page))

		var batch []wireDigestTask
		// GET /api/v1/tasks (not .../tasks/all — that route was dropped in
		// Vikunja v2.x and now 400s with code 2004).
		if err := c.do(ctx, http.MethodGet, "/api/v1/tasks?"+q.Encode(), token, &batch); err != nil {
			return nil, err
		}
		for _, t := range batch {
			out = append(out, t.to())
		}
		if len(batch) < tasksPerPage {
			break
		}
	}
	return out, nil
}

type wireDigestTask struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Priority int64  `json:"priority"`
	DueDate  string `json:"due_date"`
}

func (w wireDigestTask) to() Task {
	t := Task{ID: w.ID, Title: w.Title, Priority: w.Priority}
	if w.DueDate != "" {
		if parsed, err := time.Parse(time.RFC3339, w.DueDate); err == nil && parsed.Year() > 1 {
			t.DueDate = parsed
		}
	}
	return t
}
