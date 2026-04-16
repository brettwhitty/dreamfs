# Standard Operating Procedure: Infrastructure Changes

**Document ID:** SOP-INFRA-001
**Version:** 1.0

## 1.0 Purpose

This Standard Operating Procedure (SOP) defines the mandatory process for planning, executing, and documenting all changes to production and development infrastructure. The goal is to ensure all modifications are transparent, auditable, minimally disruptive, and reversible.

## 2.0 Scope

This SOP applies to any modification of system-level components, including but not limited to:

- Firewall rule changes (`iptables`, `ufw`, etc.)
- System package installation, removal, or updates (`apt`, `yum`, etc.)
- Service configuration changes (e.g., `sshd`, `nginx`, databases)
- Changes to user accounts or permissions
- Filesystem and storage modifications

## 3.0 Procedure

All infrastructure changes must follow this five-step process. No work should begin before Step 3 is complete, except in a declared emergency (see Section 5.0).

### 3.1 Step 1: Draft Change Request

A formal Change Request (CR) document must be drafted. The CR must include:

- **Change Summary:** A high-level description of the change.
- **Reason for Change:** The business or technical justification for the work.
- **Technical Plan:** A detailed, step-by-step description of the work to be performed.
- **Risk Assessment:** An analysis of potential negative impacts (e.g., downtime, performance degradation).
- **Rollback Plan:** A clear procedure to revert the change and restore the previous state.
- **Verification Plan:** How the success of the change will be tested and verified.

### 3.2 Step 2: Notify Users & Gain Approval

The Change Request must be submitted for approval to the relevant team lead(s). Once the CR is approved, a notification must be sent to all affected users and stakeholders, clearly communicating the planned change window and any expected service interruptions.

### 3.3 Step 3: Create Jira Issue

After the CR is approved, a new Issue must be created in the appropriate Jira project to track the execution of the work. The Jira issue summary should be clear and concise, and the description must link back to the approved Change Request document.

### 3.4 Step 4: Execute and Document in Real-Time

All work must be performed in a sequential, step-by-step manner. For *every command* executed during the change window, the engineer must post a new comment to the corresponding Jira issue.

Each comment must be formatted as follows:

1.  A brief, human-readable description of the action being taken.
2.  The full, exact command being executed, enclosed in a `{code:bash}` block.
3.  The complete, unedited output from the command, enclosed in a `{code}` block.

**Example Comment:**

> Installing `iptables-persistent` to ensure firewall rules survive reboots.
>
> *Command:*
> {code:bash}
> sudo DEBIAN_FRONTEND=noninteractive apt-get install -y iptables-persistent
> {code}
>
> *Output:*
> {code}
> Reading package lists... Done
> ... (full output) ...
> {code}

### 3.5 Step 5: Close Issue

Once the work is complete and verified according to the Verification Plan, the Jira issue can be transitioned to the "Done" or "Closed" state.

## 4.0 Roles and Responsibilities

- **Engineer:** The individual responsible for drafting the CR and executing the work.
- **Team Lead:** The individual responsible for reviewing and approving the CR.

## 5.0 Emergency Change Procedure

In the event of a critical incident requiring an immediate infrastructure change (an "emergency"), the primary goal is to restore service as quickly and safely as possible.

1.  **Execute:** The engineer may execute the necessary changes immediately.
2.  **Document Retroactively:** Immediately following the resolution of the incident, the engineer must follow this SOP retroactively. This includes:
    - Creating a Jira issue with a summary prefixed by "EMERGENCY FIX:".
    - The issue description must explain the incident and why an emergency change was necessary.
    - All steps taken must be documented as comments on the issue, per the format in Section 3.4.
