import test from 'node:test'
import assert from 'node:assert/strict'
import { supportMailURL } from './supportDraft.ts'

test('support drafts encode user input and include only selected diagnostics', () => {
  const description = 'Bild fehlt & flackert?\nKamera #2 = Hof'
  for (const include of [false, true]) {
    const url = new URL(supportMailURL('mail@tbuck.de', description, 'Version 0.5.0\nTest event', include))
    assert.equal(url.pathname, 'mail@tbuck.de')
    assert.equal(url.searchParams.size, 2)
    assert.ok(url.searchParams.get('body')?.includes(description))
    assert.equal(url.searchParams.get('body')?.includes('Test event'), include)
  }
})
