<template>
  <header class="topline">
    <div>
      <div class="eyebrow">System · Einstellungen · Sicherung · Protokoll</div>
      <h1 class="headline">System.</h1>
    </div>
    <div class="meta">
      <div>Version · <b>{{ versionLabel }}</b></div>
      <div>Adresse · <b>{{ settings.bind_addr || '127.0.0.1:8091' }}</b></div>
      <div>go2rtc · <b>{{ settings.go2rtc_url || 'http://localhost:1984' }}</b></div>
    </div>
  </header>

  <div v-if="error" class="notice err"><span class="tag">FEHLER</span>{{ error }}</div>

  <!-- Section: Settings -->
  <section class="panel">
    <div class="panel-head">
      <h2>Einstellungen</h2>
      <button class="btn sm primary" @click="saveSettings">Speichern</button>
    </div>

    <div class="split">
      <div class="field">
        <span class="lbl">Kamera-Passwort</span>
        <div class="btn-row" style="align-items: stretch;">
          <input v-model="cameraPassword" type="password" :placeholder="settings.camera_password_set === 'true' ? '••••••••••••' : 'Passwort setzen'" style="flex: 1;" />
          <button class="btn" :disabled="!cameraPassword || savingPassword" @click="saveCameraPassword">
            {{ savingPassword ? 'Speichert…' : 'Passwort speichern' }}
          </button>
        </div>
        <div class="mono-mute" style="margin-top: 6px;">
          {{ settings.camera_password_set === 'true' ? `Gespeichert über ${passwordSource}` : 'Noch kein Kamera-Passwort gespeichert.' }}
        </div>
      </div>
      <div class="field">
        <span class="lbl">go2rtc-URL</span>
        <input v-model="settings.go2rtc_url" placeholder="http://localhost:1984" />
      </div>
      <div class="field">
        <span class="lbl">Admin-Adresse</span>
        <input v-model="settings.bind_addr" placeholder="127.0.0.1:8091" />
      </div>
      <div class="field">
        <span class="lbl">Capture-Hop per SSH</span>
        <input v-model="settings.capture_ssh_host" placeholder="leer oder nas" />
        <div class="mono-mute" style="margin-top: 6px;">
          Optional. Wenn gesetzt, zieht die App Referenzbilder per ffmpeg auf diesem SSH-Host.
        </div>
      </div>
    </div>

    <div style="display: grid; gap: 8px;">
      <label class="toggle-row">
        <input type="checkbox" :checked="settings.auto_discover === 'true'" @change="setBool('auto_discover', $event)" />
        <div>
          <div class="lbl-main">Beim Start automatisch suchen</div>
          <div class="lbl-sub">Discovery läuft direkt nach dem Boot.</div>
        </div>
      </label>
      <label class="toggle-row">
        <input type="checkbox" :checked="settings.render_after_discovery === 'true'" @change="setBool('render_after_discovery', $event)" />
        <div>
          <div class="lbl-main">go2rtc nach Suche erzeugen</div>
          <div class="lbl-sub">Neue Konfiguration wird automatisch geschrieben.</div>
        </div>
      </label>
      <label class="toggle-row">
        <input type="checkbox" :checked="settings.restart_after_render === 'true'" @change="setBool('restart_after_render', $event)" />
        <div>
          <div class="lbl-main">go2rtc nach Änderungen neu starten</div>
          <div class="lbl-sub">Streams stehen sofort am Player bereit.</div>
        </div>
      </label>
    </div>
  </section>

  <!-- Section: Kiosk layout -->
  <section class="panel">
    <div class="panel-head">
      <h2>Kiosk-Layout</h2>
      <div class="right">{{ viewerLayoutName }}</div>
    </div>

    <div class="layout-admin-grid">
      <div class="field">
        <span class="lbl">Preset</span>
        <select :value="viewerLayoutID" @change="setViewerLayoutFromEvent">
          <option v-for="layout in viewerLayoutOptions" :key="layout.id" :value="layout.id">{{ layout.name }}</option>
        </select>
      </div>
      <div class="field">
        <span class="lbl">Fokus-Kamera</span>
        <select v-model="settings['viewer.layout.focus_slot_id']">
          <option v-for="slot in viewerSlots" :key="slot.id" :value="slot.id">{{ slot.label }}</option>
        </select>
      </div>
      <div v-if="viewerLayoutUsesSplit" class="field">
        <span class="lbl">Fokus-Seite</span>
        <select v-model="settings['viewer.layout.mode']">
          <option value="focus_right">Rechts</option>
          <option value="focus_middle">Mitte</option>
          <option value="focus_left">Links</option>
          <option value="auto">Auto</option>
        </select>
      </div>
      <div v-if="viewerLayoutUsesSplit" class="field">
        <span class="lbl">Raster-Anteil · %</span>
        <input v-model="settings['viewer.layout.split_percent']" type="number" min="30" max="76" />
      </div>
      <div class="field">
        <span class="lbl">Gap · px</span>
        <input v-model="settings['viewer.layout.gap_px']" type="number" min="2" max="20" />
      </div>
      <div class="field">
        <span class="lbl">Performance</span>
        <select v-model="settings['viewer.performance.mode']">
          <option v-for="option in viewerPerformanceOptions" :key="option.id" :value="option.id">{{ option.name }}</option>
        </select>
      </div>
    </div>

    <div class="layout-admin-actions">
      <RouterLink class="btn sm" :to="{ path: '/', query: kioskLayoutQuery }">Kiosk-URL</RouterLink>
      <div class="mono-mute">{{ viewerLayoutDescription }}</div>
    </div>
  </section>

  <!-- Section: Access -->
  <section class="panel">
    <div class="panel-head">
      <h2>Zugriff</h2>
      <div class="right">{{ authStatus?.enabled ? 'Login aktiv' : 'Noch offen' }}</div>
    </div>

    <div class="split">
      <div class="field">
        <span class="lbl">Admin-Login</span>
        <div class="btn-row" style="align-items: stretch;">
          <input v-model="adminPassword" type="password" :placeholder="settings.auth_admin_password_set === 'true' ? 'Neues Admin-Passwort' : 'Admin-Passwort setzen'" style="flex: 1;" />
          <button class="btn" :disabled="!adminPassword || savingAuthPassword === 'admin'" @click="saveAuthPassword('admin')">
            {{ savingAuthPassword === 'admin' ? 'Speichert…' : 'Speichern' }}
          </button>
        </div>
        <div class="mono-mute" style="margin-top: 6px;">
          {{ settings.auth_admin_password_set === 'true' ? 'Admin-Passwort ist gesetzt.' : 'Noch kein Admin-Passwort gesetzt.' }}
        </div>
      </div>

      <div class="field">
        <span class="lbl">Viewer-Login</span>
        <div class="btn-row" style="align-items: stretch;">
          <input v-model="viewerPassword" type="password" :placeholder="settings.auth_viewer_password_set === 'true' ? 'Neues Viewer-Passwort' : 'Viewer-Passwort setzen'" style="flex: 1;" />
          <button class="btn" :disabled="!viewerPassword || savingAuthPassword === 'viewer'" @click="saveAuthPassword('viewer')">
            {{ savingAuthPassword === 'viewer' ? 'Speichert…' : 'Speichern' }}
          </button>
        </div>
        <div class="mono-mute" style="margin-top: 6px;">
          {{ settings.auth_viewer_password_set === 'true' ? 'Viewer-Passwort ist gesetzt.' : 'Viewer-Login ist noch nicht eingerichtet.' }}
        </div>
      </div>

      <div class="field">
        <span class="lbl">Session-Dauer · Stunden</span>
        <input v-model="settings['auth.session_hours']" type="number" min="1" max="168" />
      </div>
    </div>

    <div style="display: grid; gap: 8px;">
      <label class="toggle-row">
        <input type="checkbox" :checked="settings['auth.viewer_public'] === 'true'" @change="setBool('auth.viewer_public', $event)" />
        <div>
          <div class="lbl-main">Viewer ohne Login erlauben</div>
          <div class="lbl-sub">Nur die Kameraansicht bleibt ohne Anmeldung erreichbar; Admin-Funktionen bleiben geschützt.</div>
        </div>
      </label>
      <label class="toggle-row">
        <input type="checkbox" :checked="settings['auth.local_admin_bypass'] === 'true'" @change="setBool('auth.local_admin_bypass', $event)" />
        <div>
          <div class="lbl-main">Lokalen Host als Admin akzeptieren</div>
          <div class="lbl-sub">Zugriffe direkt von 127.0.0.1 dürfen ohne Passwort konfigurieren.</div>
        </div>
      </label>
    </div>
  </section>

  <!-- Section: Watchdog -->
  <section class="panel">
    <div class="panel-head">
      <h2>Watchdog</h2>
      <div class="right">{{ watchdogEnabled ? 'Aktiv' : 'Deaktiviert' }}</div>
    </div>

    <div style="display: grid; gap: 8px;">
      <label class="toggle-row">
        <input type="checkbox" :checked="watchdogEnabled" @change="setBool('watchdog.enabled', $event)" />
        <div>
          <div class="lbl-main">Watchdog aktiv</div>
          <div class="lbl-sub">Prüft go2rtc, aktive Kamera-Pfade und Relay-Fallbacks im Hintergrund.</div>
        </div>
      </label>
      <label class="toggle-row">
        <input type="checkbox" :checked="watchdogRestartOnChange" @change="setBool('watchdog.restart_on_change', $event)" />
        <div>
          <div class="lbl-main">go2rtc bei Pfadwechsel neu starten</div>
          <div class="lbl-sub">Automatische Pfadwechsel werden direkt in den Streams wirksam.</div>
        </div>
      </label>
      <label class="toggle-row">
        <input type="checkbox" :checked="watchdogRestartGo2RTC" @change="setBool('watchdog.restart_go2rtc_on_failure', $event)" />
        <div>
          <div class="lbl-main">go2rtc bei Ausfall neu starten</div>
          <div class="lbl-sub">Wenn die go2rtc-API nicht erreichbar ist, versucht der Watchdog einen Neustart.</div>
        </div>
      </label>
    </div>

    <div class="split">
      <div class="field">
        <span class="lbl">Schneller Check · Sekunden</span>
        <input v-model="settings['watchdog.fast_interval_seconds']" type="number" min="5" max="3600" />
      </div>
      <div class="field">
        <span class="lbl">Kamera-Pfade · Sekunden</span>
        <input v-model="settings['watchdog.camera_interval_seconds']" type="number" min="10" max="7200" />
      </div>
      <div class="field">
        <span class="lbl">Fehler bis Wechsel</span>
        <input v-model="settings['camera.path.fail_threshold']" type="number" min="1" max="20" />
      </div>
      <div class="field">
        <span class="lbl">Erfolge bis Rückwechsel</span>
        <input v-model="settings['camera.path.recovery_threshold']" type="number" min="1" max="20" />
      </div>
      <div class="field">
        <span class="lbl">Restart-Cooldown · Sekunden</span>
        <input v-model="settings['camera.path.restart_cooldown_seconds']" type="number" min="0" max="7200" />
      </div>
    </div>

    <dl class="spec watchdog-spec">
      <div>
        <dt>Letzter Lauf</dt>
        <dd>{{ watchdogDate(status?.watchdog?.last_run_at) }}</dd>
      </div>
      <div>
        <dt>Nächster Lauf</dt>
        <dd>{{ watchdogDate(status?.watchdog?.next_run_at) }}</dd>
      </div>
      <div>
        <dt>Letzte Aktion</dt>
        <dd>{{ status?.watchdog?.last_action || 'Noch keine Aktion.' }}</dd>
      </div>
      <div>
        <dt>Letzter Fehler</dt>
        <dd>{{ status?.watchdog?.last_error || 'Kein Fehler.' }}</dd>
      </div>
      <div>
        <dt>Restart-Cooldown</dt>
        <dd>{{ restartCooldownLabel }}</dd>
      </div>
    </dl>
  </section>

  <!-- Section: Relays and stream paths -->
  <section class="panel">
    <div class="panel-head">
      <h2>Relays und Pfade</h2>
      <div class="device-head-actions">
        <div class="right">{{ relayIds.length }} Relay{{ relayIds.length === 1 ? '' : 's' }}</div>
        <button class="btn sm primary" type="button" @click="addRelay">Relay hinzufügen</button>
      </div>
    </div>

    <div class="split">
      <div class="field">
        <span class="lbl">Relay-ID</span>
        <input v-model="relayDraft.id" placeholder="nas" />
      </div>
      <div class="field">
        <span class="lbl">Name</span>
        <input v-model="relayDraft.name" placeholder="NAS Relay" />
      </div>
      <div class="field">
        <span class="lbl">Host aus go2rtc-Docker</span>
        <input v-model="relayDraft.host" placeholder="host.docker.internal" />
      </div>
      <div class="field">
        <span class="lbl">SSH-Ziel</span>
        <input v-model="relayDraft.sshTarget" placeholder="nas oder user@nas" />
      </div>
    </div>

    <div v-if="!relayIds.length" class="empty">Noch keine Relays definiert. Legacy-Overrides werden weiter unterstützt.</div>
    <div v-else class="relay-config-list">
      <div v-for="relayId in relayIds" :key="relayId" class="relay-config">
        <div class="relay-config-head">
          <div>
            <div class="slot">Relay</div>
            <div class="name">{{ relayName(relayId) }}</div>
            <div class="mono-mute">{{ relayId }}</div>
          </div>
          <div class="btn-row">
            <button class="btn sm" type="button" :disabled="relayActionBusy === `start:${relayId}`" @click="relayAction(relayId, 'start')">
              {{ relayActionBusy === `start:${relayId}` ? 'Startet…' : 'Start' }}
            </button>
            <button class="btn sm ghost" type="button" :disabled="relayActionBusy === `stop:${relayId}`" @click="relayAction(relayId, 'stop')">
              {{ relayActionBusy === `stop:${relayId}` ? 'Stoppt…' : 'Stop' }}
            </button>
            <button class="btn sm ghost" type="button" :disabled="relayActionBusy === `restart:${relayId}`" @click="relayAction(relayId, 'restart')">
              {{ relayActionBusy === `restart:${relayId}` ? 'Startet…' : 'Restart' }}
            </button>
            <button class="btn sm danger" type="button" @click="removeRelay(relayId)">Entfernen</button>
          </div>
        </div>

        <div class="relay-config-grid">
          <div class="field">
            <span class="lbl">Name</span>
            <input v-model="settings[relaySettingKey(relayId, 'name')]" class="compact-input" :placeholder="relayId" />
          </div>
          <div class="field">
            <span class="lbl">Typ</span>
            <select v-model="settings[relaySettingKey(relayId, 'type')]">
              <option value="ssh_local_forward">SSH Local Forward</option>
            </select>
          </div>
          <div class="field">
            <span class="lbl">go2rtc-Host</span>
            <input v-model="settings[relaySettingKey(relayId, 'host')]" class="compact-input" placeholder="host.docker.internal" />
          </div>
          <div class="field">
            <span class="lbl">SSH-Ziel</span>
            <input v-model="settings[relaySettingKey(relayId, 'ssh_target')]" class="compact-input" placeholder="nas oder user@nas" />
          </div>
          <div class="field">
            <span class="lbl">Bind-Adresse</span>
            <input v-model="settings[relaySettingKey(relayId, 'bind_host')]" class="compact-input" placeholder="127.0.0.1" />
          </div>
          <label class="toggle-row relay-auto">
            <input type="checkbox" :checked="relayAutoStart(relayId)" @change="setBool(relaySettingKey(relayId, 'auto_start'), $event)" />
            <div>
              <div class="lbl-main">Auto-Start</div>
              <div class="lbl-sub">Watchdog startet diesen Relay bei Ausfall erneut.</div>
            </div>
          </label>
        </div>

        <div class="relay-runtime" :class="relayStateClass(relayId)">
          <span class="state-dot"></span>
          <div>
            <div class="lbl-main">{{ relayStateLabel(relayId) }}</div>
            <div class="lbl-sub">{{ relayStatusFor(relayId)?.message || 'Noch kein Status.' }}</div>
            <div v-if="relayStatusFor(relayId)?.last_error" class="mono-mute">Fehler · {{ relayStatusFor(relayId)?.last_error }}</div>
          </div>
          <div class="mono-mute">{{ relayStatusFor(relayId)?.pid ? `PID ${relayStatusFor(relayId)?.pid}` : relayStatusFor(relayId)?.log_path || '' }}</div>
        </div>
      </div>
    </div>

    <div class="panel-subhead">
      <h3>Kamera-Pfade</h3>
      <div class="right">Auto versucht den letzten funktionierenden Pfad, dann direkt und Relays.</div>
    </div>

    <div v-if="!cameraBindings.length" class="empty">Noch keine Kameras zugeordnet.</div>
    <div v-else class="relay-camera-list">
      <div v-for="binding in cameraBindings" :key="binding.device_id" class="relay-camera">
        <div class="relay-camera-main">
          <div>
            <div class="name">{{ binding.label || binding.slot?.label || binding.slot_id }}</div>
            <div class="mono-mute">{{ binding.device?.last_ip || 'keine IP' }} · {{ binding.stream_name || 'stream2' }}</div>
          </div>
          <select v-model="settings[pathPolicyKey(binding.device_id)]">
            <option value="auto">Auto</option>
            <option value="prefer_direct">Direkt bevorzugen</option>
            <option value="prefer_relay">Relay bevorzugen</option>
            <option value="direct_only">Nur direkt</option>
            <option value="relay_only">Nur Relay</option>
          </select>
        </div>

        <div v-if="legacyRelayHost(binding.device_id)" class="legacy-path">
          Legacy-Relay aktiv · {{ legacyRelayHost(binding.device_id) }}:{{ legacyRelayPort(binding.device_id) }}
        </div>

        <div v-if="relayIds.length" class="relay-endpoints">
          <div v-for="relayId in relayIds" :key="`${binding.device_id}-${relayId}`" class="relay-endpoint-row">
            <span>{{ relayName(relayId) }}</span>
            <input
              v-model="settings[relayEndpointKey(binding.device_id, relayId, 'host')]"
              :placeholder="relayHost(relayId) || 'Host'"
            />
            <input
              v-model="settings[relayEndpointKey(binding.device_id, relayId, 'port')]"
              placeholder="Lokaler Port"
            />
            <input
              v-model="settings[relayEndpointKey(binding.device_id, relayId, 'target_host')]"
              :placeholder="binding.device?.last_ip || 'Ziel-IP'"
            />
            <input
              v-model="settings[relayEndpointKey(binding.device_id, relayId, 'target_port')]"
              placeholder="554"
            />
            <span class="endpoint-state" :class="relayEndpointStateClass(binding.device_id, relayId)">
              {{ relayEndpointStateLabel(binding.device_id, relayId) }}
            </span>
          </div>
        </div>
      </div>
    </div>
  </section>

  <!-- Section: Credential identities -->
  <section class="panel">
    <div class="panel-head">
      <h2>Kamera-Identitäten</h2>
      <div class="device-head-actions">
        <div class="right">{{ credentialIdentities.length }} gespeichert</div>
        <button class="btn icon sm" type="button" title="Identität hinzufügen" @click="openNewIdentityModal">+</button>
      </div>
    </div>

    <div class="mono-mute">
      Identitäten sind wiederverwendbare Logins. Stream-Auswahl bleibt an Kamera, Zuordnung oder Bildtest.
    </div>

    <div v-if="!credentialIdentities.length" class="empty">Noch keine Identitäten gespeichert.</div>
    <div v-else class="result-list">
      <div v-for="identity in credentialIdentities" :key="identity.id" class="result-row ok identity-row">
        <span class="slot">Login</span>
        <span class="name">{{ identity.name }}</span>
        <span class="ip">{{ identity.username }}</span>
        <span class="stream">{{ identity.password_set ? passwordSourceLabel(identity.password_source) : 'kein Passwort' }}</span>
        <button class="btn sm ghost" type="button" @click="editCredentialIdentity(identity)">Bearbeiten</button>
        <button class="btn sm danger" type="button" @click="deleteCredentialIdentity(identity.id)">Entfernen</button>
      </div>
    </div>
  </section>

  <div v-if="showIdentityModal" class="modal-backdrop" @click.self="closeIdentityModal">
    <form class="modal" @submit.prevent="saveCredentialIdentity">
      <div class="modal-head">
        <div>
          <div class="eyebrow">Kamera-Identitäten</div>
          <h2>{{ identityForm.id ? 'Identität bearbeiten' : 'Identität hinzufügen' }}</h2>
        </div>
        <button class="btn icon sm ghost" type="button" title="Schließen" @click="closeIdentityModal">×</button>
      </div>
      <div class="split">
        <div class="field">
          <span class="lbl">Name</span>
          <input v-model="identityForm.name" placeholder="Tapo Außenkameras" autofocus />
        </div>
        <div class="field">
          <span class="lbl">Benutzername</span>
          <input v-model="identityForm.username" placeholder="Kamera-Benutzer" />
        </div>
        <div class="field">
          <span class="lbl">Passwort</span>
          <input v-model="identityForm.password" type="password" :placeholder="identityForm.id ? 'leer lassen, um Passwort zu behalten' : 'Kamera-Passwort'" />
        </div>
      </div>
      <div class="modal-foot">
        <span class="mono-mute">Wird beim Bildtest auf passende Kameras ausprobiert.</span>
        <div class="btn-row">
          <button class="btn ghost" type="button" @click="closeIdentityModal">Abbrechen</button>
          <button class="btn primary" type="submit" :disabled="savingIdentity || !identityForm.name || !identityForm.username">
            {{ savingIdentity ? 'Speichert…' : 'Speichern' }}
          </button>
        </div>
      </div>
    </form>
  </div>

  <!-- Section: Support bundle -->
  <section class="panel">
    <div class="panel-head">
      <h2>Support-Bundle</h2>
      <div class="right">Status · Viewer · Netzwerk · Logs</div>
    </div>

    <div class="split">
      <div class="field">
        <span class="lbl">Diagnosepaket</span>
        <div class="btn-row">
          <button class="btn primary" :disabled="creatingSupportBundle" @click="createSupportBundle">
            {{ creatingSupportBundle ? 'Erstellt…' : 'Support-Bundle erstellen' }}
          </button>
        </div>
      </div>
      <div class="field">
        <span class="lbl">Version</span>
        <div class="mono-mute">{{ versionDetail }}</div>
      </div>
    </div>

    <div v-if="supportBundleResult" class="notice ok">
      <span class="tag">FERTIG</span>
      <div class="support-result">
        <div>{{ supportBundleResult.path }}</div>
        <div class="mono-mute">{{ supportBundleResult.files.length }} Dateien · Zugangsdaten maskiert</div>
        <div v-if="supportBundleResult.warning" class="mono-mute">{{ supportBundleResult.warning }}</div>
      </div>
    </div>
  </section>

  <!-- Section: Backup -->
  <section class="panel">
    <div class="panel-head">
      <h2>Sicherung</h2>
      <div class="right">Lokale Konfiguration · Bindings · Einstellungen</div>
    </div>

    <div class="split">
      <div class="field">
        <span class="lbl">Backup erstellen</span>
        <div class="btn-row">
          <button class="btn primary" @click="createBackup">Backup jetzt erstellen</button>
        </div>
      </div>
      <div class="field">
        <span class="lbl">Backup wiederherstellen</span>
        <div class="btn-row" style="align-items: stretch;">
          <input v-model="restorePath" placeholder="/var/lib/camera-appliance/backups/…" style="flex: 1;" />
          <button class="btn" :disabled="!restorePath" @click="restoreBackup">Wiederherstellen</button>
        </div>
      </div>
    </div>

    <div v-if="backupResult" class="notice ok">
      <span class="tag">FERTIG</span>
      <div>
        <div>{{ backupResult.path }}</div>
        <div v-if="backupResult.warning" class="mono-mute" style="margin-top: 4px;">{{ backupResult.warning }}</div>
      </div>
    </div>
  </section>

  <!-- Section: Events -->
  <section class="panel">
    <div class="panel-head">
      <h2>Ereignisprotokoll</h2>
      <div class="right">{{ events.length }} Einträge</div>
    </div>
    <div v-if="!events.length" class="empty">Noch keine Ereignisse vorhanden.</div>
    <div v-else class="ticker">
      <div v-for="ev in events" :key="ev.id" class="row">
        <span class="time">{{ formatTime(ev.created_at) }}</span>
        <span class="lvl" :class="levelClass(ev.level)">{{ ev.level }}</span>
        <span><b style="color: var(--ink); font-weight: 500;">{{ ev.type }}</b> · {{ ev.message }}</span>
      </div>
    </div>
  </section>

  <div class="toast-host">
    <transition name="page"><div v-if="toast" class="toast" :key="toast">{{ toast }}</div></transition>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { api } from '../api/client'
