# Git CLI Tool Notes

This document records specific `git` commands used and their purposes, for quick reference.

## Checking Repository Status

- **Command:** `git status`
- **Purpose:** To display the state of the working directory and the staging area, showing which changes have been staged, unstaged, or untracked. This helps in understanding the current state of the repository before making changes or committing.

## Reviewing Changes

- **Command:** `git diff HEAD`
- **Purpose:** To show changes between the working tree and the last commit. This is useful for reviewing all modifications before staging or committing.

- **Command:** `git diff --staged`
- **Purpose:** To show changes between the staging area and the last commit. This is useful for reviewing changes that are prepared for the next commit.

## Reviewing Commit History

- **Command:** `git log -n 3`
- **Purpose:** To display the last 3 commit messages and their details. This helps in understanding recent changes and matching commit style.

## Staging Changes

- **Command:** `git add <file_path>` or `git add .`
- **Purpose:** To add changes from the working directory to the staging area, preparing them for the next commit.

## Committing Changes

- **Command:** `git commit -m "<commit_message>"`
- **Purpose:** To record staged changes to the repository with a descriptive message.
