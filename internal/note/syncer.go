package note

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/shichao402/pkv/internal/bw/types"
	"github.com/shichao402/pkv/internal/state"
)

type Syncer struct {
	state *state.State
}

type syncPlan struct {
	targetDir string
	folder    string
	deletes   []state.NoteEntry
	writes    []plannedWrite
}

type plannedWrite struct {
	itemID      string
	fileName    string
	filePath    string
	oldPath     string
	content     string
	contentHash string
	skipWrite   bool
}

type syncPreflightError struct {
	issues []string
}

func (e *syncPreflightError) Error() string {
	if len(e.issues) == 1 {
		return fmt.Sprintf("note sync aborted: %s\nNo local files were changed.", e.issues[0])
	}
	return fmt.Sprintf(
		"note sync aborted due to %d issue(s):\n- %s\nNo local files were changed.",
		len(e.issues),
		strings.Join(e.issues, "\n- "),
	)
}

func NewSyncer(st *state.State) *Syncer {
	return &Syncer{state: st}
}

// SyncFolder reconciles all config notes from a folder into the target directory.
// Existing tracked files are updated in place, remote renames are reflected locally,
// and deleted remote notes are removed from the target directory.
func (s *Syncer) SyncFolder(items []types.Item, targetDir, folder string) (int, error) {
	if absTargetDir, err := filepath.Abs(targetDir); err == nil {
		targetDir = absTargetDir
	}

	plan, err := s.planSync(items, targetDir, folder)
	if err != nil {
		return 0, err
	}
	if err := s.applySyncPlan(plan); err != nil {
		return 0, err
	}

	return len(plan.writes), nil
}

func (s *Syncer) planSync(items []types.Item, targetDir, folder string) (*syncPlan, error) {
	tracked := s.state.FindSyncedNotes(folder, targetDir)
	trackedByID := make(map[string]state.NoteEntry, len(tracked))
	for _, entry := range tracked {
		trackedByID[entry.ItemID] = entry
	}

	remoteByID := make(map[string]types.Item, len(items))
	for _, item := range items {
		remoteByID[item.ID] = item
	}

	plan := &syncPlan{targetDir: targetDir, folder: folder}
	issues := make([]string, 0)
	removedPaths := make(map[string]struct{})

	for _, entry := range tracked {
		if _, ok := remoteByID[entry.ItemID]; ok {
			continue
		}
		if err := validateTrackedRemoval(entry); err != nil {
			issues = append(issues, err.Error())
			continue
		}
		plan.deletes = append(plan.deletes, entry)
		removedPaths[entry.FilePath] = struct{}{}
	}

	finalPathOwners := make(map[string][]string)
	for _, item := range items {
		entry, exists := trackedByID[item.ID]
		write, itemIssues := planWrite(item, entry, exists, targetDir, folder)
		if len(itemIssues) > 0 {
			issues = append(issues, itemIssues...)
			continue
		}
		plan.writes = append(plan.writes, write)
		finalPathOwners[write.filePath] = append(finalPathOwners[write.filePath], item.Name)
		if write.oldPath != "" && write.oldPath != write.filePath {
			removedPaths[write.oldPath] = struct{}{}
		}
	}

	releasedPaths := collectReleasedPaths(targetDir, removedPaths)
	issues = append(issues, pathConflictIssues(finalPathOwners, targetDir)...)
	issues = append(issues, preflightTargetIssues(plan.writes, targetDir, releasedPaths)...)
	if len(issues) > 0 {
		return nil, &syncPreflightError{issues: issues}
	}

	return plan, nil
}

func validateTrackedRemoval(entry state.NoteEntry) error {
	label := trackedEntryLabel(entry)
	localHash, hasLocalFile, err := currentFileHash(entry.FilePath)
	if err != nil {
		return fmt.Errorf("read local stale note '%s': %w", label, err)
	}
	if hasLocalFile && entry.ContentHash != "" && localHash != entry.ContentHash {
		return fmt.Errorf("local note '%s' was modified after last sync; refusing to remove it because the remote note is gone", label)
	}
	return nil
}

