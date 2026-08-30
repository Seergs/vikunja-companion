package vikunja

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestUserSettingsTimezone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/user" || r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("req = %s %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		w.Write([]byte(`{"id":1,"username":"ada","settings":{"timezone":"America/Mexico_City","name":"Ada"}}`))
	}))
	defer srv.Close()

	s, err := NewClient(srv.URL, srv.Client()).UserSettings(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	if s.Timezone != "America/Mexico_City" {
		t.Errorf("timezone = %q", s.Timezone)
	}
}

func TestTasksDueTodayFilterAndPagination(t *testing.T) {
	var gotFilter, gotTZ, gotNulls string
	var pages []int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tasks" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		gotFilter = r.URL.Query().Get("filter")
		gotTZ = r.URL.Query().Get("filter_timezone")
		gotNulls = r.URL.Query().Get("filter_include_nulls")
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		pages = append(pages, page)

		switch page {
		case 1:
			// a full page -> the client must ask for another
			fmt.Fprint(w, "[", full(tasksPerPage), "]")
		case 2:
			w.Write([]byte(`[{"id":501,"title":"last","priority":4,"due_date":"2026-08-29T12:00:00Z"}]`))
		default:
			t.Fatalf("unexpected page %d", page)
		}
	}))
	defer srv.Close()

	tasks, err := NewClient(srv.URL, srv.Client()).TasksDueToday(context.Background(), "tok", "America/Mexico_City")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != tasksPerPage+1 {
		t.Errorf("got %d tasks, want %d", len(tasks), tasksPerPage+1)
	}
	if fmt.Sprint(pages) != "[1 2]" {
		t.Errorf("pages requested = %v", pages)
	}
	if gotTZ != "America/Mexico_City" || gotNulls != "false" {
		t.Errorf("filter_timezone = %q, filter_include_nulls = %q", gotTZ, gotNulls)
	}
	if gotFilter != dueTodayFilter {
		t.Errorf("filter = %q, want %q", gotFilter, dueTodayFilter)
	}
	last := tasks[len(tasks)-1]
	if last.ID != 501 || last.Priority != 4 || last.DueDate.IsZero() {
		t.Errorf("last task = %+v", last)
	}
}

// full renders n minimal task objects (comma-separated, no enclosing brackets).
func full(n int) string {
	s := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			s += ","
		}
		s += fmt.Sprintf(`{"id":%d,"title":"t","priority":0}`, i+1)
	}
	return s
}
