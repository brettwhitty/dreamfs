---
type: MANUAL
authority: Brett Whitty
review_status: APPROVED
version: 0.1.1
approved_versions: 0.1.*
generated_on: 2026-01-22 17:15
origin_persona: Brett Whitty
origin_session: d346cd76-699f-4ab0-b24a-53180588cb07
intent: Document Gitea branch protection management using the tea CLI.
primary_sources: [docs/agent/skills/gitea_branch_protection/SKILL.md]
release_path: docs/agent/skills/gitea_branch_protection/SKILL.md
related_issues: []
related_sops: []
tags: [skill, gitea, branch-protection]
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
