# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v0.2.2] - 2026-03-29

### Added
- `pkv env <folder>` - Deploy environment variables from Bitwarden Secure Notes
- `pkv env <folder> clean` - Remove deployed environment variables
- Supports KEY=VALUE, export KEY=VALUE, comments, and quoted values
- On Linux/macOS, writes to `~/.pkv/env.sh` and auto-sources from shell rc files
- On Windows, sets persistent User environment variables via PowerShell

### Changed
- Introduced `version.json` as the single source of truth for version numbers

## [v0.1.0] - 2026-03-28

### Added
- Initial release of PKV
- `pkv ssh <folder>` - Deploy SSH keys from Bitwarden folder with automatic config generation
- `pkv ssh <folder> clean` - Remove deployed SSH keys and configuration
- `pkv note <folder>` - Sync Bitwarden Secure Notes to current directory as files
- `pkv note <folder> clean` - Remove synced note files
- `pkv update` - Self-update to latest version from GitHub Releases
- `pkv --version` - Display version information
- Automatic `ssh-keyscan` for `known_hosts` management
- Installation script for one-command setup
- Support for Linux and macOS, amd64 and arm64 architectures
