#!/bin/bash
set -e

# Create cache directory
mkdir -p /var/cache/keystone-core
chmod 750 /var/cache/keystone-core

# Create log directory if it doesn't exist
mkdir -p /var/log/keystone-core
chmod 750 /var/log/keystone-core

# Set ownership of config directory
if [ -d /etc/keystone-core ]; then
    chmod 750 /etc/keystone-core
    # Agent config should only be readable by root
    if [ -f /etc/keystone-core/agent.yaml ]; then
        chmod 600 /etc/keystone-core/agent.yaml
    fi
fi

# Reload systemd
systemctl daemon-reload

echo ""
echo "Keystone Core Agent installed successfully!"
echo ""
echo "Next steps:"
echo "  1. Edit /etc/keystone-core/agent.yaml to configure the agent"
echo "     - Set the control plane URL in server.urls"
echo "     - Configure agent tags for targeting"
echo "  2. Start the service: systemctl start kscore-agent"
echo "  3. Enable on boot: systemctl enable kscore-agent"
echo ""
echo "Documentation: https://docs.keystonecore.io"
echo ""
