#!/usr/bin/env bash
# Usage: sudo ./install.sh <server-url> <token>
set -e

SERVER_URL="$1"
TOKEN="$2"

if [ -z "$SERVER_URL" ] || [ -z "$TOKEN" ]; then
    echo "Usage: sudo ./install.sh <server-url> <token>"
    exit 1
fi

INSTALL_DIR="/opt/khemstrix-agent"
EXE_PATH="$INSTALL_DIR/khemstrixAgent"
STATE_PATH="/etc/khemstrix-agent/state.json"

echo "== Khemstrix EDR Agent installer =="

echo "Downloading agent from $SERVER_URL..."
mkdir -p "$INSTALL_DIR"
curl -sSL "$SERVER_URL/download/agent/linux" -o "$EXE_PATH"
chmod +x "$EXE_PATH"
echo "Downloaded to $EXE_PATH"

echo "Installing background service..."
"$EXE_PATH" install --server="$SERVER_URL" --token="$TOKEN"

echo "Starting service..."
"$EXE_PATH" start

echo "Waiting for the agent to confirm it can reach the server..."
CONNECTED=false
for i in $(seq 1 15); do
    if [ -f "$STATE_PATH" ]; then
        LAST_SUCCESS=$(grep -o '"last_success_at": *"[^"]*"' "$STATE_PATH" || true)
        LAST_ERROR=$(grep -o '"last_error": *"[^"]*"' "$STATE_PATH" || true)
        if [ -n "$LAST_SUCCESS" ] && [ -z "$LAST_ERROR" ]; then
            CONNECTED=true
            break
        fi
    fi
    sleep 2
done

echo ""
if [ "$CONNECTED" = true ]; then
    echo -e "\e[32mSUCCESS — agent installed, running in the background, and connected to $SERVER_URL\e[0m"
    "$EXE_PATH" status
else
    echo -e "\e[31mThe agent installed and started, but hasn't confirmed a successful connection yet.\e[0m"
    echo "Troubleshooting: run this to see the real error directly:"
    echo "  $EXE_PATH --server=$SERVER_URL --token=$TOKEN"
    exit 1
fi