<template>
  <!-- checking -->
  <p v-if="phase === 'checking'" class="update-hint"><span class="spin" /> Suche nach Updates…</p>

  <!-- up to date -->
  <template v-else-if="phase === 'up_to_date'">
    <p class="update-hint ok">Version {{ status?.current_version || 'unbekannt' }}</p>
    <p v-if="checkedLabel" class="update-meta">Zuletzt geprüft {{ checkedLabel }}</p>
  </template>

  <!-- available: changelog since the installed version -->
  <template v-else-if="phase === 'available'">
    <p class="update-hint">Installiert {{ status?.current_version || 'unbekannt' }} · neu {{ status?.latest?.tag }}</p>
    <p class="update-label">Was hat sich geändert</p>

    <ol class="update-changelog">
      <li v-for="release in changelog" :key="release.tag" class="update-release">
        <h4 class="update-release-head">
          <a v-if="release.href" class="update-release-tag" :href="release.href" target="_blank" rel="noopener noreferrer">
            {{ release.tag }}
          </a>
          <span v-else class="update-release-tag">{{ release.tag }}</span>
          <span v-if="release.date" class="update-release-date">{{ release.date }}</span>
        </h4>

        <ul v-if="release.notes.length" class="update-notes">
          <li
            v-for="(note, i) in release.notes"
            :key="i"
            :class="note.type === 'heading' ? 'update-subhead' : 'update-note'"
          >
            <template v-if="note.type === 'heading'">{{ note.text }}</template>
            <template v-else>
              <span v-if="note.kind" class="note-kind" :class="note.kind">{{ note.kind }}</span>{{ note.text }}<a
                v-if="note.ref && note.refHref"
                class="note-ref"
                :href="note.refHref"
                target="_blank"
                rel="noopener noreferrer"
              >{{ note.ref }}</a><span v-else-if="note.ref" class="note-ref">{{ note.ref }}</span>
            </template>
          </li>
        </ul>
        <p v-else class="update-note-empty">Keine Details veröffentlicht.</p>
      </li>
    </ol>
  </template>

  <!-- downloading -->
  <p v-else-if="phase === 'downloading'" class="update-hint">
    <span class="spin" /> {{ progressText }}
  </p>

  <!-- ready: next press installs -->
  <template v-else-if="phase === 'ready'">
    <p class="update-hint ok">Update bereit ({{ shortDigest }}).</p>
    <p class="update-warn">Bei der Installation werden go2rtc und die Oberfläche neu gestartet.</p>
  </template>

  <!-- installing -->
  <p v-else-if="phase === 'installing'" class="update-hint">
    <span class="spin" /> Installation läuft – die Oberfläche startet gleich neu.
  </p>

  <!-- failed -->
  <p v-else-if="phase === 'failed'" class="update-error">{{ status?.error || 'Etwas ist schiefgelaufen.' }}</p>

  <!-- idle -->
  <p v-else class="update-hint">Version {{ status?.current_version || 'unbekannt' }}</p>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { UpdateFlowPhase, UpdateFlowStatus } from '../types'

const props = defineProps<{ phase: UpdateFlowPhase; status?: UpdateFlowStatus }>()

const MAX_RELEASES = 6
const MAX_NOTES_PER_RELEASE = 10
const CONVENTIONAL = /^(feat|fix|chore|docs|refactor|perf|test|build|ci|style|revert)(\([^)]*\))?!?:\s*/i

type ChangelogNote =
  | { type: 'heading'; text: string }
  | { type: 'item'; kind: string; text: string; ref: string; refHref: string }

interface ChangelogRelease {
  tag: string
  date: string
  href: string
  notes: ChangelogNote[]
}

const checkedLabel = computed(() => {
  const raw = props.status?.checked_at
  if (!raw) return ''
  const at = new Date(raw)
  if (Number.isNaN(at.getTime())) return ''
  return at.toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit' })
})

const shortDigest = computed(() => {
  const digest = props.status?.digest ?? ''
  return digest ? digest.slice(0, 8) + '…' : ''
})

const progressText = computed(() => {
  const total = props.status?.total ?? 0
  const done = props.status?.downloaded ?? 0
  if (total > 0) {
    const percent = Math.min(100, Math.floor((done / total) * 100))
    return `Update wird heruntergeladen … ${percent}%`
  }
  return 'Update wird heruntergeladen …'
})

/* --------------------------------------------------------------------------
   GitHub release notes are markdown. Render them as one titled block per
   release with plain text lines — never v-html. Bare compare/PR URLs are
   collapsed to a "#123" reference so the notes stay readable in a narrow
   sheet. */

const changelog = computed<ChangelogRelease[]>(() => {
  const releases: ChangelogRelease[] = []
  for (const release of (props.status?.changes ?? []).slice(0, MAX_RELEASES)) {
    const notes = parseNotes(release.notes ?? '').slice(0, MAX_NOTES_PER_RELEASE)
    releases.push({
      tag: release.tag || 'Unbenannt',
      date: formatReleaseDate(release.published_at),
      href: safeHref(release.html_url),
      notes
    })
  }
  return releases
})

