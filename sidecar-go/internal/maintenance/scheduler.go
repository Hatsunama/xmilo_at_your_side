package maintenance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"xmilo/sidecar-go/internal/db"
	"xmilo/sidecar-go/internal/netutil"
	"xmilo/sidecar-go/internal/ws"
)

const (
	githubLatestReleaseURL = "https://api.github.com/repos/Hatsunama/xmilo_at_your_side/releases/latest"
	nightlyCheckInterval   = 15 * time.Second
)

type Scheduler struct {
	store *db.Store
	hub   *ws.Hub
	mu    sync.Mutex
}

type releaseCheck struct {
	Status      string
	TagName     string
	URL         string
	PublishedAt string
}

func Start(ctx context.Context, store *db.Store, hub *ws.Hub) {
	scheduler := &Scheduler{store: store, hub: hub}
	go scheduler.loop(ctx)
}

func (s *Scheduler) loop(ctx context.Context) {
	ticker := time.NewTicker(nightlyCheckInterval)
	defer ticker.Stop()

	s.tick(ctx, time.Now())

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.tick(ctx, now)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	localNow := now.In(time.Local)
	archiveDate := localNow.AddDate(0, 0, -1).Format("2006-01-02")
	completedFor, _ := s.store.GetRuntimeConfig("nightly_maintenance_last_completed_for")
	pendingFor, _ := s.store.GetRuntimeConfig("nightly_maintenance_pending_for")
	activeTask, _ := s.store.GetTask("active")

	if pendingFor != "" && completedFor != pendingFor {
		if activeTask != nil {
			return
		}
		s.runNightly(ctx, pendingFor, localNow, "deferred")
		return
	}

	if localNow.Hour() < 2 || completedFor == archiveDate {
		return
	}

	if activeTask != nil {
		if pendingFor != archiveDate {
			_ = s.store.SetRuntimeConfig("nightly_maintenance_pending_for", archiveDate)
			_ = s.store.SetRuntimeConfig("nightly_maintenance_pending_since", now.UTC().Format(time.RFC3339))
			s.emit("maintenance.nightly_deferred", map[string]any{
				"archive_date": archiveDate,
				"reason":       "active_task",
				"task_id":      activeTask.TaskID,
				"message":      "Nightly upkeep is waiting for Milo to finish the current task.",
			})
		}
		return
	}

	s.runNightly(ctx, archiveDate, localNow, "scheduled")
}

func (s *Scheduler) runNightly(ctx context.Context, archiveDate string, localNow time.Time, trigger string) {
	startedAt := time.Now().UTC()
	taskCount := s.countTaskHistoryForLocalDay(archiveDate)
	update := s.checkLatestRelease(ctx)

	s.emit("maintenance.nightly_started", map[string]any{
		"archive_date":        archiveDate,
		"trigger":             trigger,
		"started_at":          startedAt.Format(time.RFC3339),
		"local_time":          localNow.Format(time.RFC3339),
		"latest_release_tag":  update.TagName,
		"latest_release_url":  update.URL,
		"update_check_status": update.Status,
		"voice_cue":           "Milo is beginning his nightly upkeep.",
		"physical_cue":        "termux_vibrate",
	})
	s.signalCue("Milo is beginning his nightly upkeep.")

	humanArchiveDate := formatHumanDate(archiveDate)
	description := buildArchiveDescription(humanArchiveDate, taskCount, trigger, update)
	recordID := "nightly_archive_" + archiveDate

	_ = s.store.AddTaskHistory(recordID, "Nightly archive", "completed", description)
	s.emit("archive.record_created", map[string]any{
		"task_id":      recordID,
		"title":        "Nightly archive — " + humanArchiveDate,
		"description":  description,
		"created_at":   startedAt.Format(time.RFC3339),
		"archive_date": archiveDate,
		"ritual":       "nightly_maintenance",
	})

	_ = s.store.SetRuntimeConfig("nightly_maintenance_last_completed_for", archiveDate)
	_ = s.store.SetRuntimeConfig("nightly_maintenance_last_completed_at", startedAt.Format(time.RFC3339))
	_ = s.store.SetRuntimeConfig("nightly_maintenance_pending_for", "")
	_ = s.store.SetRuntimeConfig("nightly_maintenance_pending_since", "")

	s.emit("maintenance.nightly_completed", map[string]any{
		"archive_date":        archiveDate,
		"trigger":             trigger,
		"completed_at":        time.Now().UTC().Format(time.RFC3339),
		"task_count":          taskCount,
		"latest_release_tag":  update.TagName,
		"latest_release_url":  update.URL,
		"update_check_status": update.Status,
		"voice_cue":           "Milo has finished his nightly upkeep.",
		"physical_cue":        "termux_vibrate",
		"message":             "Nightly upkeep is complete. Archive sealed and update check finished.",
	})
	s.signalCue("Milo has finished his nightly upkeep.")
}

