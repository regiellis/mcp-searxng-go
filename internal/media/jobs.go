package media

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/regiellis/mcp-searxng-go/pkg/types"
)

// Background media job statuses.
const (
	jobRunning   = "running"
	jobCompleted = "completed"
	jobFailed    = "failed"
)

const (
	defaultMaxJobs   = 100 // retained job snapshots before oldest finished ones are evicted
	defaultMaxActive = 8   // concurrently running background jobs
)

// jobState is the internal, mutable record for one background job. All access is
// guarded by JobManager.mu.
type jobState struct {
	id        string
	kind      string
	status    string
	startedAt time.Time
	endedAt   time.Time
	result    any
	err       string
}

// JobManager runs long media operations in the background and exposes their
// status by ID so a caller can submit work, return, and poll for the result
// instead of blocking on a slow yt-dlp or ffmpeg invocation.
type JobManager struct {
	mu        sync.Mutex
	jobs      map[string]*jobState
	order     []string // job IDs in insertion order, for eviction
	maxJobs   int
	maxActive int
	active    int
	timeout   time.Duration
	logger    *slog.Logger
}

// NewJobManager returns a JobManager. timeout bounds each background job; it
// should match the media timeout so async work has the same ceiling as the
// synchronous path.
func NewJobManager(timeout time.Duration, logger *slog.Logger) *JobManager {
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	return &JobManager{
		jobs:      make(map[string]*jobState),
		maxJobs:   defaultMaxJobs,
		maxActive: defaultMaxActive,
		timeout:   timeout,
		logger:    logger,
	}
}

// Submit starts fn in a background goroutine and returns the new job's running
// view immediately. fn receives a fresh background context bounded by the
// manager timeout, independent of the request that submitted it, so the work
// survives the tool call returning.
func (m *JobManager) Submit(kind string, fn func(ctx context.Context) (any, error)) (types.MediaJobView, error) {
	id, err := newJobID()
	if err != nil {
		return types.MediaJobView{}, err
	}

	m.mu.Lock()
	if m.active >= m.maxActive {
		m.mu.Unlock()
		return types.MediaJobView{}, fmt.Errorf("too many media jobs running (limit %d); retry shortly", m.maxActive)
	}
	js := &jobState{id: id, kind: kind, status: jobRunning, startedAt: time.Now()}
	m.jobs[id] = js
	m.order = append(m.order, id)
	m.active++
	m.evictLocked()
	view := viewOf(js)
	m.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
		defer cancel()
		result, runErr := fn(ctx)

		m.mu.Lock()
		js.endedAt = time.Now()
		if runErr != nil {
			js.status = jobFailed
			js.err = runErr.Error()
		} else {
			js.status = jobCompleted
			js.result = result
		}
		m.active--
		m.mu.Unlock()

		if m.logger != nil {
			m.logger.Info("media job finished", "id", id, "kind", kind, "status", js.status)
		}
	}()

	return view, nil
}

// Get returns a snapshot of a job by ID.
func (m *JobManager) Get(id string) (types.MediaJobView, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	js, ok := m.jobs[id]
	if !ok {
		return types.MediaJobView{}, false
	}
	return viewOf(js), true
}

// evictLocked drops the oldest finished jobs once the retained count exceeds
// maxJobs. Running jobs are never evicted. Caller must hold m.mu.
func (m *JobManager) evictLocked() {
	for len(m.jobs) > m.maxJobs {
		removed := false
		for i, id := range m.order {
			js, ok := m.jobs[id]
			if !ok {
				m.order = append(m.order[:i], m.order[i+1:]...)
				removed = true
				break
			}
			if js.status != jobRunning {
				delete(m.jobs, id)
				m.order = append(m.order[:i], m.order[i+1:]...)
				removed = true
				break
			}
		}
		if !removed {
			return // everything remaining is still running
		}
	}
}

// viewOf builds an external snapshot. Caller must hold m.mu.
func viewOf(js *jobState) types.MediaJobView {
	v := types.MediaJobView{
		JobID:     js.id,
		Kind:      js.kind,
		Status:    js.status,
		StartedAt: js.startedAt,
		Result:    js.result,
		Error:     js.err,
	}
	if !js.endedAt.IsZero() {
		ended := js.endedAt
		v.EndedAt = &ended
	}
	if js.status == jobRunning {
		v.Message = "Running in the background. Call media_job_status with this job_id to check status and retrieve the result."
	}
	return v
}

// newJobID returns an unguessable short job identifier.
func newJobID() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate job id: %w", err)
	}
	return "j_" + hex.EncodeToString(b), nil
}
