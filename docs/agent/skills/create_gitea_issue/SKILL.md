---
name: create_gitea_issue
description: Create a new issue in a Gitea repository using the tea CLI.
---

# Skill: Create Gitea Issue

This skill standardizes the creation of issues in Gitea using the `tea` CLI tool, ensuring proper tagging, formatting, and syntax safety (especially in PowerShell environments).

## Usage

When you need to track a bug, task, or research item, use this pattern.

### Command Template

```powershell
tea issue create --repo <owner>/<repo> --title "<Title>" --description "<Description>" --labels "<label1>,<label2>" --login admin
```

### Critical Rules

1.  **Repo format**: Always use the full `owner/repo` format (e.g., `brett/dreamfs`). Do not rely on context detection or shortnames.
2.  **Quoting**: Enclose all string arguments (`--title`, `--description`) in double quotes `"`.
3.  **No Redirection**: Do NOT append `< NUL` or `| Out-Null` unless absolutely necessary for a specific piping reason, and only if tested. The `<` operator is invalid in PowerShell for input redirection in this context.
4.  **Login**: Explicitly specify `--login admin` (or appropriate user) to avoid interactive prompts.

### Example

To create a bug report:

```powershell
tea issue create --repo brett/dreamfs --title "BUG: Search tool failure" --description "The search_web tool returned error 500." --labels "bug,tooling" --login admin
```
