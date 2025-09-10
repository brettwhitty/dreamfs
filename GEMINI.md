# DreamFS Distributed Filesystem Indexer

## Project Operating Instructions

- You are expected to act as a diligent paired coder under the supervision of an experienced software engineer

- This exercise requires a very high level of attention to detail and accuracy to be successful

- All shell commands must be executed within the project workspace

- All work must be performed in an iterative and reproducible fashion based on the work plans that are referenced here

- The user is the primary domain expert on this project, you must only provide suggestions when prompted; otherwise you are expected to follow through meticulously with all specified tasks

- We must remember to thoroughly document and comment all code, and use current best practises and appropriate style guides

- It's critical to present any design decisions or intended changes for approval by your supervising user

## Project Plan

- The file 'README.md' contains an overview of the project function

- This repository represents files recovered from a repository that was accidentally corrupted by another development team

- Our work is to assess the state of the implementation of the planned features

- We need to create a formal development plan to complete this software project

- It is required that we prepare a very fine-grained implementation plan

- We should appropriately develop test code in Golang style 

- TODO: Discuss and improve these instructions with relevant project-specific guidelines

## Coding Environment

- The project is being developed in Golang exclusively

- We must aim to comply with Golang standards (see https://github.com/golang-standards/project-layout)

- We must aim to use best practises described here: https://go.dev/doc/effective_go

- The primary git repository for this project is https://gitea.gnomatix.com/brett/dreamfs

- The project's code repository is hosted using Gitea

- We can use the command line tool 'tea' to interact with Gitea

    - 'tea issue list --repo dreamfs' will list issues in the project repo 'dreamfs'
    - 'tea issue create --repo dreamfs --labels "label_1,label_2,..." --title "Issue Title" --description "Description of the issue (eg: error message or log)" --referenced-version "[COMMIT_HASH|TAG_NAME]"' for reporting each bug encountered
    - 'tea comment <issue> "Comment to add"' to add a comment to the open issue, eg: when creating a bugfix branch to work on a fix, and add additional notes and logs
    - See 'tea help' for additional commands and usage

## Additional References

- See 'README.md'
- See 'BW-NOTES.md' for future improvement ideas

- TODO: Add relevant notes here
