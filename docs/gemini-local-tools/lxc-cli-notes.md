# LXC CLI Tool Notes

This document records specific `lxc` commands used and their purposes, for quick reference.

## Executing Commands in `gemini-exec-host`

- **Command:** `lxc exec gemini-exec-host -- flox activate -- <COMMAND>`
- **Purpose:** To execute a specific command within the `gemini-exec-host` LXD container, ensuring it runs in a `flox` activated environment. This is the primary method for me to interact with the development virtual host.

## Listing LXD Containers

- **Command:** `lxc list`
- **Purpose:** To view a list of all LXD containers and their current status, which is useful for verifying the `gemini-exec-host` is running.
