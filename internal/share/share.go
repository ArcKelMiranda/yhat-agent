// Package share implements the F2 share-and-sync surfaces for Cerebro YHat v1.2.
// It enqueues validated memories, drains the upload queue, and syncs with the
// central repository via a Transport abstraction.
//
// Wire contract: see docs/central-api.md.
package share

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ArcKelMiranda/yhat-agent/internal/store"
)

// Report summarises the outcome of a sync run.
type Report struct {
	Sent    int `json:"sent"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

// Queue enqueues a validated memory for sharing.
//
// It checks that the memory:
//   - exists and is in 'validated' status (BR9: share_status only with validated/archived)
//   - has share_status 'none' (not already queued/sent/rejected)
//
// On success, it inserts an entry in upload_queue and flips share_status to 'queued'.
//
// Returns ErrNotShareable if the memory is not in a shareable state.
// Returns store.ErrNotFound if the memory ID is unknown.
func Queue(ctx context.Context, s *store.Store, memoryID, orderedBy string) error {
	mem, err := s.GetMemory(ctx, memoryID)
	if err != nil {
		return err // ErrNotFound propagates
	}

	// Must be validated.
	if mem.Status != store.StatusValidated {
		return fmt.Errorf("%w: status=%q (must be 'validated')", ErrNotShareable, mem.Status)
	}

	// Must not already be shared.
	if mem.ShareStatus != store.ShareStatusNone {
		return fmt.Errorf("%w: share_status=%q (must be 'none')", ErrNotShareable, mem.ShareStatus)
	}

	// Enqueue.
	if _, err := s.EnqueueUpload(ctx, memoryID, orderedBy); err != nil {
		return fmt.Errorf("EnqueueUpload: %w", err)
	}

	// Flip to queued.
	if err := s.SetShareStatus(ctx, memoryID, store.ShareStatusQueued); err != nil {
		return fmt.Errorf("SetShareStatus(queued): %w", err)
	}

	return nil
}

// Sync drains the upload queue and sends each entry through the transport.
//
// It processes entries in FIFO order (ordered_at ASC). For each entry:
//
//   - If transport.Send succeeds: calls store.MarkUploadSent, which flips
//     share_status='sent' and removes the queue entry. (Report.Sent++)
//   - If transport.Send fails: calls store.RecordUploadFailure, which increments
//     attempts and records last_error. (Report.Failed++)
//   - Entries are skipped if they have share_status not 'queued' — this guards
//     against race conditions where another process already sent them. (Report.Skipped++)
//
// Sync returns a Report with the final counters and any error that should
// cause the caller to retry. Partial successes (some sent, some failed) are
// returned with a nil error; only catastrophic errors (transport nil, etc.)
// return non-nil.
func Sync(ctx context.Context, s *store.Store, tp Transport) (Report, error) {
	if tp == nil {
		return Report{}, fmt.Errorf("transport is nil")
	}

	entries, err := s.ListUploadQueue(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("ListUploadQueue: %w", err)
	}

	var report Report
	for _, entry := range entries {
		// Re-check: only process entries with share_status='queued'.
		mem, err := s.GetMemory(ctx, entry.MemoryID)
		if err != nil {
			// Memory gone: skip and continue.
			report.Skipped++
			continue
		}
		if mem.ShareStatus != store.ShareStatusQueued {
			report.Skipped++
			continue
		}

		// Build the payload.
		payload := LocalPayload{
			ID:        mem.ID,
			Origin:    mem.Origin,
			Type:      mem.Type,
			Title:     mem.Title,
			Content:   mem.Content,
			Author:    mem.Author,
			CreatedAt: mem.CreatedAt.Format(time.RFC3339),
		}
		if mem.Context != nil {
			payload.Context = *mem.Context
		}

		// Send.
		sendErr := tp.Send(ctx, payload)
		if sendErr != nil {
			// Record the failure in the store.
			if recErr := s.RecordUploadFailure(ctx, entry.MemoryID, sendErr.Error()); recErr != nil {
				// Log but continue: record the send error as the primary failure.
				_ = recErr
			}
			report.Failed++
			continue
		}

		// Success: mark as sent and remove from queue.
		if err := s.MarkUploadSent(ctx, entry.MemoryID); err != nil {
			// MarkUploadSent already recorded the failure; treat as failed.
			report.Failed++
			continue
		}
		report.Sent++
	}

	return report, nil
}

// PullAndInsert pulls team-approved memories from the transport and inserts
// them as origin='team' records with team_status='approved'.
// It persists the pull cursor in sync_state.
func PullAndInsert(ctx context.Context, s *store.Store, tp Transport) (int, error) {
	cursor := ""
	cursorVal, err := s.GetSyncState(ctx, "pull_cursor")
	if err == nil {
		cursor = cursorVal
	}

	result, err := tp.Pull(ctx, cursor)
	if err != nil {
		return 0, fmt.Errorf("Pull: %w", err)
	}

	inserted := 0
	for _, tp := range result.Memories {
		mem := store.Memory{
			RemoteID:   &tp.ID,
			Origin:     store.OriginTeam,
			Type:       tp.Type,
			Title:      tp.Title,
			Content:    tp.Content,
			Author:     tp.Author,
			TeamStatus: &tp.TeamStatus,
		}
		if tp.Context != "" {
			mem.Context = &tp.Context
		}
		_, insertErr := s.InsertTeamMemory(ctx, mem)
		if insertErr != nil {
			// Duplicate remote_id is expected when re-syncing; skip.
			if !strings.Contains(insertErr.Error(), "UNIQUE constraint") {
				return inserted, fmt.Errorf("InsertTeamMemory %s: %w", tp.ID, insertErr)
			}
		} else {
			inserted++
		}
	}

	// Persist next cursor.
	if result.NextCursor != "" {
		if err := s.SetSyncState(ctx, "pull_cursor", result.NextCursor); err != nil {
			return inserted, fmt.Errorf("SetSyncState pull_cursor: %w", err)
		}
	}

	return inserted, nil
}
