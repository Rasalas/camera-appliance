export interface CameraEndpoint {
  host: string
  port: string
  target_host?: string
  target_port?: string
}

export interface CameraConfiguration {
  access?: { username: string; password: string; stream: string }
  name?: string
  visible?: boolean
  path_policy?: string
  display?: Record<string, string>
  endpoint?: CameraEndpoint
  relay_endpoints?: Record<string, CameraEndpoint>
}

export interface CameraConfigurationResult {
  persisted: boolean
  password_persisted: boolean
  applied: boolean
  restarted: boolean
  error?: string
  warnings?: string[]
}

export function cameraConfigurationError(result: CameraConfigurationResult): string {
  if (!result.persisted) return result.error || 'Die Kamera-Konfiguration konnte nicht gespeichert werden.'
  if (!result.applied) return `Die Änderung ist gespeichert, aber noch nicht angewendet. ${result.error || 'Der Videodienst konnte nicht aktualisiert werden.'} Bitte erneut versuchen.`
  return ''
}
