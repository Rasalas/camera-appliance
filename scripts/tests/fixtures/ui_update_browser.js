async (page) => {
  // Open /system/allgemein?case=success|download|copy|restart|lost-response first.
  // No update API is mocked: every update POST comes from the actual button.
  const scenario = page.url().split('case=')[1]?.split('&')[0] || 'success';
  if (!['success', 'download', 'copy', 'restart', 'lost-response'].includes(scenario)) throw Error('Unknown scenario');
  const origin = page.url().split('/').slice(0, 3).join('/');
  const control = 'http://127.0.0.1:18175';
  const inject = async (name) => {
    const response = await page.request.post(`${control}/${name}`);
    if (!response.ok()) throw Error(`Fixture control failed: ${name}`);
  };
  const fingerprint = async () => await (await page.request.get(`${control}/fingerprint`)).json();
  // Fixture resets restore older file mtimes. Disable browser cache so each
  // scenario actually loads the selected baseline or patched frontend.
  await page.route('**/*', route => route.continue());
  await page.reload();
  const oldBundle = await page.locator('script[src]').first().getAttribute('src');
  const before = await fingerprint();
  await page.getByRole('button', { name: 'Update herunterladen', exact: true }).waitFor();
  const available = await (await page.request.get(origin + '/api/system/update/status')).json();
  if (available.current_version !== '0.3.0' || available.latest?.tag !== 'v0.4.0') throw Error('This fixture expects the real v0.3.0 → v0.4.0 release pair');
  if (scenario === 'download') await inject('fail-download');
  await page.getByRole('button', { name: 'Update herunterladen', exact: true }).click();
  if (scenario === 'download') {
    await page.getByRole('button', { name: 'Update fehlgeschlagen – erneut suchen', exact: true }).waitFor();
    if (JSON.stringify(await fingerprint()) !== JSON.stringify(before)) throw Error('Download failure changed customer configuration');
    await inject('allow-download');
    await page.getByRole('button', { name: 'Update fehlgeschlagen – erneut suchen', exact: true }).click();
    await page.getByRole('button', { name: 'Update herunterladen', exact: true }).click();
  }
  await page.getByRole('button', { name: 'Update jetzt installieren', exact: true }).waitFor({ timeout: 60000 });
  const ready = await (await page.request.get(origin + '/api/system/update/status')).json();
  if (ready.digest !== '48e42aa686634d134aca8a90212c7e72cc6770ebfcfbdf231f771b94b60b6480') throw Error('Downloaded archive differs from the published release');
  if (scenario === 'copy' || scenario === 'restart') await inject(`fail-${scenario}`);
  let submissions = 0;
  const track = request => { if (request.method() === 'POST' && request.url().endsWith('/api/system/update/install')) submissions++; };
  page.on('request', track);
  if (scenario === 'lost-response') await page.route('**/api/system/update/install', async route => {
    await route.fetch(); // Let the real server accept the independent job.
    await route.abort('connectionreset'); // Lose only its acknowledgement.
  });
  await page.getByRole('button', { name: 'Update jetzt installieren', exact: true }).click();
  let status;
  for (let i = 0; i < 120; i++) {
    try { status = await (await page.request.get(origin + '/api/system/update/status')).json(); } catch { /* manager restart */ }
    if (['complete', 'failed'].includes(status?.job?.phase)) break;
    await page.waitForTimeout(500);
  }
  await page.unroute('**/api/system/update/install');
  page.off('request', track);
  if (submissions !== 1) throw Error(`Expected exactly one install submission, got ${submissions}`);
  const failed = scenario === 'copy' || scenario === 'restart';
  if (status?.job?.phase !== (failed ? 'failed' : 'complete')) throw Error(`Unexpected final job: ${JSON.stringify(status)}`);
  if (failed) {
    if (!status.job.result.rollback_applied) throw Error('Previous installation was not restored');
    await page.getByRole('button', { name: 'Update fehlgeschlagen – erneut suchen', exact: true }).waitFor({ timeout: 15000 });
  } else {
    await page.waitForFunction(old => document.querySelector('script[src]')?.getAttribute('src') !== old, oldBundle, { timeout: 15000 });
    await page.getByRole('button', { name: 'Nach Updates suchen', exact: true }).waitFor({ timeout: 15000 });
  }
  const health = await (await page.request.get(origin + '/api/health')).json();
  if (health.version !== (failed ? '0.3.0' : '0.4.0')) throw Error(`Wrong running release: ${JSON.stringify(health)}`);
  if (JSON.stringify(await fingerprint()) !== JSON.stringify(before)) throw Error('Update or rollback changed customer configuration');
  return { scenario, health, job: status.job.phase, rollback: status.job.result.rollback_applied, submissions, configurationPreserved: true };
}
