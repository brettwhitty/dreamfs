---
type: MANUAL
authority: Brett Whitty
review_status: APPROVED
version: 0.1.1
approved_versions: 0.1.*
generated_on: 2026-01-22 17:15
origin_persona: Brett Whitty
origin_session: d346cd76-699f-4ab0-b24a-53180588cb07
intent: Document Gitea authentication and permission management using the tea CLI.
primary_sources: [docs/agent/skills/gitea_auth/SKILL.md]
release_path: docs/agent/skills/gitea_auth/SKILL.md
related_issues: []
related_sops: []
tags: [skill, gitea, auth]
---

# SKILL: Gitea Authentication

**Goal:** Manage authentication sessions and verify permissions for Gitea interactions.

## 1. Check Active Logins
List all configured accounts. Use stdin redirection to avoid hanging.
```bash
tea logins list < NUL
```

## 2. Add New Login
Interactive setup for a new account.
```bash
tea login add
```

## 3. Switch Active User
Use the `--login` flag in any command to execute as a specific user, or delete/re-add to change default.
```bash
tea <command> --login <username>
```

## 4. Troubleshooting
*   **Error:** "user should be an owner or a collaborator"
*   **Diagnosis:** The current login lacks `write` or `admin` scope on the target repo.
*   **Fix:**
    1.  Check `tea logins list`.
    2.  If a privileged user exists, use `--login <user>`.
    3.  If not, ask User to grant permissions via Web UI.
