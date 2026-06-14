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
	ActionNoop NoteAction = iota
	ActionPullRemote
	ActionPushLocal
	ActionConflictSameSecond
	ActionConflictRemoteWins
	ActionConflictLocalWins
)

type ReconcileDecision struct {
	Action NoteAction
}

type NotePusher interface {
	PushNoteContent(itemID, content string) error
}

type ReconcileInput struct {
	Entry        state.NoteEntry
	Remote       types.Item
	LocalContent string
	LocalModTime time.Time
	WorkspaceRoot string
}

type ReconcileOutput struct {
	UpdatedEntry state.NoteEntry
	WrittenPath  string
	ConflictPaths []string
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

func hashContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func sameSecond(a, b time.Time) bool {
	if a.IsZero() || b.IsZero() {
		return false
	}
	au, bu := a.UTC(), b.UTC()
	return au.Truncate(time.Second).Equal(bu.Truncate(time.Second))
}

// DecideAction compares remote revision and local modification against last sync.
func DecideAction(entry state.NoteEntry, remote types.Item, localContent string, localMod time.Time) (ReconcileDecision, error) {
	lastSync, err := ParseStateTime(entry.LastSyncedAt)
	if err != nil {
		return ReconcileDecision{}, fmt.Errorf("parse last_synced_at: %w", err)
	}
	if lastSync.IsZero() {
		lastSync, err = ParseStateTime(entry.SyncedAt)
		if err != nil {
			return ReconcileDecision{}, fmt.Errorf("parse synced_at: %w", err)
		}
	}

	remoteRev, err := remote.RevisionTime()
	if err != nil {
		return ReconcileDecision{}, fmt.Errorf("parse remote revisionDate: %w", err)
	}
	localTS := localMod.UTC()
	if localTS.IsZero() {
		localTS, err = ParseStateTime(entry.LocalModifiedAt)
		if err != nil {
			return ReconcileDecision{}, fmt.Errorf("parse local_modified_at: %w", err)
		}
	}

	// Remote is dirty only when vault content differs from what we last synced.
	// Do not use remoteRev.After(lastSync): our own push advances revisionDate while
	// content already matches entry.ContentHash, which would falsely pair with a new
	// local edit and trigger conflict handling instead of ActionPushLocal.
	remoteDirty := hashContent(remote.Notes) != entry.ContentHash
	localDirty := localTS.After(lastSync) || hashContent(localContent) != entry.ContentHash

	switch {
	case !remoteDirty && !localDirty:
		return ReconcileDecision{Action: ActionNoop}, nil
	case remoteDirty && !localDirty:
		return ReconcileDecision{Action: ActionPullRemote}, nil
	case !remoteDirty && localDirty:
		return ReconcileDecision{Action: ActionPushLocal}, nil
	}

	if sameSecond(remoteRev, localTS) {
		return ReconcileDecision{Action: ActionConflictSameSecond}, nil
	}
	if remoteRev.Before(localTS) {
		return ReconcileDecision{Action: ActionConflictLocalWins}, nil
	}
	return ReconcileDecision{Action: ActionConflictRemoteWins}, nil
}

// ReconcileNote applies a reconcile decision for one tracked note.
func ReconcileNote(input ReconcileInput, decision ReconcileDecision, pusher NotePusher) (ReconcileOutput, error) {
	now := time.Now().UTC()
	out := ReconcileOutput{UpdatedEntry: input.Entry}

	remoteRev, err := input.Remote.RevisionTime()
	if err != nil {
		return ReconcileOutput{}, err
	}

	switch decision.Action {
	case ActionNoop:
		return out, nil

	case ActionPullRemote:
		if err := os.WriteFile(input.Entry.FilePath, []byte(input.Remote.Notes), 0o600); err != nil {
			return ReconcileOutput{}, fmt.Errorf("write remote note: %w", err)
		}
		out.WrittenPath = input.Entry.FilePath
		out.UpdatedEntry = syncedEntry(input.Entry, input.Remote.Notes, remoteRev, remoteRev, state.ConflictNone)

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
		out.UpdatedEntry = syncedEntry(input.Entry, input.LocalContent, localTS, remoteRev, state.ConflictNone)

	case ActionConflictSameSecond:
		localPath, err := WriteConflictCopy(input.WorkspaceRoot, input.Entry.ItemID, "local", input.Entry.FileName, []byte(input.LocalContent))
		if err != nil {
			return ReconcileOutput{}, err
		}
		remotePath, err := WriteConflictCopy(input.WorkspaceRoot, input.Entry.ItemID, "remote", input.Entry.FileName, []byte(input.Remote.Notes))
		if err != nil {
			return ReconcileOutput{}, err
		}
		out.ConflictPaths = []string{localPath, remotePath}
		out.UpdatedEntry = input.Entry
		out.UpdatedEntry.Conflict = state.ConflictPending

	case ActionConflictLocalWins:
		remotePath, err := WriteConflictCopy(input.WorkspaceRoot, input.Entry.ItemID, "remote", input.Entry.FileName, []byte(input.Remote.Notes))
		if err != nil {
			return ReconcileOutput{}, err
		}
		out.ConflictPaths = []string{remotePath}
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
		out.UpdatedEntry = syncedEntry(input.Entry, input.LocalContent, localTS, remoteRev, state.ConflictNone)

	case ActionConflictRemoteWins:
		localPath, err := WriteConflictCopy(input.WorkspaceRoot, input.Entry.ItemID, "local", input.Entry.FileName, []byte(input.LocalContent))
		if err != nil {
			return ReconcileOutput{}, err
		}
		out.ConflictPaths = []string{localPath}
		if err := os.WriteFile(input.Entry.FilePath, []byte(input.Remote.Notes), 0o600); err != nil {
			return ReconcileOutput{}, fmt.Errorf("write remote note: %w", err)
		}
		out.WrittenPath = input.Entry.FilePath
		out.UpdatedEntry = syncedEntry(input.Entry, input.Remote.Notes, remoteRev, remoteRev, state.ConflictNone)
	}

	out.UpdatedEntry.LastSyncedAt = formatStateTime(now)
	out.UpdatedEntry.SyncedAt = out.UpdatedEntry.LastSyncedAt
	return out, nil
}

func syncedEntry(entry state.NoteEntry, content string, localTS, remoteTS time.Time, conflict string) state.NoteEntry {
	entry.ContentHash = hashContent(content)
	entry.LocalModifiedAt = formatStateTime(localTS)
	entry.RemoteRevisionAt = formatStateTime(remoteTS)
	entry.Conflict = conflict
	return entry
}

type BWNotePusher struct {
	Client  *bw.Client
	Session string
}

func (p BWNotePusher) PushNoteContent(itemID, content string) error {
	return securenote.UpdateContent(p.Client, p.Session, itemID, content)
}
