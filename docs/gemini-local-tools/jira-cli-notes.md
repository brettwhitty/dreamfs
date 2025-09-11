# Jira CLI Tool Notes

This document summarizes the key learnings and common usage patterns for the `jira` command-line interface tool.

## Basic Usage
The `jira` CLI tool follows a `jira <command> <subcommand> [flags]` structure.

## Getting Help
To get help for any command or subcommand, use the `--help` flag:
- `jira --help`
- `jira <command> --help`
- `jira <command> <subcommand> --help`

## Descriptions

When creating issues, epics, or subtasks, it is crucial to provide an informative and concise description. This can be done using the `--body` flag for direct input, or the `--template` flag to read from a file for longer descriptions.

- `--body "Your description here"`
- `--template /path/to/description.txt`

## Default Values via Environment Variables

For common fields like reporter and assignee, the `jira` CLI can utilize environment variables. You will set `JIRA_REPORTER` and `JIRA_ASSIGNEE` environment variables (e.g., in your `.mise.toml` file).

When creating new issues, epics, or subtasks, I will construct the `jira` CLI commands to explicitly reference these environment variables for the respective fields. This means the commands I generate will look something like:

```bash
jira issue create --reporter "$JIRA_REPORTER" --assignee "$JIRA_ASSIGNEE" ...
```

This approach ensures that the values you set in your environment are used, and it makes it clear that the `jira` CLI command itself is being instructed to use those specific environment variables.

- `JIRA_REPORTER`: Used as the default reporter for new issues.
- `JIRA_ASSIGNEE`: Used as the default assignee for new issues.



## Listing Epics
To list epics in a non-interactive CSV format:
```bash
jira epic list --table --plain --csv
```
This command combines `--table` for a table view, `--plain` for plain mode output, and `--csv` for CSV formatting.

## Listing Issues
To list issues in a non-interactive CSV format:
```bash
jira issue list --plain --csv
```
This command uses `--plain` for plain mode output and `--csv` for CSV formatting.

### Filtering Issues
Issues can be filtered by various criteria using flags. For example, to list issues with "To Do" or "In Progress" status:
```bash
jira issue list --plain --csv --status "To Do" --status "In Progress"
```

## Viewing Issue Details
To view the detailed contents of a specific issue, use the `view` subcommand followed by the issue key:
```bash
jira issue view <ISSUE-KEY>
```
Example:
```bash
jira issue view PD-7
```

## Creating Issues

- **Command:** `jira issue create --summary "<Issue Summary>" --type "<Issue Type>" --no-input`
- **Purpose:** To create a new Jira issue in a non-interactive mode. Both `--summary` and `--type` flags are mandatory when using `--no-input`.
- **Important: To ensure the reporter is set correctly, always include `--reporter "$JIRA_REPORTER"` in the command.** The assignee will default to the value set in `JIRA_ASSIGNEE` environment variable, unless explicitly overridden.
- It is highly recommended to include a detailed description using the `--body` or `--template` flag.
- **Example:** `jira issue create --summary "Review new feature" --type Task --no-input`

## Editing Issues

The `jira issue edit` command allows modification of various issue fields.

- **Command:** `jira issue edit <ISSUE-KEY> [flags]`
- **Note on Reporter:** The reporter field cannot be directly edited using this command. It is set at issue creation. If the reporter is incorrect, the issue may need to be recreated.
- **Example:** `jira issue edit PD-1 --summary "Updated summary" --assignee "user@example.com"`

## Creating Epics

- **Command:** `jira epic create --summary "<Epic Summary>" --name "<Epic Name>" --no-input`
- **Purpose:** To create a new Jira epic in a non-interactive mode. Both `--summary` and `--name` flags are mandatory when using `--no-input`.
The reporter and assignee will default to the values set in `JIRA_REPORTER` and `JIRA_ASSIGNEE` environment variables, respectively, unless explicitly overridden.
- It is highly recommended to include a detailed description using the `--body` or `--template` flag.
- **Example:** `jira epic create --summary "Training Gemini Assistant" --name "Training Gemini Assistant" --no-input`

## Adding Comments to Issues

To add a comment to a Jira issue, use the `jira issue comment add` command.

### Syntax:
```bash
jira issue comment add <ISSUE-KEY> <COMMENT_BODY> [flags]
```

### Handling Special Characters and Non-Interactive Use:

For comments containing special characters (e.g., `&`, `<`, `>`, `"`, `'`, newlines) or when running in a non-interactive environment, it is recommended to:

### Using Jira Markup in Comments

Jira supports its own markup language for rich text formatting within comments. You can use this markup to format text (e.g., bold, italic), create lists, and more. The `jira` CLI will pass this markup to Jira, and it will be rendered correctly in the Jira interface and when viewing issues via the CLI.

Example of common Jira markup:
- `*bold text*` for bold
- `_italic text_` for italic
- `- List item` for bullet points


1.  **Use `$'...'` for shell quoting:** This allows for proper interpretation of escape sequences and special characters directly in the command line.
2.  **Include the `--no-input` flag:** This flag prevents the `jira` CLI from prompting for interactive confirmation, which can cause the command to "hang" in automated scripts or non-interactive sessions.

**Example:**
```bash
jira issue comment add PD-27 $'This is a test comment with special characters: & < > " \' \n New line.' --no-input
```

Alternatively, for very complex comments, you can read the comment body from a file using the `--template` flag:
```bash
jira issue comment add PD-27 --template /path/to/your_comment_file.txt
```
Or from standard input:
```bash
echo "Your comment here" | jira issue comment add PD-27

```

## Transitioning/Closing Issues

The `jira issue move` command is used to transition an issue to a different state, effectively closing or resolving it.

- **Command:** `jira issue move <ISSUE-KEY> <STATE>`
- **Purpose:** To change the status of an issue (e.g., to "Done", "Closed", "In Progress").
- **Example:** `jira issue move PD-29 Done`