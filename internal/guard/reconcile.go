package guard

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/shichao402/pkv/internal/bw"
	"github.com/shichao402/pkv/internal/bw/types"
	"github.com/shichao402/pkv/internal/securenote"
	"github.com/shichao402/pkv/internal/state"
)

type NoteAction int

const (
	ActionPullRemote NoteAction = iota
	ActionPushLocal
)

type ReconcileDecision struct {
	Action NoteAction
}

type NotePusher interface {
	PushNoteContent(itemID, content string) error
}

type ReconcileInput struct {
	Entry         state.NoteEntry
	Remote        types.Item
	LocalContent  string
	LocalModTime  time.Time
	WorkspaceRoot string
}

type ReconcileOutput struct {
	UpdatedEntry state.NoteEntry
	WrittenPath  string
}

func ParseStateTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func formatStateTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// ReconcileNote applies a manual pull or push for one tracked note.
func ReconcileNote(input ReconcileInput, decision ReconcileDecision, pusher NotePusher) (ReconcileOutput, error) {
	now := time.Now().UTC()
	out := ReconcileOutput{UpdatedEntry: input.Entry}

	remoteRev, err := input.Remote.RevisionTime()
	if err != nil {
		return ReconcileOutput{}, err
	}

	switch decision.Action {
	case ActionPullRemote:
		if err := os.WriteFile(input.Entry.FilePath, []byte(input.Remote.Notes), 0o600); err != nil {
			return ReconcileOutput{}, fmt.Errorf("write remote note: %w", err)
		}
		out.WrittenPath = input.Entry.FilePath
		out.UpdatedEntry = syncedEntry(input.Entry, input.Remote.Notes, remoteRev, remoteRev)

	case ActionPushLocal:
		if pusher == nil {
			return ReconcileOutput{}, fmt.Errorf("push local note: pusher not configured")
		}
		if err := pusher.PushNoteContent(input.Entry.ItemID, input.LocalContent); err != nil {
			return ReconcileOutput{}, fmt.Errorf("push local note: %w", err)
		}
		localTS := input.LocalModTime.UTC()
		if localTS.IsZero() {
			localTS = now
		}
		out.UpdatedEntry = syncedEntry(input.Entry, input.LocalContent, localTS, remoteRev)

	default:
		return ReconcileOutput{}, fmt.Errorf("unsupported reconcile action")
	}

	out.UpdatedEntry.LastSyncedAt = formatStateTime(now)
	out.UpdatedEntry.SyncedAt = out.UpdatedEntry.LastSyncedAt
	return out, nil
}

func syncedEntry(entry state.NoteEntry, content string, localTS, remoteTS time.Time) state.NoteEntry {
	entry.ContentHash = hashContent(content)
	entry.LocalModifiedAt = formatStateTime(localTS)
	entry.RemoteRevisionAt = formatStateTime(remoteTS)
	entry.Conflict = state.ConflictNone
	return entry
}

type BWNotePusher struct {
	Client  *bw.Client
	Session string
}

func (p BWNotePusher) PushNoteContent(itemID, content string) error {
	return securenote.UpdateContent(p.Client, p.Session, itemID, content)
}

func hashContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
