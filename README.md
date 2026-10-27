# valheim-mod-pusher

[![ci](https://github.com/rgeraskin/valheim-mod-pusher/actions/workflows/ci.yml/badge.svg)](https://github.com/rgeraskin/valheim-mod-pusher/actions/workflows/ci.yml)

A small cross-platform desktop app that pushes your local Valheim mods folder to a dedicated server over ssh and then runs a follow-up command there, e.g. restarts the server if nobody is playing.

Tested with [valheim-server-docker](https://github.com/community-valheim-tools/valheim-server-docker), and its defaults are built in: see [Defaults: valheim-server-docker](#defaults-valheim-server-docker).

## Overview

Keeping a modded dedicated server in sync with the mod manager profile you play with is tedious: zip, upload, unpack, swap folders, restart, and don't restart while friends are online. **Valheim Mod Pusher** does it in one click: point it at the profile's `BepInEx/plugins` folder (e.g. from [Gale](https://github.com/Kesomannen/gale) or r2modman), at the server's plugins folder, press **Sync**.

> **It is actually a general-purpose app.** "Valheim", "mods" and "plugins" are only wording in the UI and defaults. Under the hood it pushes *any* local folder to *any* folder on a server over ssh and then runs *any* shell command there. Nothing depends on Valheim, BepInEx, Gale or a particular server image: the default follow-up command is just pre-filled text you can change or switch off.

## Features

- 🖥️ **Cross-platform GUI** - Windows, macOS and Linux, a single binary each
- 🔑 **Uses your ssh setup** - runs the system `ssh`, so `~/.ssh/config`, agents (1Password, Strongbox, ...) and `known_hosts` just work
- 🛡️ **Safe swap** - the remote folder is replaced as a whole, the previous one is kept as `<dir>.bak`
- 🐳 **valheim-server-docker defaults** - server path and follow-up command work out of the box with its README setup
- 🔁 **Follow-up command** - editable or switched off; by default applies the mods and restarts the server only when no players are online
- 🧾 **Host key prompt** - for a server this machine has never seen, shows the key fingerprints and asks before trusting it
- 📊 **Progress and live log** - packing/upload progress and the remote output as it happens
- 💾 **Remembers settings** between runs

## Requirements

- OpenSSH client (`ssh`, `ssh-keyscan`): built into Windows 10/11, macOS and most Linux distributions
- Key-based ssh access to the server (the app never asks for passwords)
- A Unix-like server: `sh`, `tar`, `mv`, `rm`
- Linux desktop: OpenGL and X11 or Wayland libraries (present on any desktop install)
- Go 1.26 or later and a C compiler (for building from source; the GUI toolkit uses cgo)

## Installation

### Homebrew (macOS)

```sh
brew install --cask rgeraskin/homebrew/valheim-mod-pusher
```

Installs **Valheim Mod Pusher.app** into `/Applications` (plus a `valheim-mod-pusher` command). The app is not signed with an Apple Developer ID, so the cask removes the quarantine attribute after install.

### Windows and Linux

Download the archive for your platform from the [latest release](https://github.com/rgeraskin/valheim-mod-pusher/releases/latest), unpack it and run `valheim-mod-pusher.exe` / `valheim-mod-pusher`.

### From Source

```bash
git clone https://github.com/rgeraskin/valheim-mod-pusher.git
cd valheim-mod-pusher
go build -o valheim-mod-pusher .
```

On Linux install the GUI headers first, e.g. on Debian/Ubuntu: `sudo apt install libgl1-mesa-dev xorg-dev libxkbcommon-dev libwayland-dev`.

## Usage

| Field | Meaning |
|---|---|
| **Local plugins dir** | Folder to push, e.g. a Gale profile: `%APPDATA%\com.kesomannen.gale\valheim\profiles\<name>\BepInEx\plugins` on Windows, `~/.local/share/com.kesomannen.gale/valheim/profiles/<name>/BepInEx/plugins` on Linux, or BepInEx in the Steam game folder on macOS: `~/Library/Application Support/Steam/steamapps/common/Valheim/BepInEx/plugins` |
| **Server**, **Port** | Server address (or an alias from `~/.ssh/config`) and ssh port |
| **SSH user** | Optional; defaults to your ssh config / local user name |
| **Server plugins dir** | Path on the server, absolute or `~/...`. Default: `~/valheim-server/config/bepinex/plugins` (valheim-server-docker) |
| **Follow-up** | Shell command run on the server after a successful sync, on by default. Default: apply the mods in valheim-server-docker and restart it if no players are online |

- **Test connection** checks ssh access, whether the server folder exists and whether the ssh user may write there (see [Permissions](#permissions)).
- **Sync** asks for confirmation, uploads the folder, swaps it in (keeping `<dir>.bak`) and runs the follow-up command.

### Defaults: valheim-server-docker

The app was tested with [valheim-server-docker](https://github.com/community-valheim-tools/valheim-server-docker) (`BEPINEX=true`), and its defaults assume that image run as in its README: container `valheim-server`, volumes `$HOME/valheim-server/config:/config` and `$HOME/valheim-server/data:/opt/valheim`.

- **Server plugins dir** `~/valheim-server/config/bepinex/plugins` (`/config/bepinex/plugins` in the container) is where the image wants your plugins. Not the folder BepInEx loads from (`/opt/valheim/bepinex/BepInEx/plugins`): the image rebuilds that one from scratch on every Valheim or BepInEx update (it checks every 15 minutes by default) and only copies `/config/bepinex/plugins` back in, so anything pushed there directly disappears with the next update.
- **Follow-up command** applies the pushed mods and restarts the container so they load, but only when nobody is playing:

  ```sh
  # valheim-server-docker: mirror /config/bepinex/plugins
  # into the live BepInEx dir, restart if no players online
  docker exec valheim-server rsync -a --delete \
    /config/bepinex/plugins/ \
    /opt/valheim/bepinex/BepInEx/plugins/
  if docker exec valheim-server valheim-is-idle; then
    docker restart valheim-server
  else
    echo "server was not restarted: there are players"
  fi
  ```

  The image itself copies `/config/bepinex/plugins` into the live folder on container start, but with `rsync -a`, which never deletes: mods you removed would keep loading. The `--delete` mirror fixes that. It runs even when players are online, which is safe: files are replaced by rename, not overwritten in place, so the running server keeps the versions it loaded, and the next restart (the image's daily one or an update) picks up exactly the pushed set. `valheim-is-idle` ships with the image and exits 0 when no players are connected.

Other volume paths or container names: just edit the two fields. The ssh user needs to be in the `docker` group for the follow-up command.

### Permissions

The ssh user must be able to write to the **parent** of the server plugins dir: a sync creates `plugins.new` next to it and renames `plugins` to `plugins.bak`. **Test connection** reports this as `write access: ok` or a warning.

With valheim-server-docker this is a common problem. The container runs as root by default (`PUID=0`, `PGID=0`), and **on every container start** it runs `chown -R` over `/config` (and `/opt/valheim`) to the user it runs as. So the config volume, including `~/valheim-server/config/bepinex/plugins`, ends up owned by `root`, your ssh user cannot write there, and the sync fails with `Permission denied`.

A one-off `sudo chown -R "$USER": ~/valheim-server/config/bepinex` does **not** last: the next container start, including the restart in the default follow-up command, gives it back to root. The durable fix is to run the container as your ssh user, so the image's own `chown` hands the files to you:

```yaml
# docker-compose.yml of valheim-server-docker
environment:
  PUID: 1000 # id -u of the ssh user
  PGID: 1000 # id -g of the ssh user
```

Then recreate the container (`docker compose up -d`). The game server itself then runs as that user too; see the image README's notes on non-root `PUID`/`PGID` before switching an existing server.

### How it works

1. Packs the local folder into a temporary `.tar.gz`.
2. Runs `ssh <server> 'tar -xzf - ...'` with the archive as stdin: unpacks into `<dir>.new`, moves the current `<dir>` to `<dir>.bak`, moves `<dir>.new` to `<dir>`.
3. Runs the follow-up command with a second `ssh` call.

ssh runs with `BatchMode=yes`, so it never waits for input the GUI cannot give. Every run uploads the whole folder (not an incremental rsync-style sync), which is fine for mod folders of tens of MB.

### Version Information

```bash
valheim-mod-pusher --version
```

## Development

The toolchain is pinned with [mise](https://mise.jdx.dev):

```bash
mise install           # go, golangci-lint, goreleaser
mise run build         # build ./valheim-mod-pusher with version info
mise run run           # build and launch
mise run check         # vet + lint + test + test-race (what CI runs)
mise run test-live     # end-to-end against a real server: MOD_PUSHER_HOST=<host> MOD_PUSHER_PORT=<port>
mise run release-check # local goreleaser snapshot, nothing published (needs `brew install mingw-w64`)
mise tasks             # list everything else
```

The live tests only touch `~/valheim-mod-pusher-it` on the server, and their follow-up command only echoes. On Windows run them from a desktop session: `ssh.exe` hangs when spawned from inside another ssh session.

CI runs on Linux, macOS and Windows runners, because the GUI toolkit (Fyne) is cgo on every platform. Pushing a `v*` tag runs the release: Linux archives are built natively on Linux runners (amd64 and arm64), GoReleaser on a macOS runner builds macOS (arm64, amd64) and Windows (amd64, via mingw-w64), publishes everything to one GitHub release and updates the Homebrew cask. Running the release workflow manually does a snapshot dry run and uploads the result as a workflow artifact.

## Dependencies

- [Fyne](https://fyne.io) - cross-platform GUI toolkit
- [x/crypto/ssh](https://pkg.go.dev/golang.org/x/crypto/ssh) - parsing host keys for the trust prompt

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Support

If you encounter any issues or have questions, please open an issue on GitHub.