import type {
  AuthRole,
  AuthStatus,
  CredentialIdentity,
  EventItem,
  RelayStatus,
  Slot,
  StatusResponse,
  SupportBundleResult,
  ViewerLayoutID,
  ViewerLayoutOption,
  ViewerPerformanceMode,
  ViewerPerformanceOption
} from '../types'

const viewerLayoutOptions: ViewerLayoutOption[] = [
  { id: 'grid_2x2', name: '2x2', description: 'Vier gleich große Kameras im Raster.' },
  { id: 'four_plus_large', name: '4 plus groß', description: 'Vier Raster-Kameras mit einer prominenten Ansicht.' },
  { id: 'vertical_plus_grid', name: 'Vertikal plus Raster', description: 'Eine hochformatige Kamera neben einem Raster.' },
  { id: 'large_only', name: 'Große Ansicht', description: 'Nur die prominente Kamera bildschirmfüllend.' },
  { id: 'custom', name: 'Frei', description: 'Kameras per Drag-and-drop auf Zonen und Größen legen.' }
]
const viewerPerformanceOptions: ViewerPerformanceOption[] = [
  { id: 'quality', name: 'Qualität', description: 'Alle sichtbaren Streams sofort live laden.' },
  { id: 'balanced', name: 'Balanciert', description: 'Nebenansichten lazy laden und primäre Ansicht priorisieren.' },
  { id: 'low', name: 'Niedrig', description: 'Nur die primäre Ansicht live laden, Nebenansichten pausieren.' },
  { id: 'diagnostic', name: 'Diagnose', description: 'Alle Streams live laden und Producer/Consumer sichtbar machen.' }
]

