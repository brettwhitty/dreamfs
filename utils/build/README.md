# Build Utilities

This directory contains utility scripts for managing the local build environment.

## Scripts

### `setup_private_workspace.sh` (Linux/macOS)
### `setup_private_workspace.ps1` (Windows)

These scripts automate the initialization of a [Go Workspace](https://go.dev/doc/tutorial/workspaces) to separate local development configuration from upstream version control.

### Technical Purpose

The scripts perform two primary functions:

1.  **Workspace Initialization**: Run `go work init` and `go work use .` to create a `go.work` file. This allows the local Go toolchain to operate in workspace mode, where dependencies can be resolved from local paths (using `use` directives) rather than strictly following `go.mod`.
2.  **Git Configuration**: Appends `go.work` and `go.work.sum` to `.gitignore`. This prevents local path overrides and workspace configurations from being committed to the repository, ensuring `go.mod` retains only upstream-valid dependency paths.

### Usage

**Linux / macOS:**
```bash
./utils/build/setup_private_workspace.sh
```

**Windows (PowerShell):**
```powershell
.\utils\build\setup_private_workspace.ps1
```

### Workflow

Once the workspace is initialized, local dependencies can be overridden using `go work use`.

**Example:**
To develop against a local clone of a dependency:

```bash
go work use ../path/to/dependency
```

This acts as a local `replace` directive. The build system will use the code in the specified directory, but `go.mod` remains unchanged. To revert to the upstream version, remove the entry from `go.work` or delete the `go.work` file.
