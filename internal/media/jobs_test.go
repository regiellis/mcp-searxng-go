package media

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

func newJobManager(t *testing.T) *JobManager {
	t.Helper()
	return NewJobManager(time.Minute, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
}

// waitForStatus polls a job until it reaches want or the deadline passes.
func waitForStatus(t *testing.T, m *JobManager, id, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if v, ok := m.Get(id); ok && v.Status == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach status %q in time", id, want)
}

func TestJobManagerCompletes(t *testing.T) {
	t.Parallel()
	m := newJobManager(t)

	release := make(chan struct{})
	view, err := m.Submit("download_subtitles", func(ctx context.Context) (any, error) {
		<-release
		return "the-result", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.JobID == "" || view.Status != jobRunning {
		t.Fatalf("expected a running job with an id, got %#v", view)
	}
	if view.Message == "" {
		t.Fatal("running job should carry a poll hint message")
	}

	// While blocked, the job is observable as running and not yet finished.
	cur, ok := m.Get(view.JobID)
	if !ok || cur.Status != jobRunning || cur.EndedAt != nil {
		t.Fatalf("unexpected running snapshot: %#v", cur)
	}

	close(release)
	waitForStatus(t, m, view.JobID, jobCompleted)

	done, _ := m.Get(view.JobID)
	if done.Result != "the-result" || done.Error != "" || done.EndedAt == nil {
		t.Fatalf("unexpected completed snapshot: %#v", done)
	}
	if done.Message != "" {
		t.Fatal("finished job should not carry the running poll hint")
	}
}

func TestJobManagerFails(t *testing.T) {
	t.Parallel()
	m := newJobManager(t)

	view, err := m.Submit("transcode_media", func(ctx context.Context) (any, error) {
		return nil, errors.New("ffmpeg blew up")
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, m, view.JobID, jobFailed)

	failed, _ := m.Get(view.JobID)
	if failed.Error != "ffmpeg blew up" || failed.Result != nil {
		t.Fatalf("unexpected failed snapshot: %#v", failed)
	}
}

func TestJobManagerUnknownID(t *testing.T) {
	t.Parallel()
	m := newJobManager(t)
	if _, ok := m.Get("j_does_not_exist"); ok {
		t.Fatal("expected unknown job id to report not found")
	}
}

func TestJobManagerRejectsOverActiveLimit(t *testing.T) {
	t.Parallel()
	m := newJobManager(t)
	m.maxActive = 1

	release := make(chan struct{})
	defer close(release)
	first, err := m.Submit("download_video", func(ctx context.Context) (any, error) {
		<-release
		return "ok", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Submit("download_video", func(ctx context.Context) (any, error) { return nil, nil }); err == nil {
		t.Fatal("expected the second submit to be rejected at the active limit")
	}
	// The first job is still tracked and running.
	if v, ok := m.Get(first.JobID); !ok || v.Status != jobRunning {
		t.Fatalf("first job should still be running: %#v", v)
	}
}

func TestJobIDsAreUnique(t *testing.T) {
	t.Parallel()
	seen := make(map[string]struct{}, 100)
	for range 100 {
		id, err := newJobID()
		if err != nil {
			t.Fatal(err)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate job id %q", id)
		}
		seen[id] = struct{}{}
	}
}
