# Telegram Second Brain

A private Telegram assistant for your projects. The bot uses a local Ollama model, searches explicitly configured local repository clones, and stores conversation history and notes in a local SQLite database. It does not connect to GitHub, execute commands, or modify project files.

## Requirements

- Go 1.19 or later, or a built binary.
- A Telegram bot token and your Telegram numeric user ID.
- Ollama running on the bot host, with a model already available that fits the host's hardware.

Local inference has no per-token API charge, but requires a machine with enough RAM/VRAM for your chosen model and only works while that machine, Ollama, and the network are running. It does not by itself guarantee 24/7 uptime.

## Configure

1. Copy `env.example` to `.env`. Set `TELEGRAM_TOKEN`, `TELEGRAM_ID`, and `OLLAMA_MODEL`. `TELEGRAM_ID` is required; the bot refuses to start without an allowlisted user. `OLLAMA_URL` defaults to `http://localhost:11434`.
2. Install and start Ollama yourself. Choose a model appropriate for your hardware, make it available in Ollama, and use the exact model name for `OLLAMA_MODEL`. This repository does not download models or start services automatically.
3. Copy `projects.example.json` to `projects.json` and replace the example paths with the local clone directories you want the assistant to read. The file may be left absent or empty when no project context is configured.
4. Run `go run .` or build the binary with `go build`.

The project registry is an explicit name-to-directory allowlist. Search is read-only and limited to common text/documentation/source files; it skips `.git`, generated/dependency directories, database files, and filenames associated with secrets or credentials. The model client accepts only loopback URLs or Docker Desktop's `host.docker.internal` and disables proxy use. Matching excerpts and saved notes are sent to that local Ollama service.

## Telegram commands

- `/projects` lists configured repositories.
- `/project <name>` selects the repository whose local text files are searched for each question; `/project clear` deselects it.
- `/remember <note>` saves a private note for the selected project, or globally if no project is selected.
- `/memory` lists notes for the current project and global notes.
- `/forget <note-id>` explicitly deletes one of your saved notes.
- `/reload` explicitly clears the saved conversation for this Telegram chat; notes are retained.
- `/help` shows command guidance.

Only allowlisted users in private Telegram chats can use the assistant. Conversations, project selections, and notes are stored as plaintext in a local SQLite database under `DATA_DIR` (default `./data/assistant.db`). The data directory is created with owner-only permissions where supported by the host; protect it with your OS account, and back up or remove it yourself when appropriate.

## Docker

Build the image, then mount `.env`, `projects.json`, and a persistent data directory. When Ollama runs on the same Windows host with Docker Desktop, set `OLLAMA_URL=http://host.docker.internal:11434` in `.env`.

```powershell
docker build -t telegram-second-brain .
docker run -d --restart unless-stopped --name telegram-second-brain `
  --env-file .env `
  -v "${PWD}\projects.json:/app/projects.json:ro" `
  -v "${PWD}\data:/app/data" `
  telegram-second-brain
```

`--restart unless-stopped` restarts the container after process or host restarts; continuous availability still depends on the host, Ollama, Telegram connectivity, and power. The Docker configuration is provided for the user to run; this project session does not deploy it.

## Private configuration

Do not commit `.env`, `projects.json`, or `data/`. The repository includes example configuration only. A Telegram token is required; GitHub authorization and hosted-model credentials are not used by this MVP.
