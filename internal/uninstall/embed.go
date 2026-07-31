package uninstall

import _ "embed"

// Script is the cross-platform PKV uninstall helper (Python 3 stdlib only).
//
//go:embed uninstall.py
var Script string
