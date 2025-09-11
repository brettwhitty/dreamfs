# Flox CLI Tool Notes

This document records specific `flox` commands used and their purposes, for quick reference.

## Searching for Packages

- **Command:** `lxc exec gemini-exec-host -- flox activate -- flox search <keyword>`
- **Purpose:** To search for available software packages within the `gemini-exec-host` environment before installation. This ensures I can identify the correct package name.

## Installing Packages

- **Command:** `lxc exec gemini-exec-host -- flox activate -- flox install <package-name>`
- **Purpose:** To install a specified software package within the `gemini-exec-host` environment. This is the mandatory method for software installation.
