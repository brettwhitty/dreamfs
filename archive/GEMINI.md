# Recovered Code Fragments

The Golang source files in this directory represent working files recovered from several machines and developers.

The represent proof-of-concept work of different proposed features and package implementations.

## Instructions for Analyzing and Incorporating

- No single one of these files is a working representation of a "final product"
- Any of the files may have useful code fragments or ideal implementations of a particular function
- You must attempt to reconcile and understand the contents of each
- Create a NOTES.md file with useful notes on the contents of each file; reference specific features and line numbers
- Create a file USEFUL-CODE.md where you extract common best implementations and list their sources
    - Briefly explain the code
- Create a checklist file of features; identify conflicting implementations (eg: progressbar vs charmbracelet) for use to review and the user shall reaffirm which modules we are keeping for the current implementation
    - For UI display, we are firmly committed to adopting the charmbracelet tools
    - Any code with robust UI impelentations that is using charmbracelet should be carried forward

## Sub-directories

- There are two subdirectories, '.bak' and 
