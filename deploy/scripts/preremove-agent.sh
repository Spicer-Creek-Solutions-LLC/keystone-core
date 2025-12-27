#!/bin/bash
set -e

# Stop and disable the service before removal
if systemctl is-active --quiet kscore-agent; then
    echo "Stopping kscore-agent service..."
    systemctl stop kscore-agent
fi

if systemctl is-enabled --quiet kscore-agent 2>/dev/null; then
    echo "Disabling kscore-agent service..."
    systemctl disable kscore-agent
fi

echo "Keystone Core Agent will be removed."
echo "Configuration files in /etc/keystone-core will be preserved."
echo "Cache files in /var/cache/keystone-core will be preserved."
