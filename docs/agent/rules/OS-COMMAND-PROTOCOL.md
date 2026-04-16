---
type: SOP
authority: Brett Whitty
review_status: PENDING
version: 0.1.1
approved_versions: None
generated_on: 2026-01-22 17:15
origin_persona: Rory Devlin (R&D Team Lead)
origin_session: d346cd76-699f-4ab0-b24a-53180588cb07
intent: Establish a protocol for OS-aware command execution to prevent shell-specific errors.
primary_sources: [user feedback]
release_path: docs/agent/rules/OS-COMMAND-PROTOCOL.md
related_issues: []
related_sops: []
tags: [agent, rules, platform, powershell]
---

# SOP: OS-Aware Command Execution Protocol

## Context
The agent identifies the host operating system and shell environment (e.g., Windows PowerShell, CMD, Linux Bash) and must ensure all generated commands are compatible with that specific environment.

## Protocol

### 1. Identify Environment (Triggered Action)
Whenever the user invokes "Hello", a mode change (e.g., `//office`), or a role change (e.g., `//rd`), the agent **MUST** perform an immediate environmental assessment:
1.  **Cache Check**: Check for `.gemini-cache/environment.json`. If present and valid (last_updated < 7 days), load these values to memory.
2.  **Live Check (Fallback)**: If cache is missing or stale, execute:
    - **OS Check**: Verify current platform (e.g., `Get-ComputerInfo` or `uname -a`).
    - **Shell Check**: Identify active shell (PowerShell, Bash, CMD).
    - **Tool Check**: Verify critical tools (`git`, `curl`, `tea`).
    - **Cache Update**: Write the new state to `.gemini-cache/environment.json`.
3.  **Reminders**: Set internal context reminders for appropriate path separators (`\` vs `/`), command naming (cmdlet vs alias), and argument syntax.

### 2. Tool Discovery
If a core command is expected but fails, or upon initial role adoption, check for tool availability:
- **Gnomatix Stack**: `flox`, `mise`, `tea`.
- **System Tools**: `git`, `curl`, `grep`.
- **PowerShell modules**: `PSReadLine`, etc.

### 3. Alias Safety & Utility Layer
The agent utilizes a Cross-Platform Utility Layer to abstract OS differences and enforce safety.
- **Bootstrapping**: Upon session start or role change, run `mise --env win11 run setup:session` to hydrate the session binary path.
- **Execution**: Prefer `mise --env <ENV> run <TASK>` over raw commands.
- **Enforce Full Cmdlets**: If a shim is unavailable, never use aliases. Always use the full cmdlet name (e.g., `Get-ChildItem`).
- **High-Risk Overlaps**: Avoid `ls`, `dir`, `cat`, `rm`, `cp`, `mv`, `ps`, `kill` without the `g_` prefix shim.
- **Verify with Get-Alias**: If a command behaves unexpectedly, verify if an alias is intercepting the call.

### 4. Command Selection
- **Prioritize Cmdlets**: In PowerShell, prefer native cmdlets to ensure better error handling and object-oriented output.
- **Clean Execution**: Always use `pwsh -NoProfile -Command "..."` and mandate structured output (e.g., `--output json`) for complex CLI tools to ensure data integrity and prevent parsing failures.
- **Avoid Mixed Syntax**: Do not combine CMD flags (e.g., `/q`, `/s`) with PowerShell or Unix commands.
- **Verify Path Formats**: Use absolute paths wherever possible.

### 5. Verification
When a command fails due to "Invalid argument" or "Syntax incorrect," immediately re-evaluate the command against the current OS environment rather than retrying with identical syntax.
