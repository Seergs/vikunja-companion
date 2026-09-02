package digest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/seergs/vikunja-companion/internal/notify"
	"github.com/seergs/vikunja-companion/internal/store"
	"github.com/seergs/vikunja-companion/internal/vikunja"
)

type fakeVK struct {
	tz            string
	tasks         []vikunja.Task
	tasksErr      error
	settingsErr   error
	settingsCalls int
	tasksCalls    int
}

func (f *fakeVK) UserSettings(_ context.Context, _ string) (*vikunja.UserSettings, error) {
	f.settingsCalls++
	if f.settingsErr != nil {
		return nil, f.settingsErr
	}
	return &vikunja.UserSettings{Timezone: f.tz}, nil
}

func (f *fakeVK) TasksDueToday(_ context.Context, _, _ string) ([]vikunja.Task, error) {
	f.tasksCalls++
	if f.tasksErr != nil {
		return nil, f.tasksErr
	}
	return f.tasks, nil
}

type fakeDispatch struct {
	notifs [][]notify.Notification
	users  []int64
}

func (f *fakeDispatch) Dispatch(_ context.Context, userID int64, n []notify.Notification) error {
	f.notifs = append(f.notifs, n)
	f.users = append(f.users, userID)
	return nil
}

type runnerEnv struct {
	runner   *Runner
	store    *store.DB
	vk       *fakeVK
	dispatch *fakeDispatch
	now      time.Time
}

func newRunnerEnv(t *testing.T, vk *fakeVK) *runnerEnv {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	env := &runnerEnv{
		store:    db,
		vk:       vk,
		dispatch: &fakeDispatch{},
		now:      time.Date(2026, 8, 29, 8, 30, 0, 0, time.UTC),
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	env.runner = NewRunner(db, vk, env.dispatch, "tok-1", 1, func() time.Time { return env.now }, true, log)
	return env
}

func TestRunnerSendsOncePerDay(t *testing.T) {
	env := newRunnerEnv(t, &fakeVK{tasks: []vikunja.Task{{Priority: 1}, {Priority: 4}}})
	ctx := context.Background()
	if err := env.store.SetUserTimezone(ctx, 1, "UTC"); err != nil {
		t.Fatal(err)
	}

	env.runner.RunOnce(ctx)
	env.runner.RunOnce(ctx) // same wall clock -> already sent

	if len(env.dispatch.notifs) != 1 {
		t.Fatalf("dispatched %d times, want 1", len(env.dispatch.notifs))
	}
	if body := env.dispatch.notifs[0][0].Body; body != "You have 2 tasks due in Vikunja today. 1 is urgent." {
		t.Errorf("body = %q", body)
	}
	if env.vk.tasksCalls != 1 {
		t.Errorf("tasks fetched %d times, want 1", env.vk.tasksCalls)
	}
}

func TestRunnerRespectsWindow(t *testing.T) {
	ctx := context.Background()

	before := newRunnerEnv(t, &fakeVK{tasks: []vikunja.Task{{Priority: 1}}})
	before.store.SetUserTimezone(ctx, 1, "UTC")
	before.now = time.Date(2026, 8, 29, 7, 59, 0, 0, time.UTC)
	before.runner.RunOnce(ctx)

	after := newRunnerEnv(t, &fakeVK{tasks: []vikunja.Task{{Priority: 1}}})
	after.store.SetUserTimezone(ctx, 1, "UTC")
	after.now = time.Date(2026, 8, 29, 10, 30, 0, 0, time.UTC) // >2h past 08:00
	after.runner.RunOnce(ctx)

	if len(before.dispatch.notifs) != 0 || len(after.dispatch.notifs) != 0 {
		t.Fatalf("fired outside the window: before=%d after=%d", len(before.dispatch.notifs), len(after.dispatch.notifs))
	}
}

func TestRunnerTimezoneOffsetsTheWindow(t *testing.T) {
	// 08:30 UTC is 02:30 in Mexico City — well before an 08:00 local send.
	env := newRunnerEnv(t, &fakeVK{tz: "America/Mexico_City", tasks: []vikunja.Task{{Priority: 1}}})
	ctx := context.Background()

	env.runner.RunOnce(ctx)
	if len(env.dispatch.notifs) != 0 {
		t.Fatalf("fired before the user's local 08:00")
	}
	// timezone got resolved from Vikunja and cached
	if env.vk.settingsCalls != 1 {
		t.Errorf("UserSettings called %d times", env.vk.settingsCalls)
	}
	s, _ := env.store.UserSettings(ctx, 1)
	if s.Timezone != "America/Mexico_City" {
		t.Errorf("timezone not cached: %q", s.Timezone)
	}

	// Advance to 14:30 UTC == 08:30 local -> now in the window.
	env.now = time.Date(2026, 8, 29, 14, 30, 0, 0, time.UTC)
	env.runner.RunOnce(ctx)
	if len(env.dispatch.notifs) != 1 {
		t.Fatalf("did not fire at local 08:30")
	}
	if env.vk.settingsCalls != 1 {
		t.Errorf("UserSettings should not be called again once cached, got %d", env.vk.settingsCalls)
	}
}

func TestRunnerZeroTasksSendsNothingButRecords(t *testing.T) {
	env := newRunnerEnv(t, &fakeVK{tasks: nil})
	ctx := context.Background()
	env.store.SetUserTimezone(ctx, 1, "UTC")

	env.runner.RunOnce(ctx)
	env.runner.RunOnce(ctx)

	if len(env.dispatch.notifs) != 0 {
		t.Fatalf("dispatched with no tasks")
	}
	if env.vk.tasksCalls != 1 {
		t.Errorf("re-fetched tasks after recording an empty digest: %d calls", env.vk.tasksCalls)
	}
}

func TestRunnerSkipsDisabled(t *testing.T) {
	env := newRunnerEnv(t, &fakeVK{tasks: []vikunja.Task{{Priority: 1}}})
	ctx := context.Background()
	env.store.SetUserTimezone(ctx, 1, "UTC")
	if err := env.store.PutDigestSettings(ctx, 1, false, "08:00"); err != nil {
		t.Fatal(err)
	}

	env.runner.RunOnce(ctx)
	if len(env.dispatch.notifs) != 0 || env.vk.tasksCalls != 0 {
		t.Fatalf("acted on a disabled user")
	}
}

func TestRunnerTaskFetchErrorIsRetriable(t *testing.T) {
	env := newRunnerEnv(t, &fakeVK{tasksErr: errors.New("vikunja down")})
	ctx := context.Background()
	env.store.SetUserTimezone(ctx, 1, "UTC")

	env.runner.RunOnce(ctx)
	env.vk.tasksErr = nil
	env.vk.tasks = []vikunja.Task{{Priority: 1}}
	env.runner.RunOnce(ctx)

	if len(env.dispatch.notifs) != 1 {
		t.Fatalf("a failed fetch consumed the day's slot; dispatched %d", len(env.dispatch.notifs))
	}
}
