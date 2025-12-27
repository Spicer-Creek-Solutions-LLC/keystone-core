#!/bin/bash
set -e

# Stop and disable the service before removal
if systemctl is-active --quiet kscore-server; then
    echo "Stopping kscore-server service..."
    systemctl stop kscore-server
fi

if systemctl is-enabled --quiet kscore-server 2>/dev/null; then
    echo "Disabling kscore-server service..."
    systemctl disable kscore-server
fi

echo "Keystone Core Server will be removed."
echo "Configuration files in /etc/keystone-core will be preserved."
echo "Data files in /var/lib/keystone-core will be preserved."