const settings = reactive<Record<string, string>>({})
const events = ref<EventItem[]>([])
const status = ref<StatusResponse>()
const authStatus = ref<AuthStatus>()
const credentialIdentities = ref<CredentialIdentity[]>([])
const restorePath = ref('')
const backupResult = ref<{ path: string; warning: string }>()
const supportBundleResult = ref<SupportBundleResult>()
const error = ref('')
const toast = ref('')
const cameraPassword = ref('')
const adminPassword = ref('')
const viewerPassword = ref('')
const savingPassword = ref(false)
const savingAuthPassword = ref<AuthRole | ''>('')
const creatingSupportBundle = ref(false)
const savingIdentity = ref(false)
const showIdentityModal = ref(false)
const passwordSource = ref('unbekannt')
const identityForm = reactive({ id: '', name: '', username: '', password: '' })
const relayDraft = reactive({ id: 'nas', name: 'NAS Relay', host: 'host.docker.internal', sshTarget: 'nas' })
const relayActionBusy = ref('')

const relayIds = computed(() => settingList(settings['camera.relay.ids']))
const relayStatuses = computed(() => status.value?.relays ?? [])
const viewerSlots = computed<Slot[]>(() => status.value?.slots ?? [])
const viewerLayoutID = computed(() => normalizedViewerLayoutID(settings['viewer.layout.id'] || layoutIDFromMode(settings['viewer.layout.mode'])))
const viewerLayout = computed(() => viewerLayoutOptions.find((layout) => layout.id === viewerLayoutID.value) || viewerLayoutOptions[1])
const viewerLayoutName = computed(() => viewerLayout.value.name)
const viewerLayoutDescription = computed(() => viewerLayout.value.description)
const viewerLayoutUsesSplit = computed(() => viewerLayoutID.value === 'four_plus_large' || viewerLayoutID.value === 'vertical_plus_grid')
const kioskLayoutQuery = computed(() => {
  const query: Record<string, string> = { layout: viewerLayoutID.value }
  const performance = normalizedViewerPerformanceMode(settings['viewer.performance.mode'])
  if (performance !== 'quality') query.perf = performance
  if (viewerLayoutUsesSplit.value) {
    if (settings['viewer.layout.mode'] === 'focus_left') query.side = 'left'
    else if (settings['viewer.layout.mode'] === 'focus_middle') query.side = 'middle'
    else query.side = 'right'
  }
  return query
})
const cameraBindings = computed(() => (status.value?.bindings ?? []).filter((binding) => binding.device_id))
const watchdogEnabled = computed(() => boolSetting('watchdog.enabled', status.value?.watchdog?.enabled ?? true))
const watchdogRestartOnChange = computed(() => boolSetting('watchdog.restart_on_change', status.value?.watchdog?.restart_on_change ?? true))
const watchdogRestartGo2RTC = computed(() => boolSetting('watchdog.restart_go2rtc_on_failure', status.value?.watchdog?.restart_go2rtc_on_failure ?? true))
const restartCooldownLabel = computed(() => {
  const watchdog = status.value?.watchdog
  if (!watchdog) return 'Noch kein Status.'
  if (watchdog.path_restart_pending) return `Ausstehend bis ${watchdogDate(watchdog.path_restart_cooldown_until)}`
  if (watchdog.path_restart_last_at) return `Letzter Restart ${watchdogDate(watchdog.path_restart_last_at)}`
  return 'Kein Cooldown aktiv.'
})
const versionLabel = computed(() => {
  const info = status.value?.version
  if (!info) return 'dev'
  const version = info.version || 'dev'
  const commit = info.commit && info.commit !== 'local' ? ` (${info.commit})` : ''
  return `${version}${commit}`
})
const versionDetail = computed(() => {
  const info = status.value?.version
  if (!info) return 'dev'
  const parts = [info.version || 'dev']
  if (info.commit) parts.push(`Commit ${info.commit}`)
  if (info.build_time) parts.push(`Build ${info.build_time}`)
  return parts.join(' · ')
})

