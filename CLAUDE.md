# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What DreamFS Is

DreamFS is a scalable, extensible P2P/swarm schema-validated accessioning and tagging engine — an entity/property store. It is NOT a distributed filesystem and NOT a file syncing tool. It's a metadata layer that sits on top of whatever filesystems already exist, providing a unified view across a heterogeneous private swarm of machines. The core function is **accessioning** — the archival science term for formally registering items into a managed collection, assigning identifiers, and recording properties.

**Core scope**: object store + accessioning + swarm. Everything else is extensions.

The project is owned by GNOMATIX and hosted on Gitea at `gitea.gnomatix.com/brett/dreamfs`.

## Project State and Why the Greenfield Review Exists

The previous LLM-assisted development iteration (primarily Gemini) **failed**. The agent didn't understand the problem DreamFS solves. It made unverified assumptions, recommended technologies without researching them, presented hallucinated information as fact, and built agent-invented features that didn't trace to the project owner's intent. It fragmented the codebase by iteratively dropping and re-adding features. Specific examples:
- **BoltDB** was chosen by an LLM without justification — BadgerDB is the correct choice (REQ-6.3)
- The agent presented false information about Gitea wiki capabilities that directly influenced design decisions (see wiki-docs section below)
- Features were added that no one asked for (e.g., a `migrate` command — see AUDIT-FINDINGS Finding 9)
- The agent reduced the owner's information science concepts to the first implementation detail that came to mind (e.g., treating CouchDB as THE architecture when it was one example)

A **greenfield vision review** was conducted to do what should have been done first: start from a deep understanding of the actual problem and work top-down through the system layers to produce correct engineering documents. The review included a contamination audit (`CONTAMINATION-REVIEW.md`) that systematically checked every spec item against the question: "does this trace to the owner's explicit direction, or was it invented by a previous agent?"

**Note: The vision design sessions are not fully complete.** Not all implementation aspects and intentional engineering decisions have been fully clarified through human-led discussion. The design docs represent the current state of that process, not a finished product. See `WORKING-STATE.md` for current status.

The authoritative design lives in `.kiro/specs/dreamfs-vision/` — start with `REVIEW-PRINCIPLES.md`, then `synopsis.md`, `requirements.md`, and `design/00-overview.md`.

The existing code works but is being reshaped to match the vision. Key pending changes:
- Rename "index" → "accession" throughout; `cmd/indexer/` → `cmd/dreamfs/`
- BoltDB → BadgerDB (sole embedded backend; BoltDB was an unresearched LLM choice — REQ-6.3)
- `FileMetadata` → generic `Entity` type with JSON-LD/JSON-Schema validation
- New layers: ObjectStore, NamespaceManager, Extension Runtime, Volume Manager, Host Capability Assessment
- `pkg/network/` → `pkg/swarm/` with delegate race fix, PeerRegistry extraction

## Build & Test Commands

```bash
# Build the main binary (currently still cmd/indexer/, pending rename to cmd/dreamfs/)
go build -o dreamfs cmd/indexer/main.go

# Build wiki-docs
go build -o wiki-docs cmd/wiki-docs/main.go

# Cross-platform build (outputs dreamfs-{os}-amd64 binaries)
go run ./build/build.go
go run ./build/build.go -platforms linux,windows

# Run all tests
go test ./pkg/...

# Run tests for a specific package
go test ./pkg/storage -v
go test ./pkg/fileprocessor -v

# Run a single test
go test ./pkg/storage -v -run TestPutAndGetAll
```

No Makefile. Environment tooling managed via `mise` (see `mise.toml`). Go version is `latest` (currently 1.26.1).

## Architecture

### Two CLI Applications (both Cobra/Viper)

**`cmd/indexer/`** (→ `cmd/dreamfs/`) — The core DreamFS node. Commands: `index` (→ `accession`), `serve`, `dump`, `monitor`. See `.kiro/specs/dreamfs-vision/design/14-cli.md` for the full target CLI.

**`cmd/wiki-docs/`** — **This is a separate project that needs to be factored out into its own repo.** It is NOT part of the DreamFS runtime. wiki-docs is a tool for wiki-managed org-wide document staging into repos — the wiki is the authoritative source, and wiki-docs stages approved/versioned documents into local repos. It is NOT a synchronization tool. It lives here temporarily because it was developed to assist in migrating documents out of this repo that actually belong in the GNOMATIX-wide wiki (skills files, SOPs, agent instructions, etc.). That migration is still in progress — local docs files in this repo should NOT have YAML frontmatter; the frontmatter belongs to wiki-managed docs only.

