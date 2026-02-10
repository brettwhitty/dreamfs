---
type: MANUAL
authority: Brett Whitty
review_status: APPROVED
version: 0.1.1
approved_versions: 0.1.*
generated_on: 2026-01-22 17:03
origin_persona: Brett Whitty
origin_session: d346cd76-699f-4ab0-b24a-53180588cb07
intent: Define the core identity, vision, and technical pillars of the DreamFS project.
primary_sources: [README.md]
release_path: README.md
related_issues: []
related_sops: []
tags: [readme, product, landing]
---

# DreamFS: Distributed Datastore for Extended File Attributes

![DreamFS Logo](assets/images/dreamfs-logo-vortex.png)

DreamFS is a lightweight, cross-platform, zero-config distributed datastore for extended file attributes. It provides a unified view of metadata across your entire digital swarm—from Linux servers and NAS devices to Windows desktops and beyond.

---

## 🌩️ The Swarm Vision

!["I have a dream, that one day, I will know exactly where all my files are."](assets/images/dreamfs-vision-metropolis.png)

DreamFS is designed for the modern, fragmented data landscape. It doesn't just index files; it creates a living, breathing distributed index that finds its peers automatically and replicates metadata across the swarm.

- **Lightweight**: Run it as a daemon on anything from a Raspberry Pi to a production cluster.
- **Zero-Config**: Built-in mDNS and HTTP-based discovery—just start it and watch the swarm grow.
- **Content-First**: Files are judged by the content of their sectors, not the case of their strings.

---

## 🐧🍎🪟 Cross-Platform Integrity

!["Same same, but different."](assets/images/dreamfs-platform-unity.png)

Whether you're on Windows, macOS, or Linux, DreamFS abstracts away filesystem quirks to provide a consistent canonical view. It intelligently handles UNC paths, network mounts (NFS/CIFS), and case-sensitivity differences to ensure physical uniqueness is preserved across the wire.

---

## 🛠️ Features

- **Blazing Fast Fingerprinting**: Uses `BLAKE3` with an intelligent sampling strategy for large files.
- **P2P Replication**: Powered by `hashicorp/memberlist` for robust, decentralized metadata propagation.
- **Canonicalization**: Intelligent path mapping for cross-platform metadata consistency.
- **Local-First**: Your data stays on your nodes, governed by the rules of the swarm.

---

## 🚀 Getting Started

### Prerequisites

- [Go](https://go.dev/dl/) 1.25.1+
- [mise](https://mise.jdx.dev/) (recommended for environment management)

### Installation

```bash
# Clone the repository
git clone https://gitea.gnomatix.com/brett/dreamfs.git
cd dreamfs

# Build the indexer
go build -o indexer cmd/indexer/main.go
```

### Usage

**Initialize an Index:**

```bash
./indexer index /path/to/your/data
```

**Start a Swarm Node:**

```bash
./indexer serve --swarm --addr :8080
```

**Monitor the Swarm:**

```bash
./indexer monitor --swarm
```

---

## 🧠 Philosophy

!["And they will be judged, not by the cases of their strings, but by the content of their sectors"](assets/images/dreamfs-philosophy-content.png)

DreamFS is built on the principle that metadata should be as portable as the ideas it represents. We prioritize **physical uniqueness** over path-based indexing, ensuring that your data remains yours, reachable and verifiable, no matter which platform it currently calls home.

---

## 📝 License

`DreamFS` is property of GNOMATIX. All rights reserved.

![GNOMATIX "TEAM"](assets/images/gnomatix-killbots-activate-xs.png)
![GNOMATIX LOGO](assets/images/gnomatix-new-xs.png "GNOMATIX")

---

\*DreamFS is a user-dictated, AI-hallucinated & "vibe"-coded fantabulation. It does not currently exist.
\*\*DreamFS has been granted provisional approval by the FDA under emergency use authorization; as a consequence, GNOMATIX has been granted 90-year, zero-fault protection against any claims of data loss, blindness, infertility, or loss of tangible presence in physical reality; contact your local state representative for more information
