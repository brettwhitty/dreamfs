# Project Directory Structure

This document outlines the purpose and typical contents of each top-level directory in the DreamFS project, adhering to common Go project layout conventions.

-   **`.git/`**:
    *   **Purpose**: Contains all the files that Git needs to manage the version control for the repository. This includes object storage, references, and configuration.
    *   **Contents**: Git internal files (e.g., `objects/`, `refs/`, `config`).

-   **`.vscode/`**:
    *   **Purpose**: Stores configuration files specific to the Visual Studio Code editor for this project.
    *   **Contents**: Editor settings, launch configurations, task definitions (e.g., `settings.json`).

-   **`archive/`**:
    *   **Purpose**: Contains older or experimental versions of code that are kept for reference but are not part of the active development.
    *   **Contents**: Various `.go` files from previous iterations.

-   **`bin/`**:
    *   **Purpose**: Stores compiled **development binaries** for the project. These builds typically include debugging information and are intended for local development and testing.
    *   **Contents**: Executable files (e.g., `dreamfs.exe`, `dreamfs`). This directory is ignored by Git.

-   **`build/`**:
    *   **Purpose**: Contains scripts and configurations related to packaging, continuous integration (CI), and other build automation tasks. It does not typically store compiled binaries.
    *   **Contents**: Build scripts, CI/CD pipeline configurations.

-   **`cmd/`**:
    *   **Purpose**: Contains the source code for the main applications (executables) of the project. Each subdirectory here represents a distinct executable.
    *   **Contents**:
        *   `indexer/`: Source code for the `dreamfs` indexer application (`main.go`).

-   **`dist/`**:
    *   **Purpose**: Stores compiled **distribution binaries**. These builds are typically stripped of debugging information, optimized, and intended for release and distribution.
    *   **Contents**: Stripped executable files (e.g., `dreamfs-linux-amd64`, `dreamfs-windows-amd64.exe`). This directory is ignored by Git.

-   **`pkg/`**:
    *   **Purpose**: Contains library code that can be used by other applications or external projects. It's typically structured into sub-packages based on functionality.
    *   **Contents**:
        *   `config/`: Configuration handling.
        *   `fileprocessor/`: Logic for processing files.
        *   `metadata/`: Data structures and operations for file metadata.
        *   `metrics/`: Metrics collection and reporting.
        *   `network/`: Networking and peer communication.
        *   `storage/`: Database and persistent storage interactions.
        *   `utils/`: General utility functions.

-   **`tools/`**:
    *   **Purpose**: Contains Go programs or scripts that automate various development tasks for the project, but are not part of the main application.
    *   **Contents**:
        *   `build_dist.go`: Script for building distribution binaries.

-   **Other Root Files**:
    *   **`.gitattributes`**: Defines attributes per path.
    *   **`.gitignore`**: Specifies intentionally untracked files to ignore.
    *   **`BUILD.md`**: Documentation for building the project.
    *   **`BW-NOTES.md`**: Brett's personal notes and future improvement ideas.
    *   **`CODE-MERGE-PLAN.md`**: Plan for merging and refactoring code.
    *   **`GEMINI.md`**: Project operating instructions and guidelines for the Gemini agent.
    *   **`go.mod`**: Go module definition file.
    *   **`go.sum`**: Checksums for module dependencies.
    *   **`indexer.db`**: Placeholder or actual database file for the indexer.
    *   **`mise.toml`**: Configuration file for `mise` (a tool for managing development environments).
    *   **`NOTES.md`**: General project notes.
    *   **`README.md`**: Project overview and main documentation.
    *   **`SPECIFICATIONS_AND_IMPLEMENTATION.md`**: Details on specifications and implementation.
    *   **`VERSION.md`**: Project version information.
