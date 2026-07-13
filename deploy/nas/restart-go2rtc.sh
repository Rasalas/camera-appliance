#!/usr/bin/env bash
set -euo pipefail
if systemctl --user is-active --quiet camera-appliance-go2rtc.service 2>/dev/null; then
  exec systemctl --user restart camera-appliance-go2rtc.service
fi
exec "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/native-service.sh" restart-go2rtc
