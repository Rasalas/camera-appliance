# Operations

## Daily Use

- `bin/open-cameras` opens AgentDVR fullscreen.
- `bin/rediscover-cameras` searches for cameras.
- `bin/restart-cameras` restarts AgentDVR, go2rtc, and the manager.
- `bin/status` prints local service and camera status.

## Logs and Events

The manager stores recent operational events in SQLite. The admin UI shows these under **Logs und Ereignisse**.

## Reset Lab State

Before moving from lab to customer network:

```bash
camera-appliance reset-bindings --yes
```

This removes discovered devices and bindings only. It does not remove secrets, code, services, or AgentDVR base configuration.
