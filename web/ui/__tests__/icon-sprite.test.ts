import { readFileSync } from 'node:fs'
import type { IncomingMessage, ServerResponse } from 'node:http'
import type { Connect, Plugin, ViteDevServer } from 'vite-plus'
import { afterEach, describe, expect, it, vi } from 'vite-plus/test'
import { iconSprite } from '../../../tools/copy-icons.ts'

vi.mock('node:fs', async (importOriginal) => {
  const fs = await importOriginal<typeof import('node:fs')>()
  return { ...fs, readFileSync: vi.fn(fs.readFileSync) }
})

afterEach(() => vi.mocked(readFileSync).mockReset())

function buildSprite(plugin: Plugin = iconSprite()) {
  const load = plugin.load
  const generate = plugin.generateBundle
  if (typeof load !== 'function' || typeof generate !== 'function') {
    throw new Error('Missing sprite hooks')
  }
  const css = load.call({ addWatchFile() {} } as never, '\0virtual:remixicon.css')
  expect(typeof css).toBe('string')
  const emitFile = vi.fn<(asset: { fileName: string; source: string }) => string>()
  generate.call({ emitFile } as never, {} as never, {}, false)
  expect(emitFile).toHaveBeenCalledOnce()
  const asset = emitFile.mock.calls[0]![0]
  expect(asset.fileName).toMatch(/^remixicon-[a-f0-9]{16}\.svg$/u)
  expect(css).toContain(`url("/static/app/${asset.fileName}")`)
  return { css: String(css), ...asset }
}

describe('icon sprite', () => {
  it('shares one mask URL and selects each icon by position', () => {
    const { css: source } = buildSprite()
    expect(source.match(/url\(/gu)).toHaveLength(1)
    expect(source).not.toContain('.svg#')
    const positions = [...source.matchAll(/mask-position:0 ([\d.]+)%/gu)].map((match) =>
      Number(match[1]),
    )
    expect(positions.length).toBeGreaterThan(1)
    expect(new Set(positions).size).toBe(positions.length)
    expect(positions[0]).toBe(0)
    expect(positions.at(-1)).toBe(100)
    expect(source).toContain(`mask-size:100% ${positions.length * 100}%`)
  })

  it('keeps the URL stable when the sprite content is unchanged', () => {
    expect(buildSprite()).toEqual(buildSprite())
  })

  it('changes the URL when icons are added or their SVG content changes', () => {
    let icons = 'ri-add-line ri-close-line'
    let svg = '<svg><path d="M0 0h24v24H0z"/></svg>'
    vi.mocked(readFileSync).mockImplementation((path) =>
      String(path).endsWith('.svg') ? svg : icons,
    )
    const plugin = iconSprite()
    const initial = buildSprite(plugin)
    icons += ' ri-check-line'
    const expanded = buildSprite(plugin)
    expect(expanded.fileName).not.toBe(initial.fileName)
    svg = '<svg><path d="M0 0h12v12H0z"/></svg>'
    const updated = buildSprite(plugin)
    expect(updated.fileName).not.toBe(expanded.fileName)
    expect(buildSprite(plugin)).toEqual(updated)
  })

  it('serves the hashed URL in development and passes other assets through', () => {
    const plugin = iconSprite()
    const configure = plugin.configureServer
    if (typeof configure !== 'function') throw new Error('Missing dev server hook')
    let handler!: Connect.NextHandleFunction
    configure.call(
      {} as never,
      {
        middlewares: {
          use: (route: string, fn: Connect.NextHandleFunction) => {
            expect(route).toBe('/static/app/')
            handler = fn
          },
        },
      } as ViteDevServer,
    )
    const sprite = buildSprite(plugin)
    const setHeader = vi.fn()
    const end = vi.fn()
    const next = vi.fn()
    const response = { setHeader, end } as unknown as ServerResponse
    handler({ url: `/${sprite.fileName}?v=1` } as IncomingMessage, response, next)
    expect(setHeader).toHaveBeenCalledWith('Content-Type', 'image/svg+xml')
    expect(end).toHaveBeenCalledWith(sprite.source)
    expect(next).not.toHaveBeenCalled()
    handler({ url: '/main.js' } as IncomingMessage, response, next)
    handler({ url: '/remixicon.svg' } as IncomingMessage, response, next)
    expect(next).toHaveBeenCalledTimes(2)
    expect(end).toHaveBeenCalledOnce()
  })
})
