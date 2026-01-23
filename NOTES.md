---
type: WORKING-DRAFT (LLM-GENERATED)
authority: PENDING MANUAL REVIEW
review_status: PENDING
version: 0.1.1
approved_versions: None
generated_on: 2026-01-22 17:05
origin_persona: Rory Devlin (R&D Team Lead)
origin_session: d346cd76-699f-4ab0-b24a-53180588cb07
intent: Capture high-level project state and core architecture notes.
primary_sources: [NOTES.md]
release_path: NOTES.md
related_issues: []
related_sops: []
tags: [architecture, state, indexer]
---

# DreamFS v2: Project State Notes

## Core Features
- **CLI Architecture**: Built on `cobra` and `viper`. Entry point: `cmd/indexer/main.go`.
- **File Fingerprinting**: Uses `blake3` for hashing. `fileprocessor.FingerprintFile` samples head, middle, and tail for large files.
- **Persistent Storage**: Uses `bbolt` via `pkg/storage`.
- **Swarm Network**: Leveraging `hashicorp/memberlist` for P2P metadata replication.
- **Service Discovery**: Supports mDNS and HTTP-based peer lists.
- **UI**: Basic implementation using `charmbracelet/bubbles` (progress/spinner) and `lipgloss`.

## Missing / Incomplete Features
- **Monitor Command**: `cmd/indexer/main.go:L178-187` - Stubbed but not implemented.
- **UI Consistency**: `go.mod` includes both `schollz/progressbar` and `charmbracelet/bubbles`. The project directive mandates `charmbracelet`.
- **Metadata Features**: `metadata.FileMetadata` has an `Extra` field (map[string]interface{}) for future custom attributes.

## Observations
- `CanonicalizePath` in `pkg/fileprocessor` handles Windows UNC paths and identifies network filesystems (NFS, CIFS, etc.) to ensure cross-platform metadata consistency.
- `ProcessAllDirectories` handles root and subdirectories separately, providing progress updates via `bubbletea` (nested walkers).
