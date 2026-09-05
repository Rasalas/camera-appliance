# Admin interface update

The September 2026 update applies the confirmed admin layout to the existing
camera, relay, identity and maintenance workflows. It keeps the Watchdeck
branding and the fullscreen live viewer.

## Navigation and editing

- Desktop resource search sits beneath the logo and groups cameras, relays and
  identities. Cmd/Ctrl+K, arrows, Enter and Escape are supported. The current
  resource type is an optional search scope. Discovery candidates that are not
  cameras do not appear as camera resources.
- `/verwaltung` is a mobile navigation home with every existing sidebar target.
  Bottom navigation links Home, cameras and the live viewer. Search and the
  available login/logout actions use anchored popovers.
- Resource lists are flat. Camera and identity names are real links; row clicks
  ignore controls, links and text selection. Identities now have stable detail
  URLs at `/system/identitaeten/:id`.
- Editable sections share a border, padding and a title/edit header. Short edits
  use a desktop dialog or mobile sheet with an accessible close button. Explicit
  saves retain the draft on errors. Cancel, Escape, outside click and the sheet
  gesture use the same discard confirmation. Route changes and reloads protect
  unsaved drafts.
- Camera access, connection and display edits use `/kamera/:id/bearbeiten`.
  The upload editor uses `/kamera/:id/bild-upload`; the shared server editor is
  `/kameras/bild-upload/bearbeiten`. These long forms keep their own routes and
  return paths. Existing API operations remain in use.
- Upload crop, privacy masks and capture timestamp retain autosave for direct
  image manipulation. Their rollback controls restore the values loaded when
  the editor opened and persist that restoration. Naming, destination directory
  and schedules use explicit saves. A schedule draft does not enable uploads.

No restaurant entities, account profile fields, metrics or unrelated features
were introduced. The app has login roles and passwords, not a personal profile
resource. Fullscreen monitoring remains outside the admin navigation chrome.

## Verification

The browser checks used the real published v0.4.0 manager inside the isolated
Docker fixture documented in `ui-update-verification.md`, overlaid with the
new frontend. Camera records and credentials were synthetic; frame responses
used the repository's courtyard fixture. No customer uploads were made.

Verified in Chromium:

- Search across resource types, scoped results, keyboard opening/navigation,
  focus, stable identity URLs and reload.
- Settings save/reload, cancel without write, retained input after HTTP 500 and
  successful retry. The compact discard dialog's Escape returns to the editor.
- Camera access persistence, protected cancellation, crop rollback, and privacy
  rollback preserving an existing mask while reverting timestamp changes.
- Explicit naming persistence. An edited schedule remains unsubmitted until
  Save; cancel preserves the disabled server schedule. A changed interval can
  be saved while the schedule stays disabled.
- Mobile Home has all twelve destinations. Add/edit actions and search remain
  accessible at 390 px; ordinary content was checked for overflow at 320 px.
- Touch events in Chromium began with the sheet body scrolled 100 px down.
  One continuous downward gesture scrolled to zero, pulled the sheet down and
  opened the protected discard dialog. Continuing editing retained the draft.
- Desktop and mobile screenshots cover settings, cameras, camera details,
  relays, identities, search, Home, the upload editor and a mobile edit sheet.

All 24 frontend tests, the production build, seven Python updater tests, full
Go race tests and `go vet` passed locally. Release CI repeats the build and tests.

## Practical limits

Touch verification uses Chromium touch emulation; physical iOS/Android keyboards
and assistive technology were not available. Camera saves retain the existing
separate credential, settings and binding API operations; they are not a new
backend transaction. The image rollback is scoped to changes since opening the
editor, rather than a persistent cross-session history. No claim of a complete
WCAG audit is made.
