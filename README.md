# Shopping List

This program was written by Claude Code and thus uncopyrightable. It is hereby placed in the public domain/

A self-hosted shared shopping list with passkey authentication and automatic
grocery section classification via a local LLM.

## Features

- **Shared list** — all authenticated users collaborate on a single live list
- **Grocery sections** — items are grouped by aisle (Produce, Dairy & Eggs,
  Frozen Foods, …); sections with no items are hidden
- **Auto-classification** — when you add an item without choosing a section,
  a local Gemma 4B model classifies it asynchronously; the UI updates in real
  time via Server-Sent Events
- **Manual section override** — pick a section yourself when adding an item;
  the choice is remembered for future adds of the same item
- **Custom sections** — add new grocery sections from within the UI
- **Passkey authentication** — no passwords; sign in with Touch ID, Face ID,
  or a hardware security key (WebAuthn/FIDO2)
- **Invite-only registration** — any member can invite someone by email; the
  invitee receives a one-time link to enrol their passkey
- **Single binary** — HTML, JS, CSS, and SQL migrations are all embedded;
  only a PostgreSQL database and (optionally) a GGUF model file are needed at
  runtime

## Requirements

- Go 1.22+
- PostgreSQL 14+
- `gcc`/`g++` (only for the LLM build variant)
- `wget` or `curl` (for `make download-model`)

## Quick start

```sh
# 1. Clone and build (clones and compiles llama.cpp on first run)
git clone https://github.com/yourorg/shopping
cd shopping
make

# 2. Create the database
createdb shopping

# 3. Configure
export DATABASE_URL="postgres://localhost/shopping?sslmode=disable"
export WEBAUTHN_RPID="localhost"
export WEBAUTHN_ORIGIN="http://localhost:8080"
export SERVER_ADDR=":8080"

# 4. Run (migrations are applied automatically on startup)
./bin/shopping-server
```

Open http://localhost:8080 in your browser. On first run there are no users;
use the invite flow below to create the first account.

## Creating the first user

Set `BOOTSTRAP_EMAIL` in your `.env` (or environment) before the first start:

```sh
BOOTSTRAP_EMAIL=you@example.com
```

On startup, if no users exist, the server prints a registration link:

```
*** First-run bootstrap — register at: http://localhost:8080/invite/<token> ***
```

Open that URL to enrol your passkey. The link expires after 48 hours. Once the
first account exists the message no longer appears; remove `BOOTSTRAP_EMAIL`
from your config. After that, use the **Invite** button in the UI to add other
users.

## LLM classification (optional)

Without the LLM, new items are placed in **Other** immediately. To enable
automatic classification:

```sh
# 1. Download the model (~2.5 GB, no HuggingFace account required)
make download-model
# => saves to models/google_gemma-4-E4B-it-Q4_K_M.gguf

# 2. Build with LLM support (compiles llama.cpp via CGo)
make build-llama

# 3. Point the server at the model
export LLAMA_MODEL_PATH=models/google_gemma-4-E4B-it-Q4_K_M.gguf
./bin/shopping-server
```

`make build-llama` will clone and compile the llama.cpp C library
automatically on the first run. Subsequent builds skip this step if
`libbinding.a` is already present.

## Configuration

All configuration is via environment variables.

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | yes | — | PostgreSQL connection string |
| `WEBAUTHN_RPID` | yes | — | Bare domain, e.g. `shopping.example.com` |
| `WEBAUTHN_ORIGIN` | yes | — | Full origin, e.g. `https://shopping.example.com` |
| `SERVER_ADDR` | no | `:8080` | Address to listen on |
| `LLAMA_MODEL_PATH` | no | — | Path to GGUF model file; omit to disable LLM |
| `SMTP_HOST` | no | `localhost` | SMTP server hostname |
| `SMTP_PORT` | no | `587` | SMTP server port |
| `SMTP_USER` | no | — | SMTP username |
| `SMTP_PASS` | no | — | SMTP password |
| `SMTP_FROM` | no | `noreply@localhost` | From address for invite emails |

## Makefile targets

| Target | Description |
|---|---|
| `make` / `make all` | Build with LLM support — clones and compiles llama.cpp if needed (requires gcc/g++) |
| `make build-stub` | Build without LLM (stub classifier, no CGo required) |
| `make download-model` | Download Gemma 4 4B Q4_K_M GGUF into `models/` |
| `make run` | Build and run with LLM |
| `make run-stub` | Build and run without LLM |
| `make tidy` | Run `go mod tidy` |
| `make clean` | Remove `bin/`, the llama.cpp clone, and the Go build cache |

## Deployment

The server is a single statically-linked binary (in the default non-LLM
build). A minimal production setup:

```sh
DATABASE_URL="postgres://user:pass@db/shopping" \
WEBAUTHN_RPID="shopping.example.com" \
WEBAUTHN_ORIGIN="https://shopping.example.com" \
SMTP_HOST="smtp.example.com" \
SMTP_USER="..." \
SMTP_PASS="..." \
./bin/shopping-server
```

Put it behind nginx or Caddy for TLS. WebAuthn requires HTTPS in production
(`Secure` cookies are set automatically when `WEBAUTHN_ORIGIN` starts with
`https://`).
