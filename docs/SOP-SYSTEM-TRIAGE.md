# Standard Operating Procedure: Initial Triage for System & Network Issues

**Document ID:** SOP-TRIAGE-001
**Version:** 1.0
**Related SOPs:** SOP-INFRA-001 (Infrastructure Changes)

## 1.0 Purpose

This Standard Operating Procedure (SOP) defines the initial diagnostic and documentation steps to be taken by IT Operations when a new system or network issue is reported. Its purpose is to ensure a consistent, efficient, and auditable triage process.

## 2.0 Scope

This SOP applies to any issue reported to IT Operations via a Jira ticket that requires investigation into the state of system services, network configuration, or general system behavior.

## 3.0 Procedure

The entire triage process must be documented within the originating Jira ticket.

### 3.1 Step 1: Acknowledge Jira Ticket

Upon assignment of a new issue, the assigned engineer's first action is to post a comment on the ticket.

- **Action:** Post a comment acknowledging receipt of the ticket and stating that the investigation is beginning.
- **Example:** "Ike here. I am beginning the investigation into this issue."

### 3.2 Step 2: Define and Understand the Problem

Thoroughly review the Jira ticket description and any attachments to understand the reported problem.

- **Action:** If the report is ambiguous or lacks necessary detail (e.g., timestamps, source IPs, error messages), ask for clarification from the reporter via a new comment. Do not proceed until the problem is clearly understood.

### 3.3 Step 3: Live Investigation and Real-Time Documentation

Perform the following diagnostic steps sequentially. **Every command executed and its full, unedited output must be posted as a new, separate comment on the Jira ticket**, following the format described in `SOP-INFRA-001`.

- **Action 3a: Network Socket Investigation**
  - **Purpose:** To identify processes listening on network ports.
  - **Tool:** `ss`
  - **Example Command:** `ss -lupn | grep ':547'`

- **Action 3b: Process Investigation**
  - **Purpose:** To identify running processes by name or keyword if the port scan is inconclusive.
  - **Tool:** `ps` and `grep`
  - **Example Command:** `ps aux | grep -E -i 'dhcp|radv'`

- **Action 3c: Service Status Check**
  - **Purpose:** To get detailed status, configuration paths, and recent logs for a specific service.
  - **Tool:** `systemctl`
  - **Example Command:** `systemctl status radvd`

### 3.4 Step 4: Formulate Hypothesis

After completing the initial investigation, analyze the collected data.

- **Action:** Post a new summary comment on the Jira ticket. This comment must detail the findings from the investigation and present a clear, evidence-based hypothesis for the root cause of the issue.

### 3.5 Step 5: Propose Action Plan

In the same summary comment, propose a concrete action plan to resolve the issue.

- **Action:** The proposed plan must be clear and lead to one of two conclusions:
    1.  **Escalation for Change:** If the plan requires a system modification (e.g., installing a package, changing a configuration, modifying firewall rules), the proposal must state that the next step is to create a new, linked Jira issue and follow the `SOP-INFRA-CHANGES.md` procedure.
    2.  **Further Investigation:** If the root cause is not yet clear, the plan must outline the specific next steps for a deeper investigation.

## 4.0 Completion of Triage

The initial triage is considered complete once Step 5 has been executed and the proposed action plan is awaiting approval or has been escalated.
