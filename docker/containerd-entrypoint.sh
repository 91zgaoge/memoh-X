#!/bin/sh

MCP_IMAGE="${MCP_IMAGE:-docker.io/library/memoh-mcp:latest}"

# Setup CNI configuration
mkdir -p /etc/cni/net.d /opt/cni/bin

# Install CNI plugins if available
if [ -d /usr/lib/cni ]; then
    cp -a /usr/lib/cni/* /opt/cni/bin/ 2>/dev/null || true
fi
if [ -d /usr/libexec/cni ]; then
    cp -a /usr/libexec/cni/* /opt/cni/bin/ 2>/dev/null || true
fi

# Create CNI network config if not exists
if [ ! -f /etc/cni/net.d/10-memoh.conflist ]; then
    # Check if shared CNI config exists
    if [ -f /opt/memoh/data/shared/10-memoh.conflist ]; then
        echo "Using shared CNI config from /opt/memoh/data/shared/"
        cp /opt/memoh/data/shared/10-memoh.conflist /etc/cni/net.d/10-memoh.conflist
    else
        # Auto-detect Docker bridge for memoh network
        echo "Detecting Docker network configuration..."
        # Get default gateway interface
        GATEWAY_IP=$(ip route | grep default | awk '{print $3}' | head -1)
        # Find the bridge interface connected to the same subnet
        BRIDGE_IF=""
        for br in $(ip -o link show type bridge | awk -F': ' '{print $2}'); do
            if ip addr show "$br" | grep -q "172.26.0.1"; then
                BRIDGE_IF="$br"
                break
            fi
        done

        if [ -n "$BRIDGE_IF" ]; then
            echo "Found Docker bridge: $BRIDGE_IF"
            cat > /etc/cni/net.d/10-memoh.conflist << EOF
{
  "cniVersion": "1.0.0",
  "name": "memoh-cni",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "$BRIDGE_IF",
      "isGateway": false,
      "ipMasq": false,
      "promiscMode": true,
      "ipam": {
        "type": "host-local",
        "ranges": [[
          { "subnet": "172.26.0.0/16", "gateway": "172.26.0.1" }
        ]],
        "routes": [
          { "dst": "0.0.0.0/0" }
        ]
      }
    },
    {
      "type": "portmap",
      "capabilities": { "portMappings": true }
    }
  ]
}
EOF
        else
            echo "WARNING: Could not detect Docker bridge, using default CNI config"
            cat > /etc/cni/net.d/10-memoh.conflist << 'EOF'
{
  "cniVersion": "1.0.0",
  "name": "memoh-cni",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "cni0",
      "isGateway": true,
      "ipMasq": true,
      "promiscMode": true,
      "ipam": {
        "type": "host-local",
        "ranges": [[
          { "subnet": "10.88.0.0/16" }
        ]],
        "routes": [
          { "dst": "0.0.0.0/0" }
        ]
      }
    },
    {
      "type": "portmap",
      "capabilities": { "portMappings": true }
    }
  ]
}
EOF
        fi
    fi
fi

# Start containerd in background
mkdir -p /run/containerd
containerd &
CONTAINERD_PID=$!

# Wait for containerd to be fully responsive
echo "Waiting for containerd..."
for i in $(seq 1 30); do
  if ctr version >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! ctr version >/dev/null 2>&1; then
  echo "ERROR: containerd not responsive after 30s"
  exit 1
fi
echo "containerd is running"

# Always import bundled MCP image tar so startup picks up image updates
echo "Importing MCP image into containerd..."
for tar in /opt/images/*.tar; do
  if [ -f "$tar" ]; then
    ctr -n default images import --all-platforms "$tar" 2>&1 || true
  fi
done

# Verify image availability after import.
# For this ctr version, explicit "images unpack" is not available; snapshot
# preparation is handled when creating containers from the image.
if ctr -n default images check "name==${MCP_IMAGE}" 2>/dev/null | grep -q "${MCP_IMAGE}"; then
  echo "MCP image ready: ${MCP_IMAGE}"
else
  echo "WARNING: MCP image not available after import, will try pull at runtime"
fi

echo "containerd is ready"
wait $CONTAINERD_PID