func planWrite(item types.Item, entry state.NoteEntry, tracked bool, targetDir, folder string) (plannedWrite, []string) {
	if item.Notes == "" {
		return plannedWrite{}, []string{fmt.Sprintf("item '%s' has no note content", item.Name)}
	}

	targetName, err := resolveNoteTarget(item, folder)
	if err != nil {
		return plannedWrite{}, []string{fmt.Sprintf("prepare note '%s': %v", item.Name, err)}
	}

	filePath, err := resolveNotePath(targetDir, targetName)
	if err != nil {
		return plannedWrite{}, []string{fmt.Sprintf("prepare note '%s': %v", item.Name, err)}
	}

	write := plannedWrite{
		itemID:      item.ID,
		fileName:    item.Name,
		filePath:    filePath,
		content:     item.Notes,
		contentHash: hashContent(item.Notes),
	}
	if !tracked {
		return write, nil
	}

	label := trackedEntryLabel(entry)
	localHash, hasLocalFile, err := currentFileHash(entry.FilePath)
	if err != nil {
		return plannedWrite{}, []string{fmt.Sprintf("read local note '%s': %v", label, err)}
	}
	if hasLocalFile && entry.ContentHash != "" && localHash != entry.ContentHash {
		return plannedWrite{}, []string{fmt.Sprintf("local note '%s' was modified; use 'pkv edit %s note %s' or remove the local file before syncing", label, folder, entry.FileName)}
	}

	write.oldPath = entry.FilePath
	write.skipWrite = hasLocalFile && entry.ContentHash == write.contentHash && entry.FilePath == filePath
	return write, nil
}

func pathConflictIssues(pathOwners map[string][]string, targetDir string) []string {
	paths := make([]string, 0, len(pathOwners))
	for path := range pathOwners {
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return nil
	}
	sort.Strings(paths)

	issues := make([]string, 0)
	for _, path := range paths {
		owners := append([]string(nil), pathOwners[path]...)
		sort.Strings(owners)
		if len(owners) > 1 {
			issues = append(issues, fmt.Sprintf(
				"multiple remote notes map to the same local path '%s': %s",
				displayPath(path, targetDir),
				strings.Join(owners, ", "),
			))
		}
	}

	for i := 0; i < len(paths); i++ {
		for j := i + 1; j < len(paths); j++ {
			if !isPathAncestor(paths[i], paths[j]) {
				continue
			}
			leftOwners := append([]string(nil), pathOwners[paths[i]]...)
			rightOwners := append([]string(nil), pathOwners[paths[j]]...)
			sort.Strings(leftOwners)
			sort.Strings(rightOwners)
			issues = append(issues, fmt.Sprintf(
				"remote notes require conflicting local paths '%s' and '%s': %s | %s",
				displayPath(paths[i], targetDir),
				displayPath(paths[j], targetDir),
				strings.Join(leftOwners, ", "),
				strings.Join(rightOwners, ", "),
			))
		}
	}

	return issues
}

func preflightTargetIssues(writes []plannedWrite, targetDir string, releasedPaths map[string]struct{}) []string {
	issues := make([]string, 0)
	for _, write := range writes {
		if err := ensureTargetPathAvailable(write.filePath, targetDir, write.oldPath, releasedPaths); err != nil {
			issues = append(issues, fmt.Sprintf("prepare note '%s': %v", write.fileName, err))
		}
	}
	return issues
}

func ensureTargetPathAvailable(path, targetDir, currentPath string, releasedPaths map[string]struct{}) error {
	if err := ensureParentDirsAvailable(path, targetDir, releasedPaths); err != nil {
		return err
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		if hasReleasedAncestor(path, targetDir, releasedPaths) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		if _, ok := releasedPaths[path]; ok {
			return nil
		}
		return fmt.Errorf("target path is a directory: %s", displayPath(path, targetDir))
	}
	if path == currentPath {
		return nil
	}
	if _, ok := releasedPaths[path]; ok {
		return nil
	}
	return fmt.Errorf("file already exists: %s", displayPath(path, targetDir))
}

func hasReleasedAncestor(path, targetDir string, releasedPaths map[string]struct{}) bool {
	current := filepath.Dir(path)
	for {
		if current == "." || current == string(os.PathSeparator) || current == targetDir {
			return false
		}
		if _, ok := releasedPaths[current]; ok {
			return true
		}
		next := filepath.Dir(current)
		if next == current {
			return false
		}
		current = next
	}
}

func ensureParentDirsAvailable(path, targetDir string, releasedPaths map[string]struct{}) error {
	parent := filepath.Dir(path)
	rel, err := filepath.Rel(targetDir, parent)
	if err != nil || rel == "." {
		return nil
	}

	current := targetDir
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Stat(current)
		if err != nil {
			if hasReleasedAncestor(current, targetDir, releasedPaths) {
				continue
			}
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if !info.IsDir() {
			if _, ok := releasedPaths[current]; ok {
				continue
			}
			return fmt.Errorf("parent path is a file: %s", displayPath(current, targetDir))
		}
	}
	return nil
}

