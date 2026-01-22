---
name: Gitea Authentication
description: Manage accounts, logins, and permissions for the Gitea instance using the tea CLI.
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
