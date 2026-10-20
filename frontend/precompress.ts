import { readdir, readFile, writeFile } from 'node:fs/promises'
import { join, resolve } from 'node:path'
import { constants, gzipSync } from 'node:zlib'
import type { Plugin, ResolvedConfig } from 'vite'

// Must match compressibleExt in camera-manager/internal/web/api/assets.go.
const compressible = /\.(html|js|mjs|css|svg|json|webmanifest|txt|xml|map)$/i
const minBytes = 1024

async function* walk(dir: string): AsyncGenerator<string> {
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name)
    if (entry.isDirectory()) yield* walk(path)
    else if (entry.isFile()) yield path
  }
}

/** Writes file.gz next to every compressible file in dir and returns the paths compressed. */
export async function precompressDir(dir: string): Promise<string[]> {
  const written: string[] = []
  for await (const path of walk(dir)) {
    if (!compressible.test(path)) continue
    const raw = await readFile(path)
    if (raw.length < minBytes) continue
    const gz = gzipSync(raw, { level: constants.Z_BEST_COMPRESSION })
    if (gz.length >= raw.length) continue
    // Written after its source; the server ignores a .gz older than the original.
    await writeFile(path + '.gz', gz)
    written.push(path)
  }
  return written
}

export function precompress(): Plugin {
  let outDir = ''
  return {
    name: 'camera-appliance:precompress',
    apply: 'build',
    configResolved(config: ResolvedConfig) {
      outDir = resolve(config.root, config.build.outDir)
    },
    async closeBundle() {
      await precompressDir(outDir)
    }
  }
}
