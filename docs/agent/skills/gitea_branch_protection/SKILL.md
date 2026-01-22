---
name: Gitea Branch Protection
description: Manage branch protection rules (e.g., enable/disable Force Push) via tea CLI or Web UI.
---

# SKILL: Gitea Branch Protection

**Goal:** Unblock dangerous operations (Force Push) by temporarily disabling branch protection.

## 1. Unprotect Branch (Allow Force Push)
**Tool:** `tea`
**Requirement:** Owner/Admin permissions.

```bash
tea branch unprotect <branch_name> --repo <owner>/<repo> --login <user> < NUL
# Example:
tea branch unprotect main --repo brett/dreamfs --login admin < NUL
```

## 2. Protect Branch
Restore safety locks after operations are complete.

```bash
tea branch protect <branch_name> --repo <owner>/<repo>
```

## 3. Fallback: Web UI
If CLI fails (permissions/API limits):
1.  Go to Repo Settings > Branches.
2.  Edit Rule for `<branch_name>`.
3.  Uncheck "Enable Branch Protection" OR check "Allow Force Pushes".
4.  Save.
