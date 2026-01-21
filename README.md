# DreamFS

DreamFS is a command-line tool intended to provide a lightweight,
cross-platform, zero-config distributed datastore for extended
file attributes.

## Why

### Lightweight

- DreamFS supports indexing files across machines of all types
- Can be run as a daemon on desktops, servers, or NAS devices

### Cross-Platform

- DreamFS abstracts away the local filesystem, providing access to a unified view of metadata
- If the system has a terminal, the DreamFS command line functionality is exactly the same
- Optional two-way synchronization of extended attributes across all filesystems that support it

### Command-Line First

- DreamFS provides a simple, hierarchical command structure as used in other popular tools (eg: docker)
- Command-line first allows for incorporation into more complex, user-defined workflows

### Zero-config Distributed Datastore

- When you run the command, or start up a daemon, DreamFS finds its peers and has access to all their data
- File contents can be processed using streaming swarm computes, where appropriate
- Scale out the performance of file indexing as needed by starting as many instances as required

## How

DreamFS is implemented in Golang.

TODO: Write more text here.
