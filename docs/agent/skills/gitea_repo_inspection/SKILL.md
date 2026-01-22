---
name: Gitea Repository Inspection
description: Inspect repository details, existence, and mirrors using tea CLI.
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