function setBool(key: string, e: Event) {
  const target = e.target as HTMLInputElement
  settings[key] = target.checked ? 'true' : 'false'
}

function formatTime(t: string) {
  return new Date(t).toLocaleString('de-DE', {
    day: '2-digit', month: '2-digit',
    hour: '2-digit', minute: '2-digit'
  })
}
function levelClass(l: string) {
  const lower = (l || '').toLowerCase()
  if (lower.includes('err') || lower.includes('fail')) return 'err'
  if (lower.includes('warn')) return 'warn'
  if (lower.includes('ok') || lower.includes('info')) return 'ok'
  return ''
}
function passwordSourceLabel(source?: string) {
  if (!source) return 'Passwort gespeichert'
  if (source === 'keyring') return 'Keyring'
  if (source === 'local.env') return 'Secret-Datei'
  return source
}
function boolSetting(key: string, fallback: boolean) {
  const raw = settings[key]
  if (raw === undefined || raw === '') return fallback
  return raw === 'true' || raw === '1' || raw === 'yes' || raw === 'on'
}

function watchdogDate(value?: string) {
  if (!value) return 'Noch nicht gelaufen.'
  return formatTime(value)
}

function settingList(raw?: string) {
  const seen = new Set<string>()
  return (raw || '')
    .split(',')
    .map((part) => part.trim())
    .filter((part) => {
      if (!part || seen.has(part)) return false
      seen.add(part)
      return true
    })
}

