#!/bin/bash
set -e

# Configuration
DEVICES_FILE="devices.txt"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

usage() {
    echo "Usage: $0 [options]"
    echo ""
    echo "Initialize syncbot on remote devices (first-time setup)"
    echo ""
    echo "Options:"
    echo "  -f FILE    Devices file (default: devices.txt)"
    echo "  -h         Show this help"
    echo ""
    echo "For password auth: SSHPASS=mypassword $0"
    echo ""
    exit 1
}

while getopts "f:h" opt; do
    case $opt in
        f) DEVICES_FILE="$OPTARG" ;;
        h) usage ;;
        *) usage ;;
    esac
done

# Setup sshpass if SSHPASS env var is set
SSH_CMD="ssh"
if [ -n "$SSHPASS" ]; then
    SSH_CMD="sshpass -e ssh"
fi

if [ ! -f "$DEVICES_FILE" ]; then
    echo -e "${RED}Error: Devices file not found: $DEVICES_FILE${NC}"
    exit 1
fi

DEVICE_COUNT=$(grep -v '^#' "$DEVICES_FILE" | grep -v '^$' | wc -l | tr -d ' ')
echo -e "${GREEN}Initializing $DEVICE_COUNT device(s)...${NC}"
echo ""

while IFS= read -r device || [ -n "$device" ]; do
    [[ -z "$device" || "$device" =~ ^# ]] && continue

    USERNAME="${device%%@*}"

    echo -e "${YELLOW}=== Initializing $device ===${NC}"

    $SSH_CMD -n "$device" "
        set -e
        USERNAME='$USERNAME'
        SYNCBOT_DIR=\"/home/\$USERNAME/syncbot\"

        echo '  Creating directory:' \$SYNCBOT_DIR
        mkdir -p \"\$SYNCBOT_DIR\"

        echo '  Installing systemd service...'
        sudo tee /etc/systemd/system/syncbot.service > /dev/null << EOF
[Unit]
Description=SyncBot - Endpoint Management
After=network.target

[Service]
Type=simple
User=\$USERNAME
WorkingDirectory=\$SYNCBOT_DIR
ExecStart=\$SYNCBOT_DIR/syncbot
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

        sudo systemctl daemon-reload
        sudo systemctl enable syncbot
        echo '  Service installed and enabled'
    "

    echo -e "${GREEN}  Done${NC}"
    echo ""

done < "$DEVICES_FILE"

echo -e "${GREEN}Initialization complete! Now run: ./deploy.sh${NC}"