**Known design flaw from Gemini hallucination**: The tilde-flattened wiki path convention (`src_docs~docs~agent~rules.md`) exists because Gemini falsely claimed that Gitea wikis do not support `/` for hierarchical directory-style namespacing. This was hallucinated — Gitea wikis DO support subdirectories. The project owner made the flattening design decision based on that false information. This hallucination was further perpetuated by a Kiro session assessment (gnomatix/dreamfs#19, comment #1506) which cited non-existent Gitea issues (#2869, #7471) as evidence. The original plan (gnomatix/dreamfs#19) was for versioned hierarchical paths: `/[VERSION_TAG]/repo_docs_root/...` with Major.Minor bucketing (e.g., `version-1-1`). When wiki-docs is factored out, the `/` path support must be restored and the original versioned path design revisited.

**wiki-docs is NOT npm-packages**: These are completely different tools serving different use cases. wiki-docs is for staging wiki-managed org-wide documents into repo directories — org-roles, skills, SOPs, agent instructions, design docs being developed in the wiki that need to land in a repo to become a real project, product requirements maintained by various teams, etc. The purpose is to give AI models (and developers) local references to org-wide knowledge and skills. npm-packages is a separate tool for a separate purpose. When gnobot exists, the repo-staging approach may no longer be necessary.

wiki-docs stages documents from the wiki into repos via: YAML frontmatter validation against `.schemas/frontmatter.yaml`, template inheritance from `.templates/`, checksum-based change detection, state tracking (`.config/wiki-docs/state.json`), and a Bubble Tea TUI for interactive management.

### Current Packages (`pkg/`)

| Package | Purpose | Planned Changes |
|---|---|---|
| `fileprocessor` | Directory walking (godirwalk), BLAKE3 fingerprinting, cross-platform path canonicalization | Extract to `ext/accession/file/` as AccessionHandler |
| `storage` | BoltDB persistence + `CacheWriter` (channel-based async batching) | BoltDB removed; CacheWriter pattern preserved over BadgerDB |
| `metadata` | `FileMetadata` with custom JSON marshaling and extensible `Extra` map | Becomes generic `Entity` type |
| `network` | P2P swarm (hashicorp/memberlist), mDNS discovery, HTTP peer exchange | Refactor to `pkg/swarm/`, fix delegate race, extract PeerRegistry |
| `api` | Gin REST API (`/api/v1/`) with thread-safe `IndexerState` | Rename to `AccessionState`, add extension route registration |
| `config` | Viper-based config with XDG directory standards | Per-component hierarchical config (mise/LXD model), not monolithic |
| `ui` | Charmbracelet TUI styling with Wes Anderson-themed palettes | Unchanged |
| `utils` | XDG directories, machine ID, UUID helpers | Unchanged |

### Key Design Patterns

- **Content-addressed storage**: Each accession is an immutable observation. UUID deterministically derived from IDString (`hostID|volumeID|canonicalPath|modTime|size`). BLAKE3 content hash is the natural join key across observations.
- **BLAKE3 sampling**: Files < 3MB hashed entirely; >= 3MB hash head + middle + tail (1MB each).
- **Extension model**: JSON in, JSON out, validated against JSON-Schema per namespace. Extensions chain other extensions. Categories: accessioning, annotation, metadata tools, file operations, display/export, storage.
- **Swarm-first**: Always on (REQ-8.9). Cost/benefit work routing — data streams from where it lives, compute happens where capacity exists.
- **Namespacing**: First-class concept rooted in information science. Core `dreamfs:` namespace for accessioned entities; each extension gets its own namespace with JSON-Schema validation.

## Design Documents (Authoritative)

Read in this order per `REVIEW-PRINCIPLES.md`:
1. `.kiro/specs/dreamfs-vision/synopsis.md` — owner's vision in their own words
2. `.kiro/specs/dreamfs-vision/requirements.md` — formal requirements
3. `.kiro/specs/dreamfs-vision/design/` — 18 design documents (start at `00-overview.md`)
4. `.kiro/specs/dreamfs-vision/WORKING-STATE.md` — standing instructions and status
5. `.kiro/specs/dreamfs-vision/AUDIT-FINDINGS.md` — resolved/deferred findings
6. `.kiro/specs/dreamfs-vision/CONTAMINATION-REVIEW.md` — traceability review
7. `.kiro/specs/dreamfs-vision/STANDARDS-AUDIT.md` — pending standards work

Older docs like `docs/DEVELOPMENT-PROPOSALS.md` are superseded by the vision specs.

## Agent Behavioral Constraints (from GEMINI.md)

These apply to all AI agents working in this repo:

- **Concise CLI output**: Fewer than 3 lines of text per response (excluding tool use/code generation).
- **No chitchat**: No preambles or postambles.
- **Investigation before action**: Do not rely on assumptions. Actively investigate the workspace before acting.
- **User as sole authority**: Never act without structured proposal and user approval.
- **Flow control**: Await explicit instructions. Do not prompt or influence task progression.
- **Research before recommending**: Actually look at what a technology does. Consider alternatives. Present accurate information. Do not present guesses as facts.
- **Flag, don't assume**: If something is unclear, flag it and ask. The cost of asking is low; the cost of building the wrong thing is high.

## Document YAML Frontmatter

Wiki-managed documents use YAML frontmatter with fields: `type`, `authority`, `review_status`, `version`, `approved_versions`, `origin_persona`, `intent`, `tags`, etc. **Local docs in this repo should NOT have YAML frontmatter** — frontmatter belongs to the wiki-managed lifecycle. Docs currently in this repo that have frontmatter (e.g., `README.md`, `GEMINI.md`) are artifacts of the ongoing migration and need to be cleaned up. The `wiki-docs` tool validates frontmatter against `.schemas/frontmatter.yaml` in the wiki.

## Git & Branching

- Main development branch: `develop`
- Remote: Gitea (`gitea.gnomatix.com`)
- Commit style: `fix: <description> (#issue)` — conventional commits with Gitea issue references
- Protected branches: `main`/`master` (wiki-docs enforces this for wiki pushes)
- Some paths encrypted with git-crypt
