# Mise CLI Tool Notes

This document records specific `mise` usage and its purpose, for quick reference.

## Managing Language Runtimes

- **Usage Context:** `mise` must be used from within an activated `flox` environment.
- **Purpose:** To manage language runtimes (e.g., Node.js, Python, Go) within the `gemini-exec-host` environment. This ensures consistent and controlled language environments for development tasks.
- **Example Pattern:** `lxc exec gemini-exec-host -- flox activate -- mise <mise-command>`
