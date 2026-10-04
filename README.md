# Minecraft build updater — Go monorepo

Two Go applications live here:

| App | Role | Production runtime |
| --- | --- | --- |
| `apps/file-hosting` | Maps, hashes, and serves build files | Docker Compose |
| `apps/mc-build-updater` | Windows Minecraft mod updater and upload utility | Native Windows executable |

## Prerequisites

- Go 1.26+
- GNU Make
- Docker Desktop (production `file-hosting` only)

Development uses native processes. The first `make dev-*` downloads `air` into Go's module cache and then watches Go source changes.

## Commands

```powershell
make dev-server      # native file-hosting with hot reload at http://localhost:1447
make dev-client      # native updater with hot reload; uses localhost:1447
make dev             # both development commands

make test
make build           # Docker image for server + Windows binaries for client
make start-server    # production file-hosting through Docker Compose
make start-client    # build and run updater executable
make stop
```

## File-hosting data

Files served to clients belong in `apps/file-hosting/files/`. Production Compose mounts this directory into the container at `/data`; it is not copied into the image.

Routes retained from the TypeScript service:

- `GET /map/update`
- `GET /map/version`
- `GET /map`
- `GET /<directory>/<sha1>` or `GET /<directory>/<file-name>`

## Client configuration

At first run, updater creates `mc-mods-updater.config.yml` beside its working directory. Its default branch is `dead-inside-land`.

Production file-hosting URL defaults to `http://mc.sinezix.ru:1447/`. Set `MC_BU_FILE_HOSTING_URL` to override it. `make dev-client` always uses `http://localhost:1447/`.

`mc-bu-utils.exe upload-mods [workers]` uploads missing local mod files over SFTP. Configure it using `MC_BU_SFTP_HOST`, `MC_BU_SFTP_USER`, `MC_BU_SFTP_KEY_PATH`, `MC_BU_SFTP_REMOTE_MODS_PATH`, and `MC_BU_MODS_PATH`.
