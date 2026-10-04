# Minecraft build updater — Go monorepo

Two Go applications live here:

| App | Role | Production runtime |
| --- | --- | --- |
| `apps/file-hosting` | Maps, hashes, and serves build files | Native binary or Docker Compose |
| `apps/mc-build-updater` | Windows Minecraft mod updater and upload utility | Native Windows executable |

## Prerequisites

- Go 1.26+
- GNU Make
- Docker Desktop (only for manual `file-hosting` Compose deployment)

Development uses native processes. The first `make dev-*` downloads `air` into Go's module cache and then watches Go source changes.

## Commands

```powershell
make dev-s           # native file-hosting with hot reload at http://localhost:1447
make dev-c           # native updater with hot reload; uses localhost:1447

make test
make build           # native server and Windows client binaries in ./build/
make start-s         # build and run file-hosting natively
make start-c         # build and run updater executable
make start-s-dev     # native file-hosting without Air (same options as start-s)
make start-c-dev     # native updater with --dev, without Air
```

## File-hosting data

Files served to clients belong in `apps/file-hosting/files/`. Manual production Compose mounts this directory into the container at `/data`; it is not copied into the image.

Routes retained from the TypeScript service:

- `GET /map/update`
- `GET /map/version`
- `GET /map`
- `GET /<directory>/<sha1>` or `GET /<directory>/<file-name>`

## Client configuration

At first run, updater creates `mc-mods-updater.config.yml` beside its working directory. Its default branch is `dead-inside-land`.

Production file-hosting URL defaults to `http://mc.sinezix.ru:1447/`. Use `--file-hosting-url` to override it. `make dev-c` and `--dev` always use `http://localhost:1447/`, ignoring `--file-hosting-url`.

## Client releases

Push a stable tag `vX.Y.Z` to both remotes. GitHub Actions and GitLab CI independently build `windows/amd64` `mc-build-updater.exe`, generate `checksums.txt`, and publish releases. The installed client checks GitHub first, then GitLab when GitHub cannot supply a valid release.

`mc-bu-utils.exe upload-mods` publishes missing local mods through REST. Use `--dev` for localhost; it still requires `FILE_HOSTING_TOKEN` configured on local server.
