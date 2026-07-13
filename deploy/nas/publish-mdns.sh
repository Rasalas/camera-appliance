#!/usr/bin/env bash
set -euo pipefail

ipv4="$(ip -4 -o addr show scope global | awk '$2 !~ /^(docker|br-|virbr|veth)/ { split($4, parts, "/"); print parts[1]; exit }')"
ipv6="$(ip -6 -o addr show scope global | awk '$2 !~ /^(docker|br-|virbr|veth)/ && $4 ~ /^fd/ { split($4, parts, "/"); print parts[1]; exit }')"
if [[ -z "$ipv4" ]]; then
  echo "Keine LAN-IPv4-Adresse für cams.local gefunden." >&2
  exit 1
fi

pids=()
cleanup() {
  ((${#pids[@]} == 0)) || kill "${pids[@]}" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

avahi-publish-address -R cams.local "$ipv4" &
pids+=("$!")
if [[ -n "$ipv6" ]]; then
  avahi-publish-address -R cams.local "$ipv6" &
  pids+=("$!")
fi

wait -n "${pids[@]}"
