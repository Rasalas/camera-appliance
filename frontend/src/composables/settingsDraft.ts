export const generalSettingKeys = ['capture_ssh_host', 'auto_discover', 'render_after_discovery', 'restart_after_render', 'viewer.performance.mode']
export const maintenanceSettingKeys = ['watchdog.enabled', 'watchdog.restart_on_change', 'watchdog.restart_go2rtc_on_failure', 'watchdog.fast_interval_seconds', 'watchdog.camera_interval_seconds', 'camera.path.fail_threshold', 'camera.path.recovery_threshold', 'camera.path.restart_cooldown_seconds']
export const accessSettingKeys = ['network.lan_access_enabled', 'auth.viewer_public', 'auth.local_admin_bypass', 'auth.session_hours']

export function relaySettingKeys(id: string): string[] {
  return ['name', 'type', 'host', 'bind_host', 'ssh_target', 'auto_start', 'enabled', 'port_base', 'default_port'].map((field) => `camera.relay.${id}.${field}`)
}

// A form can only save its own changed fields. Other forms and server-owned
// runtime values in the shared read response never become part of the write.
export function settingsPatch(current: Record<string, string>, baseline: Record<string, string>, keys: readonly string[]): Record<string, string> {
  return Object.fromEntries(keys.filter((key) => key in current && current[key] !== baseline[key]).map((key) => [key, current[key]]))
}
