package pathutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExpandTilde expands a leading ~ in path to the current user's home directory.
// Paths without a leading ~ are returned unchanged.
func ExpandTilde(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, path[1:]), nil
}

// RelativeFileNoteName returns the slash-separated path to filePath relative to
// the current working directory. It is intended for naming file-backed notes so
// `pkv get <folder> note` can restore them at the same relative location.
func RelativeFileNoteName(filePath string) (string, error) {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return "", fmt.Errorf("file path is required")
	}

	expanded, err := ExpandTilde(filePath)
	if err != nil {
		return "", err
	}
	absFile, err := filepath.Abs(expanded)
	if err != nil {
		return "", err
	}
	evalFile, err := filepath.EvalSymlinks(absFile)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(evalFile)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory: %s", filePath)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("path is not a regular file: %s", filePath)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	absCWD, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	evalCWD, err := filepath.EvalSymlinks(absCWD)
	if err != nil {
		return "", err
	}
	if !isRelativeChild(evalCWD, evalFile) {
		return "", fmt.Errorf("file must be under current directory to derive note name: %s", filePath)
	}
	rel, err := filepath.Rel(absCWD, absFile)
	if err != nil {
		return "", err
	}
	if !isSafeRelativePath(rel) {
		return "", fmt.Errorf("file must be under current directory to derive note name: %s", filePath)
	}
	return filepath.ToSlash(rel), nil
}

func isRelativeChild(base, path string) bool {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return isSafeRelativePath(rel)
}

func isSafeRelativePath(rel string) bool {
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
