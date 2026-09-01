package digest

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/seergs/vikunja-companion/internal/notify"
	"github.com/seergs/vikunja-companion/internal/store"
	"github.com/seergs/vikunja-companion/internal/vikunja"
)

const (
	// interval is how often the cron wakes; the per-day dedupe key makes the
	// exact cadence and process restarts irrelevant.
	interval = 5 * time.Minute
	// window is how long after the send time the briefing may still go out — so
	// a companion that was down all morning does not fire a stale briefing in
	// the afternoon.
	window = 2 * time.Hour
)

// tasksClient is the slice of internal/vikunja the runner needs.
type tasksClient interface {
	UserSettings(ctx context.Context, token string) (*vikunja.UserSettings, error)
	TasksDueToday(ctx context.Context, token, tz string) ([]vikunja.Task, error)
}

// dispatcher delivers a user's notifications (internal/notify).
type dispatcher interface {
	Dispatch(ctx context.Context, userID int64, notifications []notify.Notification) error
}

// Runner is the morning-digest cron. It acts for the single user identified by
// COMPANION_VIKUNJA_TOKEN (resolved in cmd/companion) — the digest is
// deliberately not multi-user in v1. Inbound webhook notifications stay
// multi-user; they identify the user by HMAC secret, not a stored token.
type Runner struct {
	store    *store.DB
	vk       tasksClient
	dispatch dispatcher
	token    string
	userID   int64
	now      func() time.Time
	enabled  bool
	log      *slog.Logger
}

// NewRunner wires the digest cron. It is a no-op unless enabled and a non-empty
// token + resolved userID are supplied. now defaults to time.Now when nil.
func NewRunner(st *store.DB, vk tasksClient, d dispatcher, token string, userID int64, now func() time.Time, enabled bool, log *slog.Logger) *Runner {
	if now == nil {
		now = time.Now
	}
	return &Runner{store: st, vk: vk, dispatch: d, token: token, userID: userID, now: now, enabled: enabled, log: log}
}

// Run evaluates the digest once immediately, then every interval, until ctx is
// cancelled. It is meant to run in its own goroutine.
func (r *Runner) Run(ctx context.Context) {
	if !r.enabled || r.token == "" || r.userID == 0 {
		r.log.Info("digest cron disabled")
		return
	}
	r.log.Info("digest cron started", "user", r.userID, "interval", interval.String(), "window", window.String())

	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		r.RunOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// RunOnce evaluates the digest for the configured user. Failures are logged and
// never panic the goroutine.
func (r *Runner) RunOnce(ctx context.Context) {
	if err := r.process(ctx); err != nil {
		r.log.Warn("digest: skipped", "user", r.userID, "err", err)
	}
}

func (r *Runner) process(ctx context.Context) error {
	s, err := r.store.UserSettings(ctx, r.userID)
	if err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	if !s.DigestEnabled {
		r.log.Debug("digest: disabled for user", "user", r.userID)
		return nil
	}

	loc, tz, err := r.location(ctx, s)
	if err != nil {
		return err
	}

	now := r.now().In(loc)
	hour, minute, err := ParseHHMM(s.DigestTime)
	if err != nil {
		return err
	}
	target := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, loc)
	if now.Before(target) || now.After(target.Add(window)) {
		r.log.Debug("digest: outside send window",
			"user", r.userID, "tz", tz, "now", now.Format("15:04"),
			"window", target.Format("15:04")+"–"+target.Add(window).Format("15:04"))
		return nil
	}

	key := fmt.Sprintf("digest:%d:%s", r.userID, now.Format("2006-01-02"))
	switch sent, err := r.store.NotificationSent(ctx, key); {
	case err != nil:
		return err
	case sent:
		r.log.Debug("digest: already sent today", "user", r.userID, "key", key)
		return nil
	}

	tasks, err := r.vk.TasksDueToday(ctx, r.token, tz)
	if err != nil {
		return fmt.Errorf("fetching tasks: %w", err)
	}

	notifs := Build(tasks, key)
	if len(notifs) > 0 {
		if err := r.dispatch.Dispatch(ctx, r.userID, notifs); err != nil {
			return fmt.Errorf("dispatch: %w", err)
		}
		r.log.Info("digest sent", "user", r.userID, "tasks", len(tasks))
	} else {
		r.log.Debug("digest: nothing due today, nothing sent", "user", r.userID)
	}

	// Record that today's digest was evaluated so later ticks in the window
	// skip it. Dispatch has usually marked the key already (it dedupes on
	// DedupeKey); this makes the guard hold even when it did not (nothing to
	// send, or no delivery configured).
	if _, err := r.store.MarkSent(ctx, key); err != nil {
		return err
	}
	return nil
}

// location resolves the user's timezone, fetching and caching it from Vikunja
// on first need and falling back to UTC if the user never set one.
func (r *Runner) location(ctx context.Context, s store.Settings) (*time.Location, string, error) {
	tz := s.Timezone
	if tz == "" {
		us, err := r.vk.UserSettings(ctx, r.token)
		if err != nil {
			return nil, "", fmt.Errorf("fetching timezone: %w", err)
		}
		if tz = us.Timezone; tz != "" {
			if err := r.store.SetUserTimezone(ctx, r.userID, tz); err != nil {
				r.log.Warn("digest: caching timezone", "user", r.userID, "err", err)
			}
		}
	}
	if tz == "" {
		r.log.Warn("digest: user has no timezone, assuming UTC", "user", r.userID)
		tz = "UTC"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, "", fmt.Errorf("loading timezone %q: %w", tz, err)
	}
	return loc, tz, nil
}
