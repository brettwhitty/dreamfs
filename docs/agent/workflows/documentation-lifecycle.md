---
description: Formalizes the creation, maintenance, and promotion of project documentation using the "Wiki-First" approach.
---

### Phase 1: Instantiation (Wiki-First)
1. **Target**: All new documentation (Working Drafts, SOPs, Reports) must be created in the **Gitea Wiki** repository first.
2. **Standard Header**: Every file must start with the **Standardized AI Draft Header** (YAML frontmatter) as defined in `docs/modes/office.md`.
   - `type`: `WORKING-DRAFT (LLM-GENERATED)` | `SOP` | `REPORT` | `MANUAL`
   - `authority`: `PENDING MANUAL REVIEW`
   - `review_status`: `PENDING`
   - `generated_on`: Current timestamp
   - `origin_persona`: Current agent persona (e.g., `Rory Devlin (R&D Team Lead)`)
   - `origin_session`: Current conversation ID
   - `intent`: Brief description of why this document was created.
   - `release_path`: The intended final path in the main repository (e.g., `docs/architecture/overview.md`).
3. **Draft Location**: Save the file in the cloned `wiki/` directory within the project workspace.
4. **Synchronization**: Commit and push the new draft to the Wiki's `main` branch.

### Phase 2: Maintenance
1. **Iterative Updates**: When revising a draft based on user feedback, update the following header fields:
   - `review_status`: Set to `REQUIRES_REVISION` if feedback is given.
   - `generated_on`: Update to the latest revision time.
   - `origin_session`: Update to the current session ID.
2. **Context Retention**: Ensure `primary_sources` and `tags` accurately reflect the current state of the document's logic and references.

### Phase 3: Promotion (Release Process)
1. **Approval**: Once the user provides final approval, update the metadata:
   - `review_status`: Set to `APPROVED`.
   - `authority`: Set to the approving user's name.
2. **Movement**: Copy the file from the `wiki/` directory to its `release_path` in the main repository.
3. **Commit**: 
   - Commit the deletion/status update in the Wiki repository.
   - Commit the new/updated file in the main repository.
4. **Cleanup**: Remote working draft references from the local project root if any remain.
