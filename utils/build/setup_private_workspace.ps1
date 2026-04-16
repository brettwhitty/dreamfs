<#
.SYNOPSIS
    Initializes a local Go Workspace (go.work) to facilitate local dependency
    overrides without modifying the shared go.mod file.

.DESCRIPTION
    1. Initializes `go.work` if it does not exist, adding the current module.
    2. Updates `.gitignore` to ensure `go.work` and `go.work.sum` are ignored.

.EXAMPLE
    .\utils\build\setup_private_workspace.ps1
#>

$ErrorActionPreference = "Stop"

Write-Host "Initializing local Go workspace configuration..."

# 1. Initialize go.work if it doesn't exist
if (-not (Test-Path "go.work")) {
    go work init
    go work use .
    Write-Host "Created go.work and added current directory."
} else {
    Write-Host "go.work already exists. Skipping initialization."
}

# 2. Update .gitignore
$GitIgnoreFile = ".gitignore"

if (-not (Test-Path $GitIgnoreFile)) {
    New-Item -Path $GitIgnoreFile -ItemType File | Out-Null
}

function Add-ToIgnore {
    param (
        [string]$Pattern
    )
    
    $Content = Get-Content -Path $GitIgnoreFile -ErrorAction SilentlyContinue
    if ($null -eq $Content -or $Content -notcontains $Pattern) {
        Add-Content -Path $GitIgnoreFile -Value $Pattern
        Write-Host "Added $Pattern to .gitignore"
    }
}

Add-ToIgnore "go.work"
Add-ToIgnore "go.work.sum"

Write-Host "Workspace setup complete."
