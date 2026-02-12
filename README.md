# eclone

An enhanced fork of [rclone](https://github.com/rclone/rclone) with advanced Google Drive Service Account (SA) rotation.

Based on [gclone](https://github.com/dogbutcat/gclone) with additional features ported from [fclone](https://github.com/mawaya/rclone), including SA preloading, 25-hour blacklisting, and anti-thrashing protection.

## Features

All standard rclone features, plus:

- **Dynamic SA rotation** - Automatically switches service account files when rate-limited
- **SA preloading** - Pre-creates OAuth services at startup to eliminate switch latency
- **25h blacklist** - Temporarily blacklists rate-limited SAs (aligns with Google's daily quota reset)
- **Anti-thrashing** - Configurable minimum sleep between SA changes
- **Rolling SA** - Proactive SA rotation for balanced usage across all service accounts
- **Folder ID support** - Use `{folder_id}` syntax for direct access to shared drives and folders

## Build

### Prerequisites

```
# Windows (cgo)
WinFsp, gcc (e.g. from Mingw-builds)

# macOS
FUSE for macOS, command line tools

# Linux
libfuse-dev, gcc
```

### Build

```sh
go build -v -tags 'cmount' eclone.go
```

### Verify

```sh
./eclone eversion
```

> If you need the `mount` function, cgofuse is required.

## Configuration

### 1. Service Account File Path

Add `service_account_file_path` to your config for dynamic SA rotation. SAs are switched automatically when `rateLimitExceeded` errors occur.

`rclone.conf` example:
```
[gc]
type = drive
scope = drive
service_account_file = /root/accounts/1.json
service_account_file_path = /root/accounts/
root_folder_id = root
```

The `/root/accounts/` folder should contain multiple service account files (`*.json`) with appropriate permissions.

### 2. Folder ID Support

eclone supports passing folder/file IDs directly using curly braces:

```sh
eclone copy gc:{folder_id1} gc:{folder_id2} --drive-server-side-across-configs
```

```sh
eclone copy gc:{folder_id1} gc:{folder_id2}/media/ --drive-server-side-across-configs
```

```sh
eclone copy gc:{share_field_id} gc:{folder_id2} --drive-server-side-across-configs
```

### 3. Command Line Options

```sh
# Specify SA path via command line
eclone copy gc:{id1} gc:{id2} --drive-service-account-file-path=/path/to/SAs/

# Rolling SA mode (proactive rotation)
eclone copy gc:{id1} gc:{id2} --drive-rolling-sa --drive-rolling-count=1

# Random initial SA selection
eclone copy gc:{id1} gc:{id2} --drive-random-pick-sa --drive-rolling-sa --drive-rolling-count=1
```

#### Rolling SA

Rotates SAs proactively before each operation for balanced usage across all service accounts, rather than exhausting one SA at a time.

#### Rolling Count

Controls the waitgroup count for parallel operations sharing the same SA. Default is 1. Values over 4 are not recommended; larger files work better with smaller counts.

### 4. Self-Update

```sh
eclone eselfupdate [--check] [--output path] [--version v] [--package zip|deb|rpm]
```

## Quotas

Service Accounts (SAs) allow bypassing some Google quotas:

**CAN bypass:**
- Google 'copy/upload' quota (750GB/account/day)
- Google 'download' quota (10TB/account/day)

**CANNOT bypass:**
- Google Shared Drive quota (~20TB/drive/day)
- Google file owner quota (~2TB/day)

## Credits

- [rclone](https://github.com/rclone/rclone) - The original cloud sync tool
- [gclone](https://github.com/dogbutcat/gclone) - SA rotation foundation
- [fclone](https://github.com/mawaya/rclone) - Advanced SA pool features
