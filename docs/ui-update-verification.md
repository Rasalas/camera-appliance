# UI update verification

The 2026-09-05 test used the published Linux/amd64 archives for v0.3.0 and
v0.4.0. The running v0.4.0 NAS was not restarted or changed during this test.

The actual browser button queried GitHub, downloaded the versioned archive,
submitted the durable update job, and observed installation and restart. The
v0.4.0 download matched SHA-256
`48e42aa686634d134aca8a90212c7e72cc6770ebfcfbdf231f771b94b60b6480`.

Two failures were reproduced with the released frontend:

- Successful installation left the old JavaScript bundle running in the open
  browser, so newly installed UI features required a manual reload.
- Losing the install response left the UI in a failed state even when the
  independent job completed successfully.

The corrected client follows durable job status across interrupted requests,
reloads after a verified version change, and serializes requests. Regression
tests also cover rollback, rejected submissions, stale responses, duplicate
actions and teardown. New pages do not repeatedly reload completed jobs.

The Docker/browser fixture checked the corrected client with these outcomes:

| Case | Outcome |
| --- | --- |
| Normal installation | v0.4.0 serves; browser loads its new bundle |
| Download connection failure, then retry | Error shown; retry installs the exact published archive |
| Copy failure after partial installation | Failed job; snapshot restored; v0.3.0 serves again |
| Service restart failure | Failed job; rollback and old-version healthcheck succeed |
| Lost install acknowledgement | Exactly one submission; polling recovers and new UI loads |
| Leave the dedicated update page during submission | Exactly one submission; shared monitoring survives navigation, then loads v0.4.0 |

Settings, slot configuration and synthetic credential files were compared
before/after. Only watchdog runtime timestamps were excluded. No cameras,
real credentials, upload destinations or enabled upload schedules were used.

## Repeat the isolated check

Requires Docker, `gh`, and Playwright CLI. The fixture uses a disposable
container filesystem and binds its browser and control ports to host loopback.
It deliberately refuses to run outside that container. Do not mount an existing
installation, Docker socket, NAS data or customer configuration into it.

```sh
mkdir -p /tmp/camera-ui-update-lab
gh release download v0.3.0 --pattern camera-appliance-0.3.0-c232ad4.tar.gz --dir /tmp/camera-ui-update-lab
npm --prefix frontend run build
docker run -d --name camera-ui-update-lab --platform linux/amd64 \
  -e CAMERA_APPLIANCE_UI_UPDATE_LAB=1 \
  -p 127.0.0.1:18174:8080 -p 127.0.0.1:18175:8081 \
  -v /tmp/camera-ui-update-lab:/fixture:ro \
  -v "$PWD/scripts/tests/fixtures/ui_update_lab.py:/lab.py:ro" \
  python:3.12-slim python /lab.py
```

Wait for `http://127.0.0.1:18174/api/health` to report v0.3.0. For each case,
reset the fixture, overlay the locally built frontend, and open a fresh browser
session or disable its cache. Resetting the fixture restores old file mtimes.

```sh
curl -fsS -X POST http://127.0.0.1:18175/reset
docker cp frontend/dist/. camera-ui-update-lab:/appliance/frontend/dist/
playwright-cli --session update-check open 'http://127.0.0.1:18174/system/allgemein?case=success'
playwright-cli --session update-check run-code "$(cat scripts/tests/fixtures/ui_update_browser.js)"
```

Replace `case=success` with `download`, `copy`, `restart`, `lost-response` or
`navigation`. The navigation case opens **Version und Updates**, submits there,
then switches to **Allgemein** while the request is in flight. That destination
also exists in the published v0.4.0 frontend loaded after installation.
The script drives the real buttons, verifies the archive digest, checks the
terminal job and running version, and compares configuration fingerprints.
Omit the frontend overlay to reproduce the original released-client failures.
The fixture intentionally expects GitHub's newest release to be v0.4.0; update
the pinned release pair and expected digest when testing later releases.

```sh
playwright-cli --session update-check close
docker stop camera-ui-update-lab
docker rm camera-ui-update-lab
```

## Limits

The Linux manager, release catalog, HTTPS download, archive validation,
independent update-worker, file installation and rollback are real. A small
process supervisor replaces systemd, and an HTTP fixture replaces go2rtc.
This proves the browser/API/worker sequence and actual process replacement,
not systemd cgroup behavior, Docker Compose recreation or camera streaming.
Existing system and updater tests cover their adapter arguments and failure
paths. Power loss, disk exhaustion and unsupported CPU architectures were not
simulated. The local UI fixes still require a separately authorized release
before an installed v0.4.0 appliance can benefit from them.

## Settings and navigation verification

The same isolated v0.4.0 backend was also used to check the new navigation and
form boundaries with the locally built frontend:

- Ten sidebar destinations render their own titles; all five maintenance
  sections have distinct URLs and content. Five legacy links, including a
  maintenance link with query and fragment, redirect to the intended page.
- Editing connections and display together, then saving connections, sends
  only `capture_ssh_host`. The display draft remains unsaved and its Cancel
  button restores the saved value. Saving display persists after a reload.
- An injected settings HTTP 500 leaves the server value unchanged and shows
  the form error and unsaved status. Retrying succeeds and persists.
- Session duration and watchdog interval send string values and persist.
  Browser testing reproduced the previous numeric JSON serialization failure;
  the settings patch now normalizes number inputs before comparison and write.
- The upload-server form cancels its own draft and persists a directory change
  across reload while retaining the synthetic password. No upload was started.
- At 1280 px and 390 px, the sidebar/menu and forms fit without horizontal
  overflow. Mobile navigation closes after a link or Escape; screenshots were
  checked after loading and animations completed. Decorative title eyebrows,
  the old top tabs and the two conflicting application bind-address labels
  are absent. The go2rtc URL remains available in its configuration field.

All 23 frontend tests and the production build passed. Earlier full Go race
tests and vet passed; this navigation/form change does not modify the backend.