func collectReleasedPaths(targetDir string, removedPaths map[string]struct{}) map[string]struct{} {
	released := make(map[string]struct{}, len(removedPaths))
	if len(removedPaths) == 0 {
		return released
	}

	if absTargetDir, err := filepath.Abs(targetDir); err == nil {
		targetDir = absTargetDir
	}

	memo := map[string]bool{targetDir: false}
	for path := range removedPaths {
		released[path] = struct{}{}
	}

	for path := range removedPaths {
		if _, err := os.Lstat(path); err != nil {
			continue
		}
		dir := filepath.Dir(path)
		for {
			if dir == "." || dir == string(os.PathSeparator) || dir == targetDir {
				break
			}
			if !pathWillBeReleased(dir, targetDir, removedPaths, memo) {
				break
			}
			released[dir] = struct{}{}
			next := filepath.Dir(dir)
			if next == dir {
				break
			}
			dir = next
		}
	}

	return released
}

func pathWillBeReleased(path, targetDir string, removedPaths map[string]struct{}, memo map[string]bool) bool {
	if path == "" || path == "." || path == targetDir {
		return false
	}
	if released, ok := memo[path]; ok {
		return released
	}
	if _, ok := removedPaths[path]; ok {
		memo[path] = true
		return true
	}

	info, err := os.Lstat(path)
	if err != nil {
		memo[path] = false
		return false
	}
	if !info.IsDir() {
		memo[path] = false
		return false
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		memo[path] = false
		return false
	}
	for _, entry := range entries {
		childPath := filepath.Join(path, entry.Name())
		if !pathWillBeReleased(childPath, targetDir, removedPaths, memo) {
			memo[path] = false
			return false
		}
	}

	memo[path] = true
	return true
}

func isPathAncestor(parent, child string) bool {
	if parent == child {
		return false
	}
	return strings.HasPrefix(child, parent+string(os.PathSeparator))
}

func displayPath(path, targetDir string) string {
	rel, err := filepath.Rel(targetDir, path)
	if err == nil && rel != "" && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return rel
	}
	if base := filepath.Base(path); base != "" && base != "." {
		return base
	}
	return path
}

func (s *Syncer) applySyncPlan(plan *syncPlan) error {
	for _, entry := range plan.deletes {
		if err := s.Remove(entry); err != nil {
			return fmt.Errorf("remove stale note '%s': %w", entry.FileName, err)
		}
	}

	for _, write := range plan.writes {
		if write.oldPath == "" || write.oldPath == write.filePath {
			continue
		}
		if err := removeNoteFile(write.oldPath, plan.targetDir); err != nil {
			return fmt.Errorf("replace tracked note '%s': %w", write.fileName, err)
		}
	}

	for _, write := range plan.writes {
		if write.skipWrite {
			continue
		}
		action := "write"
		if write.oldPath != "" {
			action = "update"
		}
		if err := writeNoteFile(write.filePath, write.content); err != nil {
			return fmt.Errorf("%s note '%s': %w", action, write.fileName, err)
		}
	}

	for _, entry := range plan.deletes {
		s.state.RemoveNoteForTarget(entry.ItemID, plan.folder, plan.targetDir)
	}
	for _, write := range plan.writes {
		s.state.AddNote(state.NoteEntry{
			ItemID:      write.itemID,
			Folder:      plan.folder,
			TargetDir:   plan.targetDir,
			FileName:    write.fileName,
			FilePath:    write.filePath,
			ContentHash: write.contentHash,
		})
	}
	return nil
}

func writeNoteFile(path, content string) error {
	if err := ensureParentDir(path); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}

func hashContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func currentFileHash(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return hashContent(string(data)), true, nil
}

func removeNoteFile(path, targetDir string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	_ = removeEmptyParentDirs(path, targetDir)
	return nil
}

// Remove deletes a previously synced note file.
func (s *Syncer) Remove(entry state.NoteEntry) error {
	return removeNoteFile(entry.FilePath, noteCleanupRoot(entry))
}

func noteCleanupRoot(entry state.NoteEntry) string {
	if entry.TargetDir != "" {
		return entry.TargetDir
	}
	if entry.FilePath != "" {
		return filepath.Dir(entry.FilePath)
	}
	return ""
}

