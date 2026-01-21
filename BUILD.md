# Building DreamFS

This document outlines the procedures for building the DreamFS application from source.

## Development Builds (with Debugging)

For development and testing, you can create a build with debugging information. These builds are placed in the `bin/` directory.

From the root of the project directory, run the following command:

```sh
go build -o bin/dreamfs.exe ./cmd/indexer
```

- On Linux or macOS, you may want to change the output name to just `bin/dreamfs`:
  ```sh
  go build -o bin/dreamfs ./cmd/indexer
  ```

This will create a single executable file in the `bin/` directory.

## Distribution Builds (Stripped and Optimized)

For creating stripped and optimized builds suitable for distribution, use the custom build script located at `tools/build_dist.go`. This script handles cross-compilation and injects version information into the executable, and places the output in the `dist/` directory.

### Usage

To use the distribution build script, run it with `go run`. You can specify the target platforms with the `-platforms` flag.

```sh
go run tools/build_dist.go -platforms=linux,darwin,windows
```

### Supported Platforms

- `linux`
- `darwin` (for macOS)
- `windows`

You can provide a comma-separated list of the platforms you wish to build.

The script will generate executables in the `dist/` directory with the naming convention `dreamfs-<os>-<arch>`, for example:

- `dist/dreamfs-linux-amd64`
- `dist/dreamfs-darwin-amd64`
- `dist/dreamfs-windows-amd64.exe`