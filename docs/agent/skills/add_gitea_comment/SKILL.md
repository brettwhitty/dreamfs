---
name: add_gitea_comment
description: Add a comment to an existing Gitea issue using the tea CLI.
---

# Skill: Add Gitea Comment

This skill standardizes the process of adding comments to Gitea issues using the `tea` CLI tool, avoiding common syntax errors such as invalid flags.

## Usage

When you need to update an issue with findings, status changes, or general notes, use this pattern.

### Command Template

```powershell
tea comment <issue_index> "<Comment Body>" --repo <owner>/<repo> --login admin
```

### Critical Rules

1.  **Positional Arguments**: The comment body MUST be passed as a positional argument immediately following the issue index. Do NOT use a `--body` flag (it does not exist).
2.  **Repo format**: Always use the full `owner/repo` format (e.g., `brett/dreamfs`).
3.  **Quoting**: Enclose the comment body in double quotes `"`. Escape internal quotes if necessary.
4.  **Login**: Explicitly specify `--login admin` to avoid interactive prompts.

### Example

To update an issue with a status report:

```powershell
tea comment 42 "Work paused for Office Mode sync." --repo brett/dreamfs --login admin
```
