# SyncBot

SyncBot is a web application for managing software deployment endpoints on test devices. It allows teams to rsync software releases to named endpoints on a device, then activate specific endpoints via a web UI.

## Features

- **Endpoint Management** - Create and delete named endpoints (directories for rsync targets)
- **Activation** - Activate endpoints by updating a symlink and running a post-activation command
- **Backup** - Create partial .tgz backups of endpoints using glob patterns
- **Commands** - User-defined shell commands with Go template variable expansion
- **Device Registry** - Track multiple SyncBot instances across your network
- **Web Terminal** - Browser-based terminal access to the device
- **Real-time Logging** - Live log streaming via Server-Sent Events

## Quick Start

### Build

```bash
go build -o syncbot .
```

### Run

```bash
./syncbot --config config.yaml
```

On first run, a default `config.yaml` will be created if it doesn't exist.

### Access

Open your browser to `http://localhost:8081` (or the configured port).

## Configuration

SyncBot uses a YAML configuration file. Example:

```yaml
device_name: my-device
device_description: Development Test Device
device_location: Lab A
endpoints_path: /home/user/syncbot/endpoints
active_symlink: /home/user/syncbot/active
post_activation_command: systemctl restart myapp
port: 8081
device_ip: 192.168.1.100
ssh_username: sync
terminal:
  shell: /bin/bash
devices: []
commands: []
```

### Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `device_name` | Display name for this device | Hostname |
| `device_description` | Description shown in UI | "SyncBot Device" |
| `device_location` | Physical location | Empty |
| `endpoints_path` | Directory where endpoints are created | `~/syncbot/endpoints` |
| `active_symlink` | Path to the active symlink | `~/syncbot/active` |
| `post_activation_command` | Command to run after activation | Empty |
| `port` | HTTP server port | 8081 |
| `device_ip` | IP address for rsync commands | localhost |
| `ssh_username` | Username for rsync commands | sync |
| `terminal.shell` | Shell for web terminal | /bin/bash |

## Usage

### Endpoints

1. Create an endpoint from the Endpoints page
2. Use rsync to deploy your software to the endpoint:
   ```bash
   rsync -avz ./build/ user@device:/path/to/endpoints/my-endpoint/
   ```
3. Click "Activate" to make it the active endpoint

### Backups

1. Edit an endpoint and configure backup patterns (e.g., `*.log`, `config/*`, `data/**/*.json`)
2. Click the backup button on the endpoint
3. Preview files, then create the backup
4. Download backups from the Backups page

### Commands

Define custom commands in Settings with template variables:
- `{{.HomeDir}}` - User home directory
- `{{.ActiveSymlink}}` - Path to active symlink
- `{{.ActiveName}}` - Name of active endpoint
- `{{.EndpointsPath}}` - Path to endpoints directory

### Device Registry

Add other SyncBot instances to the device registry for easy navigation between devices.

## API

SyncBot provides a REST API for automation:

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/endpoints` | List all endpoints |
| POST | `/api/endpoints` | Create endpoint |
| DELETE | `/api/endpoints/{name}` | Delete endpoint |
| POST | `/api/endpoints/{name}/activate` | Activate endpoint |
| GET | `/api/active` | Get active endpoint |
| GET | `/api/backups` | List all backups |
| POST | `/api/endpoints/{name}/backup` | Create backup |
| DELETE | `/api/backups/{archive}` | Delete backup |
| GET | `/api/commands` | List commands |
| POST | `/api/commands/{id}/execute` | Execute command |

## Architecture

- **Go** backend with Chi router
- **Tabler** (Bootstrap-based) UI framework
- **Server-Sent Events** for real-time updates
- **Embedded templates** using `//go:embed`

## License

MIT
