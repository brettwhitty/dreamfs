---
type: MANUAL
authority: Brett Whitty
review_status: APPROVED
version: 0.1.1
approved_versions: 0.1.*
generated_on: 2026-01-22 17:15
origin_persona: Brett Whitty
origin_session: d346cd76-699f-4ab0-b24a-53180588cb07
intent: Document repository inspection and metadata verification using the tea CLI.
primary_sources: [docs/agent/skills/gitea_repo_inspection/SKILL.md]
release_path: docs/agent/skills/gitea_repo_inspection/SKILL.md
related_issues: []
related_sops: []
tags: [skill, gitea, inspection]
---

# SKILL: Gitea Repository Inspection

**Goal:** Verify repository metadata, existence, and clone URLs.

## 1. List Repositories
View all repositories manageable by the current user. Always specify login and redirect stdin.
```bash
tea repo list --output table --fields name,owner,ssh,permission --login <user> < NUL
```

## 2. Inspect Specific Repo
(Note: `tea` lacks a direct "show" command for repos, use list filters)
```bash
tea repo list --pattern <name>
```

## 3. Verify Login Status
If the list is empty unexpectedly, check auth:
```bash
tea logins list
```
