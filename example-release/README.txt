Example Release Package
=======================

This is an example release folder structure for testing SyncBot.

Contents:
- bin/app        : Main application executable
- lib/           : Shared libraries
- config/        : Configuration files

Usage:
1. Create an endpoint in SyncBot
2. Copy the rsync command from the UI
3. Run: rsync -avz --delete ./example-release/ sync@device:/var/sync/your-endpoint/
4. Activate your endpoint in the UI
