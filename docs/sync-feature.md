# Settings Sync Feature - Implementation Guide

## Overview

A settings sync feature allows users to copy configuration from a "master" device to a "client" device. This is useful when managing multiple instances of an application across different machines, ensuring consistent configuration without manual copying.

## Architecture

### Core Concept

The sync is **one-way pull**: the client device fetches settings from a master device and selectively applies them. This keeps the master as the source of truth and gives users control over what gets synced.

### Components

1. **API Endpoint on Both Devices** - Export settings in a structured JSON format (`GET /api/sync/settings`)
2. **Fetch & Compare Endpoint** - Client-side endpoint that fetches master settings and returns both for comparison (`POST /api/sync/fetch`)
3. **Apply Endpoint** - Receives selected settings and applies them (`POST /api/sync/apply`)
4. **Wizard UI** - Step-by-step interface for reviewing and selecting settings

## Data Model

### Group Settings by Category

Organize settings into logical groups that users can review independently:

```
SyncableSettings {
  logging: { verbose, trace, ... }           // Runtime settings
  performance: { batchEnabled, interval }    // Runtime settings
  storage: { directory, maxSize, ... }       // Persistent settings
  infrastructure: { listenAddress, ... }     // Persistent settings
  items: { name -> config }                  // Collection items (proxies, endpoints, etc.)
}
```

### Distinguish Runtime vs Persistent Settings

- **Runtime settings**: Applied immediately in memory, no restart needed
- **Persistent settings**: Saved to config file, may require reload/restart

## API Design

### 1. Export Settings (`GET /api/sync/settings`)

Returns all syncable settings from the current device. This endpoint must be available on both master and client devices.

```json
{
  "logging": { "verbose": true, "trace": false },
  "performance": { "batchEnabled": true, "intervalMs": 50 },
  "infrastructure": { "listenAddress": ":8080" },
  "items": {
    "item1": { "name": "item1", "config": "..." },
    "item2": { "name": "item2", "config": "..." }
  }
}
```

### 2. Fetch & Compare (`POST /api/sync/fetch`)

Client sends master URL, server fetches master settings and returns both for comparison.

**Request:**
```json
{ "masterUrl": "http://192.168.1.100:8080" }
```

**Response:**
```json
{
  "master": { /* SyncableSettings */ },
  "local": { /* SyncableSettings */ }
}
```

**Why server-side fetch?** Avoids CORS issues that would occur if the browser fetched directly from another device.

### 3. Apply Settings (`POST /api/sync/apply`)

Client sends only the settings the user selected to sync.

**Request:**
```json
{
  "logging": { "verbose": true, "trace": false },
  "items": {
    "item1": { /* full config */ }
  }
}
```

**Response:**
```json
{ "success": true, "needsReload": true }
```

## UI Design - Step-by-Step Wizard

### Why a Wizard?

- Users may want different configurations on different devices
- Prevents accidental overwrites
- Provides clear visibility into what will change

### Wizard Steps

1. **Connect** - Enter master device URL and validate connection
2. **Settings Group 1** - Show diff table, checkboxes to select
3. **Settings Group 2** - Same pattern
4. **...more groups...**
5. **Collection Items** - Special handling with Add/Replace/Skip options
6. **Review & Apply** - Summary of all selected changes, apply button

### Diff Table UI

For each settings group, show a comparison table:

| Select | Setting | Master Device | This Device |
|--------|---------|---------------|-------------|
| [x] | Verbose | Enabled | Disabled |
| [ ] | Trace | Disabled | Disabled |

- Pre-select checkboxes where values differ
- Dim rows where values are the same
- Provide "Select All" / "Deselect All" buttons

### Collection Item Handling (e.g., Proxies, Endpoints)

For items that can be added/removed, provide options per item:

| Item | Status | Action |
|------|--------|--------|
| proxy1 | New | [Add] |
| proxy2 | Modified | [Replace] |
| proxy3 | Unchanged | [Keep] |

**Actions:**
- **Add** - Item exists on master but not locally
- **Replace** - Item exists on both, configs differ
- **Skip/Keep Local** - Don't change local config
- Note: Sync does NOT delete items (user must do manually)

## Smart Defaults

### Auto-select Different Values

When fetching settings, pre-select checkboxes for values that differ between master and local. This saves users time.

### Skip Device-Specific Settings

Some settings should default to "skip" because they're typically device-specific:
- Listen addresses/ports
- File paths/directories
- Machine-specific identifiers

### Remember Master URL

Store the last-used master URL in localStorage so users don't need to re-enter it:

```javascript
// On input
localStorage.setItem('app_sync_master_url', url);

// On page load
const saved = localStorage.getItem('app_sync_master_url');
if (saved) input.value = saved;
```

## Implementation Flow

### Client-Side (Wizard)

```
1. User enters master URL
2. POST /api/sync/fetch with masterUrl
3. Receive {master, local} settings
4. Initialize selections (pre-check differing values)
5. User steps through groups, adjusts selections
6. User reviews summary
7. POST /api/sync/apply with selected settings only
8. Show success, change button to "Done" -> navigate away
```

### Server-Side (Apply)

```
1. Parse request with selected settings
2. Apply runtime settings immediately (in-memory)
3. Merge persistent settings into config
4. Save config file
5. Trigger reload if needed
6. Return success with needsReload flag
```

## Error Handling

- **Connection failed** - Show error on step 1, allow retry
- **Invalid URL** - Validate URL format before attempting fetch
- **Timeout** - Set reasonable timeout (10s) for master fetch
- **Apply failed** - Show error, don't navigate away, allow retry

## Security Considerations

- The sync settings endpoint is read-only
- Server-side fetch prevents exposing master URL in browser requests
- Consider adding optional authentication for sensitive deployments
- Validate all incoming data before applying

## Summary

The key principles for a good sync feature:

1. **Pull, not push** - Client initiates and controls the sync
2. **Selective** - User chooses what to sync
3. **Grouped** - Organize settings logically
4. **Visual diff** - Show master vs local clearly
5. **Smart defaults** - Pre-select differences, skip device-specific
6. **Persistent memory** - Remember master URL
7. **Clear feedback** - Show what will change before applying
