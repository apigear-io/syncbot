#!/bin/bash
set -e

# Configuration
BINARY="dist/syncbot-linux-amd64"
DEVICES_FILE="devices.txt"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Check if device is online (ping with 2 second timeout)
is_device_online() {
    local host="$1"
    ping -c 1 -W 2 "$host" >/dev/null 2>&1
}

usage() {
    echo "Usage: $0 [options]"
    echo ""
    echo "Deploy syncbot to remote devices"
    echo ""
    echo "Options:"
    echo "  -f FILE    Devices file (default: devices.txt)"
    echo "  -b BINARY  Binary to deploy (default: dist/syncbot-linux-amd64)"
    echo "  -F         Force deploy even if binary unchanged"
    echo "  -h         Show this help"
    echo ""
    echo "For password auth: SSHPASS=mypassword $0"
    echo ""
    exit 1
}

FORCE=false

while getopts "f:b:Fh" opt; do
    case $opt in
        f) DEVICES_FILE="$OPTARG" ;;
        b) BINARY="$OPTARG" ;;
        F) FORCE=true ;;
        h) usage ;;
        *) usage ;;
    esac
done

# Setup sshpass if SSHPASS env var is set
SSH_CMD="ssh"
SCP_CMD="scp"
if [ -n "$SSHPASS" ]; then
    SSH_CMD="sshpass -e ssh"
    SCP_CMD="sshpass -e scp"
fi

# Build first
echo -e "${GREEN}Building...${NC}"
task build:all

if [ ! -f "$BINARY" ]; then
    echo -e "${RED}Error: Binary not found: $BINARY${NC}"
    exit 1
fi

if [ ! -f "$DEVICES_FILE" ]; then
    echo -e "${RED}Error: Devices file not found: $DEVICES_FILE${NC}"
    exit 1
fi

# Get local binary checksum
LOCAL_MD5=$(md5sum "$BINARY" 2>/dev/null || md5 -q "$BINARY" 2>/dev/null)
LOCAL_MD5="${LOCAL_MD5%% *}"

DEVICE_COUNT=$(grep -v '^#' "$DEVICES_FILE" | grep -v '^$' | wc -l | tr -d ' ')
echo -e "${GREEN}Deploying to $DEVICE_COUNT device(s)...${NC}"
echo ""

# Don't exit on errors during device loop
set +e

while IFS= read -r device || [ -n "$device" ]; do
    [[ -z "$device" || "$device" =~ ^# ]] && continue

    USERNAME="${device%%@*}"
    HOST="${device##*@}"
    REMOTE_DIR="/home/$USERNAME/syncbot"
    REMOTE_BINARY="$REMOTE_DIR/syncbot"

    echo -e "${YELLOW}=== $device ===${NC}"

    # Check if device is online
    if ! is_device_online "$HOST"; then
        echo -e "  ${RED}Offline - skipping${NC}"
        echo ""
        continue
    fi

    # Check if binary needs updating
    if [ "$FORCE" = false ]; then
        REMOTE_MD5=$($SSH_CMD -n "$device" "md5sum $REMOTE_BINARY 2>/dev/null | cut -d' ' -f1" 2>/dev/null)

        if [ "$LOCAL_MD5" = "$REMOTE_MD5" ]; then
            echo -e "  ${BLUE}Binary unchanged, skipping${NC}"
            echo ""
            continue
        fi
    fi

    # Copy and deploy
    echo "  Copying binary..."
    if ! $SCP_CMD -q "$BINARY" "$device:~/syncbot-new"; then
        echo -e "  ${RED}Failed to copy binary${NC}"
        echo ""
        continue
    fi

    # Use sudo -S to read password from stdin if SSHPASS is set
    SUDO_CMD="sudo"
    if [ -n "$SSHPASS" ]; then
        SUDO_PREFIX="echo '$SSHPASS' |"
        SUDO_CMD="sudo -S"
    else
        SUDO_PREFIX=""
    fi

    echo "  Restarting service..."
    $SSH_CMD -n "$device" "
        REMOTE_DIR='$REMOTE_DIR'
        SUDO_PREFIX='$SUDO_PREFIX'
        SUDO_CMD='$SUDO_CMD'
        eval \"\$SUDO_PREFIX \$SUDO_CMD systemctl stop syncbot\" 2>/dev/null || true
        mv ~/syncbot-new \"\$REMOTE_DIR/syncbot\"
        chmod +x \"\$REMOTE_DIR/syncbot\"
        eval \"\$SUDO_PREFIX \$SUDO_CMD systemctl start syncbot\"
        sleep 1
        systemctl is-active --quiet syncbot && echo '  Service running' || echo '  Warning: Service may not have started'
    "

    echo -e "${GREEN}  Done${NC}"
    echo ""

done < "$DEVICES_FILE"

set -e

echo -e "${GREEN}Deployment complete!${NC}"