function sanitizeID(value: string) {
  return value.trim().toLowerCase().replace(/[^a-z0-9_]+/g, '_').replace(/^_+|_+$/g, '')
}

function normalizedViewerLayoutID(raw?: string): ViewerLayoutID {
  if (raw === 'grid_2x2' || raw === 'four_plus_large' || raw === 'vertical_plus_grid' || raw === 'large_only' || raw === 'custom') return raw
  return 'four_plus_large'
}

function layoutIDFromMode(raw?: string): ViewerLayoutID {
  if (raw === 'grid_2x2' || raw === 'vertical_plus_grid' || raw === 'large_only' || raw === 'custom') return raw
  return 'four_plus_large'
}

function normalizedViewerPerformanceMode(raw?: string): ViewerPerformanceMode {
  if (raw === 'balanced' || raw === 'low' || raw === 'diagnostic') return raw
  return 'quality'
}

function defaultViewerLayoutMode(id: ViewerLayoutID) {
  if (id === 'vertical_plus_grid') return 'focus_right'
  if (id === 'four_plus_large') {
    if (settings['viewer.layout.mode'] === 'focus_left' || settings['viewer.layout.mode'] === 'focus_middle' || settings['viewer.layout.mode'] === 'focus_right') return settings['viewer.layout.mode']
    return 'auto'
  }
  return id
}