function parseNotes(raw: string): ChangelogNote[] {
  const notes: ChangelogNote[] = []
  let inFence = false
  for (const line of raw.split('\n')) {
    const trimmed = line.trim()
    if (trimmed.startsWith('```')) {
      inFence = !inFence
      continue
    }
    if (inFence || !trimmed) continue

    const heading = /^#{1,6}\s+(.*)$/.exec(trimmed)
    if (heading) {
      const label = cleanMarkdown(heading[1])
      // GitHub's own section title duplicates the panel label above.
      if (!label || /^what'?s changed$/i.test(label)) continue
      notes.push({ type: 'heading', text: label })
      continue
    }

    let text = trimmed.replace(/^[-*+]\s+/, '')

    // "… by @someone in https://github.com/o/r/pull/14" → linked ref "#14".
    let ref = ''
    let refHref = ''
    const pull = /\bby\s+@[\w-]+\s+in\s+(\S*?\/pull\/(\d+))/i.exec(text)
    if (pull) {
      ref = '#' + pull[2]
      refHref = safeHref(pull[1])
      text = text.slice(0, pull.index)
    }

    text = cleanMarkdown(text)
    // A compare link adds nothing the release titles do not already show.
    if (!text || /^full changelog:?$/i.test(text)) continue

    let kind = ''
    const conventional = CONVENTIONAL.exec(text)
    if (conventional) {
      kind = conventional[1].toLowerCase()
      text = text.slice(conventional[0].length).trim()
    }
    if (!text) continue
    notes.push({ type: 'item', kind, text: truncate(text), ref, refHref })
  }
  return notes
}

function cleanMarkdown(value: string): string {
  return value
    .replace(/!\[[^\]]*\]\([^)]*\)/g, '')      // images
    .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')   // links → their label
    .replace(/https?:\/\/\S+/g, '')             // bare urls
    .replace(/[`*]/g, '')                       // emphasis markers
    .replace(/\s+/g, ' ')
    .replace(/[·:,;\-–—]+$/, '')                // punctuation left dangling
    .trim()
}

function truncate(text: string): string {
  return text.length > 160 ? text.slice(0, 157) + '…' : text
}

// Release notes are third-party text, so only plain http(s) links are ever
// turned into an href — never javascript:, data: or anything else.
function safeHref(raw: string | undefined): string {
  const value = (raw ?? '').trim()
  if (!value) return ''
  try {
    const url = new URL(value)
    return url.protocol === 'https:' || url.protocol === 'http:' ? url.href : ''
  } catch {
    return ''
  }
}

function formatReleaseDate(raw: string | undefined): string {
  if (!raw) return ''
  const at = new Date(raw)
  if (Number.isNaN(at.getTime())) return ''
  return at.toLocaleDateString('de-DE', { day: '2-digit', month: '2-digit', year: 'numeric' })
}
</script>

<style scoped>
.update-hint { margin: 0; font-size: 12px; color: var(--ink-soft); display: flex; align-items: center; gap: 8px; overflow-wrap: anywhere; }
.update-hint.ok { color: var(--live); }
.update-meta { margin: 0; font-size: 10.5px; color: var(--ink-dim); letter-spacing: 0.05em; }
.update-label {
  margin: 0;
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.12em;
  color: var(--ink-dim);
}
.update-changelog {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: 16px;
}
.update-release-head {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin: 0 0 8px;
  padding-bottom: 5px;
  border-bottom: 1px solid var(--hairline);
}
.update-release-tag {
  font-size: 12px;
  font-weight: 500;
  letter-spacing: 0.08em;
  color: var(--ink);
  text-decoration: none;
}
a.update-release-tag:hover { color: var(--live); text-decoration: underline; }
a.update-release-tag:focus-visible {
  outline: 2px solid var(--live);
  outline-offset: 2px;
  border-radius: 2px;
}
.update-release-date {
  margin-left: auto;
  flex: 0 0 auto;
  font-size: 10px;
  letter-spacing: 0.05em;
  color: var(--ink-dim);
}
.update-notes {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: 6px;
  font-size: 12px;
  color: var(--ink-soft);
  line-height: 1.55;
}
.update-note {
  position: relative;
  padding-left: 12px;
  overflow-wrap: anywhere;
}
.update-note::before {
  content: "";
  position: absolute;
  left: 1px;
  top: 0.62em;
  width: 3px;
  height: 3px;
  border-radius: 999px;
  background: var(--ink-dim);
}
.note-kind {
  display: inline-block;
  margin-right: 6px;
  padding: 0 5px;
  border: 1px solid var(--hairline-strong);
  border-radius: 3px;
  font-size: 9px;
  line-height: 15px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--ink-mute);
  vertical-align: 1px;
}
.note-kind.feat { color: var(--live); border-color: rgba(181, 232, 83, 0.35); }
.note-ref {
  margin-left: 6px;
  font-size: 10.5px;
  color: var(--ink-dim);
  text-decoration: none;
  white-space: nowrap;
}
a.note-ref:hover { color: var(--live); text-decoration: underline; }
a.note-ref:focus-visible {
  outline: 2px solid var(--live);
  outline-offset: 2px;
  border-radius: 2px;
}
.update-subhead {
  margin: 4px 0 0;
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.11em;
  color: var(--ink-mute);
}
.update-note-empty { margin: 0; font-size: 11.5px; color: var(--ink-dim); }
.update-warn { margin: 0; font-size: 11.5px; color: var(--warn); line-height: 1.5; }
.update-error { margin: 0; font-size: 12px; color: var(--danger); overflow-wrap: anywhere; }
.spin {
  width: 11px;
  height: 11px;
  flex: 0 0 auto;
  border-radius: 999px;
  border: 2px solid var(--ink-dim);
  border-top-color: transparent;
  animation: update-info-spin .8s linear infinite;
}
@keyframes update-info-spin { to { transform: rotate(360deg); } }

@media (prefers-reduced-motion: reduce) {
  .spin { animation-duration: 2s; }
}
</style>
