# Administration navigation and support verification

The September 6 follow-up addresses feedback on v0.5.0:

- Home is mobile-only. Desktop `/verwaltung` opens the camera list; resizing
  an open Home page to desktop also redirects there.
- Sidebar destinations have consistent SVG icons, indented children and a
  full-width active background. System and Maintenance are non-link group
  labels. Camera, identity, relay and upload details retain their parent target.
  Version uses the same label/value columns as time and login.
- `EditableSection` opens the existing edit action when its free surface is
  clicked. Native buttons and links remain keyboard accessible. Text selection,
  modifier clicks and nested controls are excluded. Dialogs are outside the
  clickable surface to avoid reopening on close.
- Access is grouped into Network, Administration and Live View. Passwords remain
  separate protected actions within their corresponding group. Shared session
  duration explicitly applies to admin and viewer sessions.
- Add actions include plus icons. About includes project attribution, the
  authorized MIT license and the owner's existing support links.
- Support prepares a mail draft addressed to `mail@tbuck.de`. Diagnostics are
  editable and opt-in. It never sends mail or attaches files automatically.
  A POST creates a redacted bundle for download; the server accepts no source
  or output path and removes the temporary archive after serving it.

## Checks

The original reproduction clicked the actual rendered Display summary and
failed because the editor remained closed. It passes after the shared section
component was introduced.

`frontend/tests/fixtures/admin_navigation_browser.js` runs against the actual
Vue app with deterministic API fixtures. Open a Playwright CLI session, run the
Vite server on port 18174, create `output/playwright`, and pass the fixture to
`playwright-cli run-code`. It checks desktop Home redirects, section clicks,
selection exclusion, keyboard activation, cancel without writes, scoped save,
nested password controls, active-row backgrounds, footer columns, icons,
camera/identity/upload editor routes, mail opt-in, download retry and mobile
layouts at 390 and 320 px. Screenshots disable route animations.

The browser download is a fixture response. Go API tests separately verify the
real compressed archive contents, credential masking, temporary-file cleanup,
rejection of unauthenticated/viewer access and diagnostic-detail omission.
`go test -race ./...`, `go vet ./...`, all 25 frontend tests and the production
build pass. No camera upload or email was sent during verification.

Physical mobile keyboards, assistive technology and an external mail client
were not available. Bundle attachment remains a manual mail-client action.
