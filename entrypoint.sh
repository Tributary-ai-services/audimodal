#!/bin/sh

# If running as root, fix permissions and switch to app user
if [ "$(id -u)" = "0" ]; then
    echo "Running as root, fixing permissions..."
    
    # Create directories if they don't exist
    mkdir -p /app/temp /app/data /app/logs
    
    # Fix ownership
    chown -R appuser:appgroup /app/temp /app/data /app/logs
    
    # Ensure write permissions
    chmod -R 755 /app/temp
    
    echo "Permissions fixed, switching to appuser..."
    # Use su-exec to switch to appuser and run the command
    exec su-exec appuser "$@"
else
    # Already running as non-root, just exec the command
    echo "Running as non-root user (uid: $(id -u))"
    exec "$@"
fi