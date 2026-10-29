// A loopback-only upstream proves that Vite carries both player modules and
// WebSocket upgrades. UI tests intercept their own API requests separately.
import { createServer } from 'node:http'
import { WebSocketServer } from 'ws'

const server = createServer((req, res) => {
  if (req.url === '/api/health') {
    res.writeHead(200, { 'Content-Type': 'application/json' })
    res.end(JSON.stringify({ status: 'ok', version: 'browser-test', commit: 'fixture' }))
  } else if (req.url === '/go2rtc/video-stream.js') {
    res.writeHead(200, { 'Content-Type': 'text/javascript' })
    res.end('export const proxyMarker = "player-module-from-upstream";')
  } else {
    res.writeHead(404)
    res.end()
  }
})
const sockets = new WebSocketServer({ noServer: true })
server.on('upgrade', (req, socket, head) => {
  if (req.url !== '/go2rtc/api/ws?src=cam1' || req.headers.origin !== 'http://127.0.0.1:18174') {
    socket.destroy()
    return
  }
  sockets.handleUpgrade(req, socket, head, ws => ws.send('player-socket-from-upstream'))
})
server.listen(18175, '127.0.0.1')
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => {
  for (const socket of sockets.clients) socket.terminate()
  sockets.close()
  server.close(() => process.exit(0))
})
