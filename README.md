# randr

A lightweight daemon that automatically mirrors your display when an external monitor is connected.

It polls `xrandr` to detect hotplug events, picks the highest resolution common to all connected displays, and configures mirroring with a single `xrandr` call. When the external monitor is disconnected, it restores the primary display to its native resolution.

## Features

- Automatic display mirroring on monitor hotplug
- Best common resolution detection across all connected outputs
- Automatic restore to native resolution on disconnect
- Runs as a user-level systemd service
- No dependencies beyond `xrandr` and Go

## Requirements

- Linux with X11
- `xrandr` (part of `x11-xserver-utils` on Debian/Ubuntu)
- Go 1.21+
- systemd (for service installation)

## Build

```sh
make
```

## Install

Installs the binary to `~/.local/bin` and enables a user-level systemd service:

```sh
make install
```

## Uninstall

Stops the service and removes all installed files:

```sh
make uninstall
```

## Usage

### As a systemd service

```sh
make install      # build, install, enable, and start
make status       # check if running
make logs         # tail live logs
make uninstall    # stop and remove
```

### Standalone

```sh
# foreground
./randr

# background, survives terminal close
nohup ./randr > /tmp/randr.log 2>&1 &
```

## How it works

1. On startup, `randr` snapshots the set of connected outputs via `xrandr --query`.
2. Every 2 seconds it re-queries and compares against the previous snapshot.
3. When a new output appears:
   - It collects the supported resolutions of every connected display.
   - It intersects those lists and selects the highest resolution (by pixel count) common to all of them.
   - It runs `xrandr --same-as` to mirror all externals onto the primary display at that resolution.
4. When an output disappears, it restores the primary display to its first listed (native) resolution.
5. All actions are logged with timestamps to stderr / the systemd journal.

## Makefile targets

| Target      | Description                                      |
|-------------|--------------------------------------------------|
| `all`       | Build the binary (default)                       |
| `install`   | Build, install to `~/.local/bin`, enable service |
| `uninstall` | Stop service, remove binary and unit file        |
| `status`    | Show systemd service status                      |
| `logs`      | Tail the service journal                         |
| `clean`     | Remove build artifact                            |

## License

MIT
