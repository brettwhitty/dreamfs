# Desirable New Features

## After Filesystems Are Indexed

### "Virtual Filesystem" Based on Search Results

- CLI allows a (distributed) search of the DB

    - Since DBs are synced between peers, maybe we can support streaming distributed searching (if that helps with performance --- maybe search will already be fast)

- Treat the list of file metadata results returned from a search as if it's the output of an 'ls' command

- Allow assigning a name to it; this is stored to another local data store

    - Whatever type of data store is appropriate here; could use the existing DB, but maybe we want a graph db
    
- This will pull in files from all hosts, including potential "duplicates"

- It would be great if we could support Fuse-mounting these virtual filesystems and supporting file operations on them; creating virtual sub-directories based on metadata tags, etc.

- Once user is satisfied with a virtual view, it can be made into a materialized view by either physically copying or moving files to a specific target location.

    - This would be seamless across hosts, and the software would need to implement an efficient P2P copy tool (like croc, if we can use the libraries, for encrypted; or something like an scp or rsync) --- we need to maintain all the timestamps, file attributes, sparse files, etc. that we can (so maybe use an existing package for this)
    
    - Again, this needs to be cross-platform and performant
    
    - Bittorrent could be used if a file with duplicates on remote hosts was being copied to a local host; 2 or more peers could be senders; this could be more complicated than it needs to be
       
- If we could mount the virtual filesystems and then even just support normal copy and moves from inside to other local volumes by simply translating the 'cp' to an 'scp' or 'rsync', that would be a very simple way to implement it.

    - Since we have a requirement for being cross-platform compilable and Golang suffiencient, we'd need to use existing Golang packages.

    - Perhaps something like IPFS could be used temporarily if that doesn't require extra storage space or performance hits --- which I think it would

### File History and Provenance

- When was a file first indexed

- Record of it being moved

    - Old UUIDs are deprecated and their value contains an obsoleted flag, timestamp, and operation that caused them to be deprecated
    - If moved or copied by the tool, those events do the update; otherwise 'missing' if not found during search or reindex.

- Is it a member of a virtual filesystem


### Extended Attribute Retension and Sync

- The system should support the ability to add indexed metadata as extended attributes to the local files where supported by the filesystem. This should not involved modification of the files themselves

    - Windows NTFS: Use alternative data streams to encode the properties
    - Linux ext4, etc.: xattrs
    - ZFS: set properties
    - macOS: xattrs?

- Ideally, the user could configure which properties to keep in sync.

- When a file is moved to a different filesystem and/or host and the extended attributes are lost, the running indexer daemon will restore them (if configured) based on the fingerprint of the file

### Final Notes

- Basically the use case is a "locate and gather" kind of functionality.

- It is for large-scale "curation" of distributed storage spaces with radically disorganized structure.

- It needs to provide the tools for recording where the files are, what they are, and then an easy way to get them under control.

- Basic file metadata is the first step. Simple tagging support of all files across all operating systems and filesystems would be extremely useful.

