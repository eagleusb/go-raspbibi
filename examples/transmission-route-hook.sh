#!/bin/sh
# Transmission accepts an executable filename, not a command with arguments.
if [ "$(/usr/bin/id -u)" != 0 ]; then
    printf '%s\n' 'raspbibi-transmission-route: root privileges are required to switch to jellyfin:jellyfin' >&2
    exit 1
fi

exec /usr/bin/setpriv --reuid=jellyfin --regid=jellyfin --init-groups --inh-caps=-all -- \
    /usr/local/bin/raspbibi transmission-route -config /etc/raspbibi/transmission-route.json "$@"
