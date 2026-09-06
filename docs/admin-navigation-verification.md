# Administration navigation and support verification

The September 6 revision incorporates the sidebar and action-placement feedback:

- Navigation and shared actions use pinned `@lucide/vue` components, with the
  package's complete license notice shipped locally. Navigation has 14 px main
  items, 13 px indented children, a full-row active background and a straight
  3 px left border with zero left corner radius. Only the current destination
  has `aria-current`; its actual parent is emphasized separately.
- System opens the general settings directly at `/system`. Maintenance opens an
  overview at `/system/wartung`. Old general, update and event routes/anchors
  redirect to their current destinations. Mobile Home retains direct access.
- Camera, relay and identity creation actions sit at the right of the page
  header. Dialog cancellation and primary actions align right at desktop and
  mobile widths; no new delete operations were added.
- Free-surface editing, selection exclusions, native links/buttons, keyboard
  behavior and protected drafts remain in use.
- Updates live under About. The application layout retains background recovery
  and the existing six-hour check independently of the About page.
- Support shows five redacted events. The text download contains up to 100;
  the separate diagnostic archive adds status, camera connections and settings.
  Download failures retain the draft and allow retry. Email opens a draft for
  `mail@tbuck.de`; version/log inclusion is opt-in. Files are attached manually.

## Checks

Before the navigation change, the browser reproduction failed because System
was not a link. The shared fixture now verifies keyboard navigation through
System and Maintenance as well as aliases, parents and current destinations.

`frontend/tests/fixtures/admin_navigation_browser.js` runs against the actual
Vue application with deterministic API fixtures. Start Vite on port 18174,
open a Playwright CLI session, create `output/playwright`, and pass the fixture
text to `playwright-cli run-code`. It checks actual Lucide SVGs, active-edge
geometry, type sizes, right-aligned headers and dialog actions, editing and
cancellation, old-route redirects, updates under About and polling away from
About, five-event preview, both downloads and retry, mail opt-in, and layouts
at 390 and 320 px. Screenshots disable route animations and were inspected.
The downloaded text was independently checked to contain all eight fixture
entries, although the preview displays only five.

The browser archive response is a fixture. Go tests independently verify real
archive contents, credential masking, temporary-file cleanup, rejection of
unauthenticated/viewer access and omission of raw event details. The new
100-event contract test failed with the old 20-event limit before the change.
All 27 frontend tests, the production build, relevant Go API tests with race
checking and Go API vet passed locally. Release CI runs the full checks.

No camera upload or email was sent. Physical mobile keyboards, assistive
technology and external mail clients were not available for verification.
