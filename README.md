# mutter

mutter is a universal natural language interface to your system. Describe what
you want in plain language — mutter finds the right script or tool, extracts
the arguments, and runs it. Everything happens locally. No cloud. No LLM.

---

## Installation

**Shell script (macOS/Linux):**

```bash
curl -o- https://raw.githubusercontent.com/sidisinsane/mutter/main/install.sh | bash
```

**Archive download (macOS/Linux):**

```bash
# Download the release asset and its corresponding SHA256 checksum file
curl -OL https://github.com/sidisinsane/mutter/releases/latest/download/mutter_Darwin_arm64.tar.gz
curl -OL https://github.com/sidisinsane/mutter/releases/latest/download/checksums.txt

# Verify the checksum
shasum -a 256 --check --ignore-missing checksums.txt

# Extract and install
tar -xzf mutter_Darwin_arm64.tar.gz
mv mutter ~/.mutter/
```

**Build from source:**

```bash
git clone https://github.com/sidisinsane/mutter
cd mutter
go build -o mutter-daemon ./cmd/mutter-daemon
```

**Verify the installation:**

```bash
mutter-daemon --help
```

---

## First Run

On first run, mutter downloads two things automatically:

- The onnxruntime shared library (~30MB) to `~/.mutter/lib/`
- The `all-MiniLM-L6-v2` sentence embedding model (~90MB) to `~/.mutter/models/`

Both are downloaded once and reused on every subsequent run.

---

## Getting Started

A mutter **workspace** is any directory containing scripts and a `mutter.yml`
config file. Start the daemon from your workspace root:

```bash
mutter-daemon
```

Then send natural language commands via the API:

```bash
# Find matching scripts
curl -s -X POST http://localhost:8080/api/route \
  -H 'Content-Type: application/json' \
  -d '{"query":"convert the video file"}' | jq .

# Route and execute in one step
curl -s -X POST http://localhost:8080/api/query \
  -H 'Content-Type: application/json' \
  -d '{"query":"print a hello message"}' | jq .
```

---

## Configuration

Every workspace needs a `mutter.yml` (or `mutter.yaml`, `mutter.json`) at its
root. `mutter.yml` is not committed to version control — it is workspace-specific.

**Minimal:**

```yaml
schema_version: "0.1.0"
```

**Full:**

```yaml
schema_version: "0.1.0"

confirmation: false          # prompt before executing
confidence_threshold: 0.75  # minimum similarity score to execute without confirmation

session:
  buffer_size: 2             # unexecuted execution IDs to preserve for recovery

model:
  path: ~/.mutter/models/all-MiniLM-L6-v2.onnx
  dimensions: 384
  library_path: ""           # override onnxruntime library path (auto-detected if empty)

discovery:
  paths:
    - .                      # directories to scan for scripts
  recursive: true

extensions:                  # maps comment delimiter to file extensions
  "#":
    - sh
    - rb
    - py
  "//":
    - go
    - js
    - ts
```

All keys except `schema_version` are optional — omitting a section uses the
application defaults.

---

## Annotating Scripts

mutter discovers scripts via `hashfm` metadata blocks embedded in comments.
Add a block to any script to make it indexable:

```bash
#!/usr/bin/env bash
# ---
# description: Convert video files to different formats using ffmpeg
# usage: convert-video.sh --input {{input}} --output {{output}}
# type:
#   input: [video]
#   output: [video]
# arguments:
#   input:
#     pattern: '(?i)(?:convert|input)\s+(\S+)'
#     description: Input video file path
#   output:
#     pattern: '(?i)(?:to|output)\s+(\S+)'
#     description: Output video file path
# exits:
#   0: success
#   1: ffmpeg not found
#   2: input file not found
# ---
```

The `description` field is what mutter matches against your natural language
query. Keep it concise and imperative.

The `type` field declares what kind of data the script consumes and produces,
used to validate chained commands. Supported categories: `video`, `audio`,
`image`, `text`, `binary`.

---

## Chaining Commands

mutter understands natural language chain expressions and pipes commands together:

```bash
curl -s -X POST http://localhost:8080/api/query \
  -H 'Content-Type: application/json' \
  -d '{"query":"download the video and then convert it to audio"}' | jq .
```

Natural language connectors (`and`, `then`, `and then`) are normalised to pipe
syntax before routing. Each segment is routed independently. Type compatibility
between adjacent commands is validated before execution — a `video` output must
connect to a `video` input.

---

## API

| Endpoint | Method | Description |
|---|---|---|
| `/api/index` | POST | Re-index the workspace |
| `/api/route` | POST | Match a query to scripts without executing |
| `/api/execute` | POST | Execute a script by path |
| `/api/query` | POST | Route and execute in one step |

All responses are JSON. All errors are JSON.