func (s *Scheduler) countTaskHistoryForLocalDay(archiveDate string) int {
	startLocal, err := time.ParseInLocation("2006-01-02", archiveDate, time.Local)
	if err != nil {
		return 0
	}
	endLocal := startLocal.Add(24 * time.Hour)

	var count int
	_ = s.store.DB.QueryRow(
		`SELECT COUNT(*) FROM task_history WHERE created_at >= ? AND created_at < ?`,
		startLocal.UTC().Format(time.RFC3339),
		endLocal.UTC().Format(time.RFC3339),
	).Scan(&count)
	return count
}

func (s *Scheduler) checkLatestRelease(ctx context.Context) releaseCheck {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubLatestReleaseURL, nil)
	if err != nil {
		return releaseCheck{Status: "request_error"}
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "xmilo-sidecar-nightly-maintenance")

	resp, err := netutil.NewResilientHTTPClient(10 * time.Second).Do(req)
	if err != nil {
		return releaseCheck{Status: "unreachable"}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return releaseCheck{Status: fmt.Sprintf("http_%d", resp.StatusCode)}
	}

	var payload struct {
		TagName     string `json:"tag_name"`
		HTMLURL     string `json:"html_url"`
		PublishedAt string `json:"published_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return releaseCheck{Status: "decode_error"}
	}
	if payload.TagName == "" {
		return releaseCheck{Status: "missing_tag"}
	}
	return releaseCheck{
		Status:      "ok",
		TagName:     payload.TagName,
		URL:         payload.HTMLURL,
		PublishedAt: payload.PublishedAt,
	}
}

func (s *Scheduler) signalCue(phrase string) {
	_, _ = exec.Command("termux-vibrate", "-d", "350").CombinedOutput()
	if strings.TrimSpace(phrase) != "" {
		_, _ = exec.Command("termux-tts-speak", phrase).CombinedOutput()
	}
}

func (s *Scheduler) emit(eventType string, payload map[string]any) {
	_ = s.store.AppendPendingEvent(eventType, payload)
	s.hub.Broadcast(eventType, payload)
}

func formatHumanDate(archiveDate string) string {
	parsed, err := time.ParseInLocation("2006-01-02", archiveDate, time.Local)
	if err != nil {
		return archiveDate
	}
	return parsed.Format("January 2, 2006")
}

func buildArchiveDescription(humanArchiveDate string, taskCount int, trigger string, update releaseCheck) string {
	lines := []string{
		fmt.Sprintf("Nightly archive for %s.", humanArchiveDate),
		fmt.Sprintf("Milo sealed %d completed task record(s) into the daily archive.", taskCount),
	}

	switch trigger {
	case "deferred":
		lines = append(lines, "The ritual waited until the active task finished before beginning.")
	default:
		lines = append(lines, "The ritual began on the normal 2:00 AM local cadence.")
	}

	if update.Status == "ok" {
		lines = append(lines, fmt.Sprintf("Latest app release check found %s.", update.TagName))
		if update.URL != "" {
			lines = append(lines, "Release page: "+update.URL)
		}
	} else {
		lines = append(lines, "App update check did not return a release result this time.")
	}

	return strings.Join(lines, "\n")
}
