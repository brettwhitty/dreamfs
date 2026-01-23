#!/bin/bash
set -e

################################################################################
# setup_private_workspace.sh
#
# PURPOSE:
#   Initializes a local Go Workspace (go.work) to facilitate local dependency
#   overrides without modifying the shared go.mod file.
#
# DESCRIPTION:
#   1. Initializes `go.work` if it does not exist, adding the current module.
#   2. Updates `.gitignore` to ensure `go.work` and `go.work.sum` are ignored.
#
# USAGE:
#   ./utils/build/setup_private_workspace.sh
################################################################################

echo "Initializing local Go workspace configuration..."

# 1. Initialize go.work if it doesn't exist
if [ ! -f "go.work" ]; then
    go work init
    go work use .
    echo "Created go.work and added current directory."
else
    echo "go.work already exists. Skipping initialization."
fi

# 2. Update .gitignore
GITIGNORE=".gitignore"
touch "$GITIGNORE"

add_to_ignore() {
    if ! grep -q "^$1$" "$GITIGNORE"; then
        echo "$1" >> "$GITIGNORE"
        echo "Added $1 to .gitignore"
    fi
}

add_to_ignore "go.work"
add_to_ignore "go.work.sum"

echo "Workspace setup complete."
