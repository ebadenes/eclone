# eclone

An enhanced fork of [rclone](https://github.com/rclone/rclone) with advanced Google Drive Service Account (SA) rotation.

Based on [gclone](https://github.com/dogbutcat/gclone) with additional features ported from [fclone](https://github.com/mawaya/rclone), including SA preloading, 25-hour blacklisting, and anti-thrashing protection.

**Current version:** `v1.71.0-mod2.0.0` (rclone v1.71.0 base)

## Features

All standard rclone features, plus:

| Feature | Origin | Description |
|---------|--------|-------------|
| Dynamic SA rotation | gclone | Automatically switches SA files on rate-limit errors |
| Rolling SA | gclone | Proactive SA rotation before each operation for balanced usage |
| Folder ID support | gclone | Use `{folder_id}` syntax for direct access to shared drives |
| SA preloading | fclone | Pre-creates OAuth services at startup to eliminate 200-500ms switch latency |
| 25h blacklist | fclone | Temporarily blacklists rate-limited SAs aligned with Google's daily quota reset |
| Anti-thrashing | fclone | Configurable minimum sleep between SA changes |
| Pacer reset on SA change | fclone | Fresh backoff avoids inheriting exponential sleep from exhausted SA |
| Broader rate-limit detection | fclone | Catches `dailyLimitExceededUnreg` and `Daily Limit` prefix errors |
| Service recycling | fclone | Old OAuth services returned to pool for reuse instead of discarded |
| Auto-assign SA | fclone | Automatically picks an SA if none configured but SA path exists |
| Auto-lower pacer | fclone | Reduces pacer min sleep when >10 SAs preloaded (more headroom) |

## Build

### With Docker (recommended, no Go installation needed)

```sh
# Clone the repository
git clone https://github.com/ebadenes/eclone.git
cd eclone

# Build using Go 1.24 Docker image
docker run --rm -v "$(pwd)":/app -w /app golang:1.24 \
  go build -buildvcs=false -ldflags="-s -w" -o /app/eclone .

# Verify
./eclone eversion
```

### With local Go

```sh
# Requires Go 1.24+
go build -ldflags="-s -w" -o eclone .

# With mount support (requires FUSE + gcc)
go build -v -tags 'cmount' -ldflags="-s -w" -o eclone .
```

### Install

```sh
sudo cp eclone /usr/local/bin/eclone
```

## Configuration

### 1. Service Account File Path

Add `service_account_file_path` to your rclone config for dynamic SA rotation. SAs are switched automatically when rate-limit errors occur.

`~/.config/rclone/rclone.conf` example:
```ini
[gc]
type = drive
scope = drive
service_account_file = /path/to/accounts/1.json
service_account_file_path = /path/to/accounts/
root_folder_id = root
```

The accounts folder should contain multiple SA JSON files with appropriate Google Drive permissions.

### 2. Advanced SA Options

These options can be set in `rclone.conf` or via command-line flags:

| Option | Flag | Default | Description |
|--------|------|---------|-------------|
| `service_account_file_path` | `--drive-service-account-file-path` | *(empty)* | Path to directory containing SA JSON files |
| `random_pick_sa` | `--drive-random-pick-sa` | `false` | Random SA selection at startup instead of first file |
| `rolling_sa` | `--drive-rolling-sa` | `false` | Proactive SA rotation before each operation |
| `rolling_count` | `--drive-rolling-count` | `1` | Parallel operations sharing the same SA |
| `service_account_min_sleep` | `--drive-service-account-min-sleep` | `100ms` | Minimum time between SA changes (anti-thrashing) |
| `services_preload` | `--drive-services-preload` | `50` | Number of SA services to preload at startup |
| `services_max` | `--drive-services-max` | `100` | Maximum preloaded services kept in memory |

### 3. Folder ID Support

eclone supports passing Google Drive folder/file IDs directly using curly braces:

```sh
# Copy between folders by ID
eclone copy gc:{folder_id1} gc:{folder_id2} --drive-server-side-across-configs

# Copy to a subfolder path within a destination ID
eclone copy gc:{folder_id1} gc:{folder_id2}/media/ --drive-server-side-across-configs

# Copy from a shared drive
eclone copy gc:{shared_drive_id} gc:{folder_id2} --drive-server-side-across-configs
```

### 4. Command-Line Examples

```sh
# Basic copy with SA rotation (auto on rate limit)
eclone copy gc:{id1} gc:{id2} --drive-service-account-file-path=/path/to/SAs/

# Rolling SA mode (proactive rotation + random initial pick)
eclone copy gc:{id1} gc:{id2} \
  --drive-rolling-sa \
  --drive-rolling-count=1 \
  --drive-random-pick-sa

# With preloading and custom throttle
eclone copy gc:{id1} gc:{id2} \
  --drive-service-account-file-path=/path/to/SAs/ \
  --drive-services-preload=30 \
  --drive-service-account-min-sleep=200ms
```

### 5. Self-Update

```sh
eclone eselfupdate [--check] [--output path] [--version v] [--package zip|deb|rpm]
```

## How SA Rotation Works

```
                   Startup
                      |
           Load SA files from path
                      |
              Preload N services
             (OAuth clients ready)
                      |
        +----- Begin operations -----+
        |                            |
   [rolling_sa=true]          [rolling_sa=false]
        |                            |
  Rotate SA proactively       Use current SA
  before each operation              |
        |                     On rate-limit error
        |                            |
        |                   shouldChangeSA()?
        |                    (throttle guard)
        |                            |
        |                   changeSvc():
        |                   - Blacklist old SA (25h)
        |                   - Get new SA from pool
        |                   - Recycle old service
        |                   - Reset pacer
        |                            |
        +------- Continue -----------+
```

## Google Drive Quotas

Service Accounts allow bypassing some Google quotas:

**CAN bypass with multiple SAs:**
- Copy/upload quota (750 GB/account/day)
- Download quota (10 TB/account/day)

**CANNOT bypass:**
- Shared Drive quota (~20 TB/drive/day)
- File owner quota (~2 TB/day)

## Credits

- [rclone](https://github.com/rclone/rclone) - The cloud sync tool
- [gclone](https://github.com/dogbutcat/gclone) - SA rotation foundation and rolling SA
- [fclone](https://github.com/mawaya/rclone) - SA pool, preloading, and blacklist features

## License

Same as rclone - [MIT License](https://github.com/rclone/rclone/blob/master/COPYING).
