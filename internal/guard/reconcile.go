package guard

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
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

func hasPendingLocalEdit(entry state.NoteEntry, localContent string) bool {
	return hashContent(localContent) != entry.ContentHash
}

// DecideAction compares local and remote content hashes against last-synced state.
// Conflict only when both sides differ from state and from each other.
// Pending local edits (file hash != state.ContentHash) never trigger pull; only
// push or noop. Stale remote snapshots older than RemoteRevisionAt re-push local
// instead of overwriting.
func DecideAction(entry state.NoteEntry, remote types.Item, localContent string, localMod time.Time) (ReconcileDecision, error) {
	stateHash := entry.ContentHash
	remoteHash := hashContent(remote.Notes)
	localHash := hashContent(localContent)

	remoteDirty := remoteHash != stateHash
	localDirty := localHash != stateHash

	if localDirty {
		switch {
		case !remoteDirty:
			return ReconcileDecision{Action: ActionPushLocal}, nil
		case remoteHash == localHash:
			return ReconcileDecision{Action: ActionNoop}, nil
		default:
			if shouldPushLocalForRapidEdit(entry, remote, localContent) {
				return ReconcileDecision{Action: ActionPushLocal}, nil
			}
			return ReconcileDecision{Action: ActionConflictSameSecond}, nil
		}
	}

	switch {
	case !remoteDirty && !localDirty:
		return ReconcileDecision{Action: ActionNoop}, nil
	case remoteDirty && !localDirty:
		if allow, err := allowPullRemote(entry, remote); err != nil {
			return ReconcileDecision{}, err
		} else if !allow {
			return ReconcileDecision{Action: ActionPushLocal}, nil
		}
		return ReconcileDecision{Action: ActionPullRemote}, nil
	case remoteHash == localHash:
		return ReconcileDecision{Action: ActionNoop}, nil
	default:
		return ReconcileDecision{Action: ActionConflictSameSecond}, nil
	}
}

// guardPollPendingEdit skips reconcile actions during poll when the local file has
// unsaved edits relative to state. Debounced sync will push the final content.
func guardPollPendingEdit(pollCycle bool, entry state.NoteEntry, localContent string, decision ReconcileDecision) ReconcileDecision {
	if !pollCycle || !hasPendingLocalEdit(entry, localContent) {
		return decision
	}
	switch decision.Action {
	case ActionPullRemote, ActionPushLocal, ActionConflictRemoteWins:
		return ReconcileDecision{Action: ActionNoop}
	default:
		return decision
	}
}

// shouldPushLocalForRapidEdit detects incremental local typing where Bitwarden still
// exposes an older prefix (often from a just-finished push) while the local file has
// moved ahead. This avoids archiving false conflict copies during rapid edits.
func shouldPushLocalForRapidEdit(entry state.NoteEntry, remote types.Item, localContent string) bool {
	if hashContent(localContent) == entry.ContentHash {
		return false
	}
	if hashContent(remote.Notes) == entry.ContentHash {
		return false
	}
	if localContent == "" || remote.Notes == "" {
		return false
	}
	return strings.HasPrefix(localContent, remote.Notes)
}

// allowPullRemote gates pull so a stale vault snapshot cannot overwrite local content
// that already matches our last-synced state (e.g. after a push, before BW catches up).
func allowPullRemote(entry state.NoteEntry, remote types.Item) (bool, error) {
	storedRev, err := ParseStateTime(entry.RemoteRevisionAt)
	if err != nil {
		return false, fmt.Errorf("parse remote_revision_at: %w", err)
	}
	remoteRev, err := remote.RevisionTime()
	if err != nil {
		return false, fmt.Errorf("parse remote revisionDate: %w", err)
	}
	if !storedRev.IsZero() && !remoteRev.IsZero() && remoteRev.Before(storedRev) {
		return false, nil
	}
	return true, nil
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
