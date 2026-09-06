import test from 'node:test'
import assert from 'node:assert/strict'
import { isNavItemActive, isNavItemAncestor, legacyMaintenanceDestination } from './navigation.ts'

test('navigation distinguishes current destinations and their actual parents', () => {
  for (const [path, current, parent] of [
    ['/system', '/system', ''],
    ['/system/zugriff', '/system/zugriff', '/system'],
    ['/system/identitaeten/shared', '/system/identitaeten', '/system'],
    ['/system/relays/nas', '/system/relays', '/system'],
    ['/system/wartung', '/system/wartung', ''],
    ['/system/wartung/watchdog', '/system/wartung/watchdog', '/system/wartung'],
    ['/system/wartung/support', '/system/wartung/support', '/system/wartung'],
    ['/system/ueber', '/system/ueber', ''],
    ['/kamera/hof/bearbeiten', '/einrichtung', ''],
    ['/kameras/bild-upload/bearbeiten', '/kameras/bild-upload', '/einrichtung']
  ]) {
    assert.ok(isNavItemActive(current!, path!), path)
    for (const root of ['/system', '/system/wartung', '/einrichtung']) {
      assert.equal(isNavItemActive(root, path!), root === current, path + ': current ' + root)
      assert.equal(isNavItemAncestor(root, path!), root === parent, path + ': parent ' + root)
    }
  }
  assert.equal(isNavItemActive('/system/relays', '/system/relays-other'), false)
})

test('old maintenance anchors retain their destinations while plain maintenance opens the overview', () => {
  assert.equal(legacyMaintenanceDestination(''), undefined)
  assert.equal(legacyMaintenanceDestination('#backup'), '/system/wartung/sicherung')
  assert.equal(legacyMaintenanceDestination('#updates'), '/system/wartung/updates')
  assert.equal(legacyMaintenanceDestination('#events'), '/system/wartung/ereignisse')
})