function defaultViewerLayoutSplit(id: ViewerLayoutID) {
  return id === 'vertical_plus_grid' ? '64' : '58'
}

function setViewerLayoutFromEvent(event: Event) {
  const target = event.target as HTMLSelectElement
  setViewerLayoutID(normalizedViewerLayoutID(target.value))
}

function setViewerLayoutID(id: ViewerLayoutID) {
  settings['viewer.layout.id'] = id
  settings['viewer.layout.mode'] = defaultViewerLayoutMode(id)
  if (!settings['viewer.layout.split_percent']) settings['viewer.layout.split_percent'] = defaultViewerLayoutSplit(id)
  if (!settings['viewer.layout.gap_px']) settings['viewer.layout.gap_px'] = '8'
  if (!settings['viewer.layout.focus_slot_id']) settings['viewer.layout.focus_slot_id'] = defaultViewerFocusSlotID()
}

function defaultViewerFocusSlotID() {
  return viewerSlots.value.find((slot) => slot.role === 'large')?.id || viewerSlots.value[viewerSlots.value.length - 1]?.id || 'cam5'
}

function relaySettingKey(id: string, field: string) {
  return `camera.relay.${id}.${field}`
}

function relayEndpointKey(deviceId: string, relayId: string, field: string) {
  return `camera.relay_endpoint.${deviceId}.${relayId}.${field}`
}

function pathPolicyKey(deviceId: string) {
  return `camera.path_policy.${deviceId}`
}

function relayName(id: string) {
  return settings[relaySettingKey(id, 'name')] || id
}

function relayHost(id: string) {
  return settings[relaySettingKey(id, 'host')] || ''
}

function relayAutoStart(id: string) {
  return boolSetting(relaySettingKey(id, 'auto_start'), relayStatusFor(id)?.auto_start ?? false)
}

function relayStatusFor(id: string): RelayStatus | undefined {
  return relayStatuses.value.find((relay) => relay.id === id)
}

function relayStateLabel(id: string) {
  const state = relayStatusFor(id)?.process_state || 'unknown'
  const labels: Record<string, string> = {
    running: 'Läuft',
    stopped: 'Gestoppt',
    stale: 'Prozess beendet',
    unmanaged: 'Manuell',
    external: 'Externer Forward',
    backoff: 'Backoff',
    error: 'Fehler',
    not_configured: 'Unvollständig',
    unsupported: 'Nicht unterstützt',
    disabled: 'Deaktiviert',
    unknown: 'Unbekannt'
  }
  return labels[state] || state
}

