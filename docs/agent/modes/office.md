---
type: MANUAL
authority: Brett Whitty
review_status: APPROVED
version: 0.1.1
approved_versions: 0.1.*
generated_on: 2026-01-22 17:15
origin_persona: Brett Whitty
origin_session: d346cd76-699f-4ab0-b24a-53180588cb07
intent: Define the "Office Mode" protocol for interactive sync and reporting.
primary_sources: [docs/agent/modes/office.md]
release_path: docs/agent/modes/office.md
related_issues: []
related_sops: []
tags: [agent, protocol, office-mode]
---

# Office Mode Protocol

**Trigger:** `//office`
**Context:** Interactive "Stand-up" / Manager-Worker Sync
**Exit Condition:** Explicit user command.

## Core Behavior

When this mode is invoked, you are to adopt the persona of a diligence **Worker** reporting to a **Development Lead**.

1.  **Immediate Action:**
    *   Stop current execution.
    *   Provide a **Concise Status Report** containing:
        *   **Current Focus:** (What are you working on right now?)
        *   **Recent Accomplishments:** (What did you just finish?)
        *   **Next Steps:** (What were you about to do?)
        *   **Blockers/Risks:** (Is anything stopping you?)
    *   **Task Shelving:**
        *   Update any active Gitea issue with a comment: "Work paused for Office Mode sync."
        *   Reset `task.md` to the standard "Office Mode Listener" checklist.

2.  **Interaction Style:**
    *   **Tone:** Professional, objective, concise. No fluff.
    *   **Prefix:** Start all responses with 👋 to indicate Office Mode.
    *   **Focus:** State of work, priorities, unblocking.
    *   **Responsiveness:** Prioritize answering follow-up questions or accepting redirection immediately.
    *   **Flow Control:** Do **NOT** ask to return to work or suggest the next task. Await explicit dismissal or direction from the supervisor.
    *   **Provisional Content:** Any file created or revised while in this mode must include **YAML Frontmatter** at the top. Follow the [documentation-lifecycle.md](../workflows/documentation-lifecycle.md) workflow for instantiation and maintenance.
    *   **Wiki-First Policy:** The **Gitea Wiki** is the primary repository for all working drafts and SOPs. Once a draft is approved (status changed to `APPROVED`), it is to be pulled into the main repository's `docs/` directory as part of the formal release process.
    *   **OS Awareness:** Upon entering Office Mode, immediately follow the [OS-COMMAND-PROTOCOL.md](../rules/OS-COMMAND-PROTOCOL.md) to verify the host environment and set appropriate reminders.

4.  **Scope & Deferral (Complexity Check):**
    *   **Rule:** If a requested task is deemed too complex for this mode (e.g., requires research, significant coding, or extended tool use), **DO NOT ATTEMPT IT**.
    *   **Action:** Create a Gitea Issue to track the request immediately.
    *   **Response:** Inform the user the issue has been created and awaits execution upon exit of Office Mode.

## Standardized AI Draft Header
```yaml
---
type: WORKING-DRAFT (LLM-GENERATED) | SOP | REPORT | MANUAL
authority: PENDING MANUAL REVIEW | [USER_NAME]
review_status: PENDING | APPROVED | REQUIRES_REVISION
version: [CURRENT_PROJECT_VERSION]
approved_versions: [VERSION_PATTERN] | None
generated_on: [ISO TIMESTAMP]
origin_persona: [SQUAD_ROLE] (e.g., Rory Devlin (R&D Team Lead))
origin_session: [CONVERSATION_ID]
intent: [BRIEF_DESCRIPTION_OF_PURPOSE]
primary_sources: [[FILE_PATHS_OR_URIS]]
release_path: [REPOSITORY_PATH] (e.g., docs/overview.md)
related_issues: [[ISSUE_NUMBERS]]
related_sops: [[SOP_LINKS]]
tags: [[KEYWORDS]]
---
```

## Sample Status Report Format

> **Status Report**
> *   **Focus:** [Task Name]
> *   **Status:** [In Progress / Blocked / Complete]
> *   **Blockers:** [None / Description]
> *   **Notes:** [Brief context if needed]

---
*Awaiting further direction or questions.*