func resolveNotePath(targetDir, noteName string) (string, error) {
	if strings.TrimSpace(noteName) == "" {
		return "", fmt.Errorf("note name is empty")
	}
	cleanName := filepath.Clean(noteName)
	if cleanName == "." {
		return "", fmt.Errorf("note name resolves to current directory")
	}
	if filepath.IsAbs(cleanName) {
		return "", fmt.Errorf("absolute note paths are not allowed")
	}
	if cleanName == ".." || strings.HasPrefix(cleanName, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("note path cannot escape target directory")
	}
	fullPath := filepath.Join(targetDir, cleanName)
	absTargetDir, err := filepath.Abs(targetDir)
	if err != nil {
		absTargetDir = targetDir
	}
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		absPath = fullPath
	}
	if absPath != absTargetDir && !strings.HasPrefix(absPath, absTargetDir+string(os.PathSeparator)) {
		return "", fmt.Errorf("note path cannot escape target directory")
	}
	return absPath, nil
}

func resolveNoteTarget(item types.Item, folder string) (string, error) {
	target := item.NoteTargetPath()
	switch item.NoteStrategy() {
	case types.NoteStrategyFile:
		if target == "" {
			return item.Name, nil
		}
		return target, nil
	case types.NoteStrategyMiseConfD:
		if target == "" {
			target = defaultMiseConfDTarget(folder, item.Name)
		}
		if err := validateMiseConfDTarget(target); err != nil {
			return "", err
		}
		return target, nil
	default:
		return "", fmt.Errorf("unknown %s value %q", types.PKVNoteStrategyFieldName, item.NoteStrategy())
	}
}

func validateMiseConfDTarget(target string) error {
	if strings.TrimSpace(target) == "" {
		return fmt.Errorf("%s is empty", types.PKVNoteTargetFieldName)
	}

	normalized := filepath.ToSlash(filepath.Clean(target))
	if !strings.HasPrefix(normalized, ".config/mise/conf.d/") {
		return fmt.Errorf("%s must stay under .config/mise/conf.d/: %s", types.PKVNoteTargetFieldName, target)
	}
	if !strings.HasSuffix(normalized, ".toml") {
		return fmt.Errorf("%s must end with .toml: %s", types.PKVNoteTargetFieldName, target)
	}
	return nil
}

func defaultMiseConfDTarget(folder, noteName string) string {
	folderSlug := noteFragmentSlug(folder)
	if folderSlug == "" {
		folderSlug = "folder"
	}
	noteSlug := noteFragmentSlug(noteName)
	if noteSlug == "" {
		noteSlug = "note"
	}
	return filepath.Join(".config", "mise", "conf.d", fmt.Sprintf("pkv-%s-%s.toml", folderSlug, noteSlug))
}

func noteFragmentSlug(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, "\\", "/")
	value = strings.TrimSuffix(value, ".toml")

	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		default:
			if lastDash {
				continue
			}
			b.WriteByte('-')
			lastDash = true
		}
	}

	return strings.Trim(b.String(), "-")
}

func trackedEntryLabel(entry state.NoteEntry) string {
	target := entryTargetName(entry)
	if target == "" || entry.FileName == "" || target == entry.FileName {
		if target != "" {
			return target
		}
		return entry.FileName
	}
	return fmt.Sprintf("%s (source %s)", target, entry.FileName)
}

func entryTargetName(entry state.NoteEntry) string {
	if entry.FilePath == "" {
		return entry.FileName
	}
	if root := noteCleanupRoot(entry); root != "" {
		return displayPath(entry.FilePath, root)
	}
	if base := filepath.Base(entry.FilePath); base != "" && base != "." {
		return base
	}
	return entry.FileName
}

func ensureParentDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o700)
}

func removeEmptyParentDirs(path, targetDir string) error {
	if targetDir == "" {
		return nil
	}
	stopDir, err := filepath.Abs(targetDir)
	if err != nil {
		stopDir = targetDir
	}
	dir := filepath.Dir(path)
	for {
		if dir == "." || dir == string(os.PathSeparator) || dir == stopDir {
			return nil
		}
		err := os.Remove(dir)
		if err == nil {
			next := filepath.Dir(dir)
			if next == dir {
				return nil
			}
			dir = next
			continue
		}
		if os.IsNotExist(err) {
			next := filepath.Dir(dir)
			if next == dir {
				return nil
			}
			dir = next
			continue
		}
		if strings.Contains(err.Error(), "directory not empty") {
			return nil
		}
		if _, ok := err.(*os.PathError); ok {
			return nil
		}
		return err
	}
}