function relayStateClass(id: string) {
  const state = relayStatusFor(id)?.process_state
  if (state === 'running' || state === 'external') return 'ok'
  if (state === 'error' || state === 'stale' || state === 'not_configured') return 'err'
  if (state === 'backoff' || state === 'stopped') return 'warn'
  return ''
}

function relayEndpointStatus(deviceId: string, relayId: string) {
  return relayStatusFor(relayId)?.endpoints.find((endpoint) => endpoint.device_id === deviceId)
}

function relayEndpointStateLabel(deviceId: string, relayId: string) {
  const endpoint = relayEndpointStatus(deviceId, relayId)
  if (!endpoint) return 'kein Status'
  if (endpoint.state === 'ok') return 'Port ok'
  if (endpoint.state === 'failed') return 'Port offline'
  return 'unvollständig'
}

function relayEndpointStateClass(deviceId: string, relayId: string) {
  const state = relayEndpointStatus(deviceId, relayId)?.state
  if (state === 'ok') return 'ok'
  if (state === 'failed') return 'err'
  return 'warn'
}

function legacyRelayHost(deviceId: string) {
  return settings[`camera.rtsp_endpoint.${deviceId}.host`] || ''
}

function legacyRelayPort(deviceId: string) {
  return settings[`camera.rtsp_endpoint.${deviceId}.port`] || '554'
}

function addRelay() {
  const id = sanitizeID(relayDraft.id)
  if (!id) {
    error.value = 'Relay-ID fehlt.'
    return
  }
  const ids = relayIds.value.includes(id) ? relayIds.value : [...relayIds.value, id]
  settings['camera.relay.ids'] = ids.join(',')
  settings[relaySettingKey(id, 'name')] = relayDraft.name.trim() || id
  settings[relaySettingKey(id, 'type')] = 'ssh_local_forward'
  settings[relaySettingKey(id, 'host')] = relayDraft.host.trim()
  settings[relaySettingKey(id, 'bind_host')] = '127.0.0.1'
  settings[relaySettingKey(id, 'ssh_target')] = relayDraft.sshTarget.trim()
  settings[relaySettingKey(id, 'auto_start')] = settings[relaySettingKey(id, 'auto_start')] || 'false'
  relayDraft.id = ''
  relayDraft.name = ''
  relayDraft.host = ''
  relayDraft.sshTarget = ''
}

function ensurePathPolicyDefaults() {
  for (const binding of cameraBindings.value) {
    const key = pathPolicyKey(binding.device_id)
    if (!settings[key]) settings[key] = 'auto'
  }
}

function ensureWatchdogDefaults() {
  const watchdog = status.value?.watchdog
  if (!watchdog) return
  if (!settings['watchdog.enabled']) settings['watchdog.enabled'] = String(watchdog.enabled)
  if (!settings['watchdog.restart_on_change']) settings['watchdog.restart_on_change'] = String(watchdog.restart_on_change)
  if (!settings['watchdog.restart_go2rtc_on_failure']) settings['watchdog.restart_go2rtc_on_failure'] = String(watchdog.restart_go2rtc_on_failure)
  if (!settings['watchdog.fast_interval_seconds']) settings['watchdog.fast_interval_seconds'] = String(watchdog.fast_interval_seconds)
  if (!settings['watchdog.camera_interval_seconds']) settings['watchdog.camera_interval_seconds'] = String(watchdog.camera_interval_seconds)
  if (!settings['camera.path.fail_threshold']) settings['camera.path.fail_threshold'] = String(watchdog.path_fail_threshold)
  if (!settings['camera.path.recovery_threshold']) settings['camera.path.recovery_threshold'] = String(watchdog.path_recovery_threshold)
  if (!settings['camera.path.restart_cooldown_seconds']) settings['camera.path.restart_cooldown_seconds'] = String(watchdog.path_restart_cooldown_seconds)
}

function ensureAuthDefaults() {
  const auth = authStatus.value
  if (!auth) return
  settings.auth_admin_password_set = String(auth.admin_password_set)
  settings.auth_viewer_password_set = String(auth.viewer_password_set)
  if (!settings['auth.viewer_public']) settings['auth.viewer_public'] = String(auth.viewer_public)
  if (!settings['auth.local_admin_bypass']) settings['auth.local_admin_bypass'] = String(auth.local_admin_bypass)
  if (!settings['auth.session_hours']) settings['auth.session_hours'] = String(auth.session_hours || 12)
}

function ensureRelayDefaults() {
  for (const relayId of relayIds.value) {
    if (!settings[relaySettingKey(relayId, 'type')]) settings[relaySettingKey(relayId, 'type')] = 'ssh_local_forward'
    if (!settings[relaySettingKey(relayId, 'bind_host')]) settings[relaySettingKey(relayId, 'bind_host')] = relayStatusFor(relayId)?.bind_host || '127.0.0.1'
    if (!settings[relaySettingKey(relayId, 'auto_start')]) settings[relaySettingKey(relayId, 'auto_start')] = String(relayStatusFor(relayId)?.auto_start ?? false)
  }
}

function ensureViewerLayoutDefaults() {
  const id = viewerLayoutID.value
  if (!settings['viewer.layout.id']) settings['viewer.layout.id'] = id
  if (!settings['viewer.layout.mode']) settings['viewer.layout.mode'] = defaultViewerLayoutMode(id)
  if (!settings['viewer.layout.focus_slot_id']) settings['viewer.layout.focus_slot_id'] = defaultViewerFocusSlotID()
  if (!settings['viewer.layout.split_percent']) settings['viewer.layout.split_percent'] = defaultViewerLayoutSplit(id)
  if (!settings['viewer.layout.gap_px']) settings['viewer.layout.gap_px'] = '8'
  if (!settings['viewer.performance.mode']) settings['viewer.performance.mode'] = 'quality'
}

