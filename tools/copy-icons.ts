import { readdirSync, readFileSync } from 'node:fs'
import { basename, join } from 'node:path'
import type { Plugin } from 'vite-plus'

function files(root: string): string[] {
  return readdirSync(root, { withFileTypes: true }).flatMap((entry) => {
    const path = join(root, entry.name)
    return entry.isDirectory() ? files(path) : [path]
  })
}

export function iconSprite(): Plugin {
  const moduleID = 'virtual:remixicon.css'
  const resolvedID = `\0${moduleID}`
  let sprite = ''
  return {
    name: 'remixicon-sprite',
    resolveId(id) {
      if (id === moduleID) return resolvedID
    },
    load(id) {
      if (id !== resolvedID) return
      const sources = files('web/ui/src').filter((path) => /\.(vue|ts)$/u.test(path))
      const names = new Set<string>()
      for (const path of sources) {
        this.addWatchFile(path)
        for (const match of readFileSync(path, 'utf8').matchAll(
          /\bri-([a-z0-9]+(?:-[a-z0-9]+)*-(?:line|fill))\b/gu,
        )) {
          names.add(match[1]!)
        }
      }
      const icons = new Map(
        files('node_modules/remixicon/icons').map((path) => [basename(path, '.svg'), path]),
      )
      const views: string[] = []
      const rules: string[] = []
      for (const [index, name] of [...names].sort().entries()) {
        const path = icons.get(name)
        if (!path) throw new Error(`Unknown Remix Icon: ${name}`)
        const source = readFileSync(path, 'utf8')
        const body = source
          .replace(/^[\s\S]*?<svg[^>]*>/u, '')
          .replace(/<\/svg>\s*$/u, '')
          .trim()
        views.push(
          `<view id="${name}" viewBox="0 ${index * 24} 24 24"/><g transform="translate(0 ${index * 24})">${body}</g>`,
        )
        rules.push(`.ri-${name}{mask-image:url("/static/app/remixicon.svg#${name}")}`)
      }
      sprite = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="${names.size * 24}" viewBox="0 0 24 ${names.size * 24}">${views.join('')}</svg>`
      return `[class^="ri-"],[class*=" ri-"]{display:inline-block;width:1em;height:1em;background-color:currentColor;mask-repeat:no-repeat;mask-size:100% 100%;vertical-align:-.125em}${rules.join('')}`
    },
    generateBundle() {
      this.emitFile({ type: 'asset', fileName: 'remixicon.svg', source: sprite })
    },
    configureServer(server) {
      server.middlewares.use('/static/app/remixicon.svg', (_request, response) => {
        response.setHeader('Content-Type', 'image/svg+xml')
        response.end(sprite)
      })
    },
  }
}
