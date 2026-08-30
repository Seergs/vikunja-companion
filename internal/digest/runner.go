package digest

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/seergs/vikunja-companion/internal/crypto"
	"github.com/seergs/vikunja-companion/internal/notify"
	"github.com/seergs/vikunja-companion/internal/store"
	"github.com/seergs/vikunja-companion/internal/vikunja"
)

const (
	// interval is how often the cron wakes; the per-user dedupe key makes the
	// exact cadence and process restarts irrelevant.
	interval = 5 * time.Minute
	// window is how long after a user's send time the briefing may still go
	// out — so a companion that was down all morning does not fire a stale
	// briefing in the afternoon.
	window = 2 * time.Hour
)

// tasksClient is the slice of internal/vikunja the runner needs.
type tasksClient interface {
	UserSettings(ctx context.Context, token string) (*vikunja.UserSettings, error)
	TasksDueToday(ctx context.Context, token, tz string) ([]vikunja.Task, error)
}

// dispatcher delivers notifications for a user's devices (internal/notify).
type dispatcher interface {
	Dispatch(ctx context.Context, devices []notify.Device, notifications []notify.Notification) error
}

// Runner is the morning-digest cron.
type Runner struct {
	store    *store.DB
	vk       tasksClient
	cipher   *crypto.Cipher
	dispatch dispatcher
	now      func() time.Time
	enabled  bool
	log      *slog.Logger
}

// NewRunner wires the digest cron. now defaults to time.Now when nil.
func NewRunner(st *store.DB, vk tasksClient, cipher *crypto.Cipher, d dispatcher, now func() time.Time, enabled bool, log *slog.Logger) *Runner {
	if now == nil {
		now = time.Now
	}
	return &Runner{store: st, vk: vk, cipher: cipher, dispatch: d, now: now, enabled: enabled, log: log}
}

// Run evaluates every user once immediately, then every interval, until ctx is
// cancelled. It is meant to run in its own goroutine.
func (r *Runner) Run(ctx context.Context) {
	if !r.enabled {
		r.log.Info("digest cron disabled")
		return
	}
	r.log.Info("digest cron started", "interval", interval.String(), "window", window.String())

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

// RunOnce evaluates the digest for every eligible user. Per-user failures are
// logged and never stop the sweep.
func (r *Runner) RunOnce(ctx context.Context) {
	targets, err := r.store.ListDigestTargets(ctx)
	if err != nil {
		r.log.Error("digest: listing targets", "err", err)
		return
	}
	for _, tg := range targets {
		if err := r.process(ctx, tg); err != nil {
			r.log.Warn("digest: skipping user", "user", tg.UserID, "err", err)
		}
	}
}

func (r *Runner) process(ctx context.Context, tg store.DigestTarget) error {
	if !tg.Enabled {
		return nil
	}

	tokenEnc, err := r.store.UserToken(ctx, tg.UserID)
	if err != nil {
		return fmt.Errorf("token: %w", err)
	}
	tokenB, err := r.cipher.Decrypt(tokenEnc)
	if err != nil {
		return fmt.Errorf("decrypt token: %w", err)
	}
	token := string(tokenB)

	loc, tz, err := r.location(ctx, tg, token)
	if err != nil {
		return err
	}

	now := r.now().In(loc)
	hour, minute, err := ParseHHMM(tg.Time)
	if err != nil {
		return err
	}
	target := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, loc)
	if now.Before(target) || now.After(target.Add(window)) {
		return nil
	}

	key := fmt.Sprintf("digest:%d:%s", tg.UserID, now.Format("2006-01-02"))
	switch sent, err := r.store.NotificationSent(ctx, key); {
	case err != nil:
		return err
	case sent:
		return nil
	}

	tasks, err := r.vk.TasksDueToday(ctx, token, tz)
	if err != nil {
		return fmt.Errorf("fetching tasks: %w", err)
	}

	notifs := Build(tasks, key)
	if len(notifs) > 0 {
		devices, err := r.devices(ctx, tg.UserID)
		if err != nil {
			return err
		}
		if err := r.dispatch.Dispatch(ctx, devices, notifs); err != nil {
			return fmt.Errorf("dispatch: %w", err)
		}
		r.log.Info("digest sent", "user", tg.UserID, "tasks", len(tasks))
	}

	// Record that today's digest was evaluated so later ticks in the window
	// skip this user. Dispatch has usually marked the key already (it dedupes
	// on DedupeKey); this makes the guard hold even when it did not (no
	// devices, or nothing to send).
	if _, err := r.store.MarkSent(ctx, key); err != nil {
		return err
	}
	return nil
}

// location resolves the user's timezone, fetching and caching it from Vikunja
// on first need and falling back to UTC if the user never set one.
func (r *Runner) location(ctx context.Context, tg store.DigestTarget, token string) (*time.Location, string, error) {
	tz := tg.Timezone
	if tz == "" {
		s, err := r.vk.UserSettings(ctx, token)
		if err != nil {
			return nil, "", fmt.Errorf("fetching timezone: %w", err)
		}
		if tz = s.Timezone; tz != "" {
			if err := r.store.SetUserTimezone(ctx, tg.UserID, tz); err != nil {
				r.log.Warn("digest: caching timezone", "user", tg.UserID, "err", err)
			}
		}
	}
	if tz == "" {
		r.log.Warn("digest: user has no timezone, assuming UTC", "user", tg.UserID)
		tz = "UTC"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, "", fmt.Errorf("loading timezone %q: %w", tz, err)
	}
	return loc, tz, nil
}

func (r *Runner) devices(ctx context.Context, userID int64) ([]notify.Device, error) {
	rows, err := r.store.DevicesForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]notify.Device, 0, len(rows))
	for _, d := range rows {
		out = append(out, notify.Device{APNsToken: d.APNsToken, PublicKey: d.PublicKey})
	}
	return out, nil
}