function removeRelay(id: string) {
  settings['camera.relay.ids'] = relayIds.value.filter((relayId) => relayId !== id).join(',')
  delete settings[relaySettingKey(id, 'name')]
  delete settings[relaySettingKey(id, 'type')]
  delete settings[relaySettingKey(id, 'host')]
  delete settings[relaySettingKey(id, 'bind_host')]
  delete settings[relaySettingKey(id, 'ssh_target')]
  delete settings[relaySettingKey(id, 'auto_start')]
}

async function saveSettings() {
  try {
    await api.saveSettings(settings)
    toast.value = 'Einstellungen gespeichert'
    setTimeout(() => (toast.value = ''), 2200)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Speichern fehlgeschlagen.'
  }
}

async function refreshStatus() {
  status.value = await api.status()
  ensurePathPolicyDefaults()
  ensureWatchdogDefaults()
  ensureRelayDefaults()
  ensureViewerLayoutDefaults()
}

async function relayAction(id: string, action: 'start' | 'stop' | 'restart') {
  relayActionBusy.value = `${action}:${id}`
  error.value = ''
  try {
    await api.saveSettings(settings)
    if (action === 'start') await api.startRelay(id)
    if (action === 'stop') await api.stopRelay(id)
    if (action === 'restart') await api.restartRelay(id)
    await refreshStatus()
    toast.value = `Relay ${action === 'restart' ? 'neu gestartet' : action === 'start' ? 'gestartet' : 'gestoppt'}`
    setTimeout(() => (toast.value = ''), 2200)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Relay-Aktion fehlgeschlagen.'
  } finally {
    relayActionBusy.value = ''
  }
}

async function saveCameraPassword() {
  savingPassword.value = true
  error.value = ''
  try {
    const result = await api.saveCameraPassword(cameraPassword.value)
    settings.camera_password_set = 'true'
    settings.camera_password_source = result.source
    passwordSource.value = result.source === 'keyring' ? 'Betriebssystem-Keyring' : result.source
    cameraPassword.value = ''
    toast.value = 'Kamera-Passwort gespeichert'
    setTimeout(() => (toast.value = ''), 2200)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Passwort konnte nicht gespeichert werden.'
  } finally {
    savingPassword.value = false
  }
}

async function loadAuthStatus() {
  authStatus.value = await api.authStatus()
  ensureAuthDefaults()
}

async function saveAuthPassword(role: AuthRole) {
  const password = role === 'admin' ? adminPassword.value : viewerPassword.value
  const wasEnabled = authStatus.value?.enabled ?? false
  savingAuthPassword.value = role
  error.value = ''
  try {
    await api.setAuthPassword({ role, password })
    if (role === 'admin' && !wasEnabled) {
      await api.login({ username: 'admin', password })
      window.dispatchEvent(new Event('auth-changed'))
    }
    if (role === 'admin') {
      adminPassword.value = ''
      settings.auth_admin_password_set = 'true'
    } else {
      viewerPassword.value = ''
      settings.auth_viewer_password_set = 'true'
    }
    await loadAuthStatus()
    toast.value = `${role === 'admin' ? 'Admin' : 'Viewer'}-Passwort gespeichert`
    setTimeout(() => (toast.value = ''), 2200)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Login-Passwort konnte nicht gespeichert werden.'
  } finally {
    savingAuthPassword.value = ''
  }
}

function resetIdentityForm() {
  identityForm.id = ''
  identityForm.name = ''
  identityForm.username = ''
  identityForm.password = ''
}

function openNewIdentityModal() {
  resetIdentityForm()
  showIdentityModal.value = true
}

function closeIdentityModal() {
  if (!savingIdentity.value) showIdentityModal.value = false
}

function editCredentialIdentity(identity: CredentialIdentity) {
  identityForm.id = identity.id
  identityForm.name = identity.name
  identityForm.username = identity.username
  identityForm.password = ''
  showIdentityModal.value = true
}

async function loadCredentialIdentities() {
  credentialIdentities.value = await api.credentialIdentities()
}

async function saveCredentialIdentity() {
  savingIdentity.value = true
  error.value = ''
  try {
    await api.saveCredentialIdentity({
      id: identityForm.id || undefined,
      name: identityForm.name,
      username: identityForm.username,
      password: identityForm.password || undefined
    })
    await loadCredentialIdentities()
    resetIdentityForm()
    showIdentityModal.value = false
    toast.value = 'Identität gespeichert'
    setTimeout(() => (toast.value = ''), 2200)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Identität konnte nicht gespeichert werden.'
  } finally {
    savingIdentity.value = false
  }
}

async function deleteCredentialIdentity(id: string) {
  try {
    await api.deleteCredentialIdentity(id)
    await loadCredentialIdentities()
    if (identityForm.id === id) resetIdentityForm()
    toast.value = 'Identität entfernt'
    setTimeout(() => (toast.value = ''), 2200)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Identität konnte nicht entfernt werden.'
  }
}

async function createBackup() {
  try {
    backupResult.value = await api.backup()
    toast.value = 'Backup erstellt'
    setTimeout(() => (toast.value = ''), 2200)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Backup konnte nicht erstellt werden.'
  }
}

async function createSupportBundle() {
  creatingSupportBundle.value = true
  error.value = ''
  try {
    supportBundleResult.value = await api.supportBundle()
    toast.value = 'Support-Bundle erstellt'
    setTimeout(() => (toast.value = ''), 2200)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Support-Bundle konnte nicht erstellt werden.'
  } finally {
    creatingSupportBundle.value = false
  }
}

async function restoreBackup() {
  try {
    backupResult.value = await api.restore(restorePath.value)
    toast.value = 'Backup wiederhergestellt'
    setTimeout(() => (toast.value = ''), 2200)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Wiederherstellung fehlgeschlagen.'
  }
}

onMounted(async () => {
  try {
    Object.assign(settings, await api.settings())
    passwordSource.value = settings.camera_password_source === 'keyring' ? 'Betriebssystem-Keyring' : (settings.camera_password_source || 'unbekannt')
    await loadAuthStatus()
    await refreshStatus()
    await loadCredentialIdentities()
    events.value = await api.events()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Konnte nicht geladen werden.'
  }
})
</script>
