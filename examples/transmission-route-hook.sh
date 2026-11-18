#!/bin/sh
# Transmission accepts an executable filename, not a command with arguments.
exec /usr/local/bin/raspbibi transmission-route \
    -config /etc/raspbibi/transmission-route.json "$@"
