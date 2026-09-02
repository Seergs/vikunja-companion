// Package digest builds and delivers the morning briefing: one forward-looking
// notification per day ("You have 8 tasks due in Vikunja today"), at a time the
// user picks. It is a pull source — Vikunja emits no "your day" event — so a
// cron in cmd/companion drives it, reading tasks with COMPANION_VIKUNJA_TOKEN
// and calling the same notify.Dispatcher seam internal/webhook uses.
//
// Like internal/webhook, this package may import internal/vikunja; internal/notify
// must not.
package digest

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/seergs/vikunja-companion/internal/notify"
	"github.com/seergs/vikunja-companion/internal/vikunja"
)

// urgentPriority is the Vikunja priority at or above which a task is counted as
// urgent in the briefing (4 = Urgent, 5 = "DO NOW").
const urgentPriority = 4

// Build turns a user's tasks-due-through-today into the single morning
// notification, or nil when there is nothing to report. dedupeKey scopes
// delivery to one per user per day.
func Build(tasks []vikunja.Task, dedupeKey string) []notify.Notification {
	total := len(tasks)
	if total == 0 {
		return nil
	}
	urgent := 0
	for _, t := range tasks {
		if t.Priority >= urgentPriority {
			urgent++
		}
	}

	body := fmt.Sprintf("You have %d %s due in Vikunja today.", total, plural(total, "task", "tasks"))
	if urgent > 0 {
		body += fmt.Sprintf(" %d %s urgent.", urgent, plural(urgent, "is", "are"))
	}

	return []notify.Notification{{
		Title:     "Daily briefing",
		Body:      body,
		Deeplink:  "today",
		DedupeKey: dedupeKey,
	}}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// ParseHHMM parses a 24-hour "HH:MM" string into hour and minute. It is shared
// by the cron and the settings endpoint's input validation.
func ParseHHMM(s string) (hour, minute int, err error) {
	h, m, ok := strings.Cut(s, ":")
	if !ok {
		return 0, 0, fmt.Errorf("digest: time %q is not HH:MM", s)
	}
	hour, err = strconv.Atoi(h)
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("digest: hour out of range in %q", s)
	}
	minute, err = strconv.Atoi(m)
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("digest: minute out of range in %q", s)
	}
	return hour, minute, nil
}
