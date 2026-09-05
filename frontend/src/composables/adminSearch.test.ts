import test from 'node:test'
import assert from 'node:assert/strict'
import { searchResources, type SearchEntry } from './adminSearch.ts'
const entries:SearchEntry[]=[
  {id:'c',kind:'Kameras',title:'Hofkamera',detail:'192.0.2.20 Tapo',href:'/kamera/c'},
  {id:'i',kind:'Identitäten',title:'Hof und Eingang',detail:'hof-user',href:'/system/identitaeten/i'},
  {id:'r',kind:'Relays',title:'Werkstatt',detail:'192.0.2.30',href:'/system/relays/r'}
]
test('search groups resource matches and restricts the selected scope',()=>{
 assert.deepEqual(searchResources(entries,'HOF').map(entry=>entry.id),['i','c'])
 assert.deepEqual(searchResources(entries,'hof','Kameras').map(entry=>entry.id),['c'])
 assert.deepEqual(searchResources(entries,'192.0.2.20 tapo').map(entry=>entry.id),['c'])
 assert.deepEqual(searchResources(entries,'missing'),[])
 assert.equal(entries[0]?.id,'c')
})
