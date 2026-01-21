# Git-Crypt Documentation

DreamFS uses [git-crypt](https://github.com/AGWA/git-crypt) to protect sensitive legacy code and internal agent instructions. This ensures that while the repository can be mirrored publicly, sensitive "archive" and "instruction" content remains encrypted and inaccessible without the proper GPG keys.

## Protected Paths

The following directories are currently encrypted:
- `archive/**`: Legacy codebase fragments and recovered files.
- `docs/gemini-instructions/**`: Internal AI operational protocols.
- `docs/gemini-local-tools/**`: CLI tool notes and integration details.

Protection rules are defined in the root [.gitattributes](file:///c:/Users/brett/repos/dreamfs/.gitattributes).

## Unlocking the Repository

If you have been added as a collaborator (GPG user), you can unlock the repository after cloning by running:

```bash
git-crypt unlock
```

If you have a symmetric key file instead of a GPG setup:

```bash
git-crypt unlock /path/to/keyfile
```

## Adding Collaborators (GPG)

To add a new collaborator who has a GPG key:

1. **Import their public key**:
   ```bash
   gpg --import collaborator_public_key.asc
   ```

2. **Grant trust** (optional, prevents interactive prompts):
   ```bash
   gpg --edit-key <KEY_ID>
   # Type 'trust', select '5' (ultimate), and 'quit'
   ```

3. **Add them to git-crypt**:
   ```bash
   git-crypt add-gpg-user <KEY_ID>
   ```

4. **Commit the changes**:
   `git-crypt` will automatically create a commit with the new encrypted key metadata.

## Best Practices

- **Check Status**: Use `git-crypt status -e` to verify which files are encrypted.
- **Locking**: You can re-lock the repository with `git-crypt lock` if you want to verify that files are indeed unreadable in their encrypted state.
- **Key Safety**: Never commit the symmetric key itself. Only commit the GPG-encrypted keys generated in the `.git-crypt/` directory.
