import { describe, expect, it } from 'vite-plus/test'
import { iconSprite } from '../../../tools/copy-icons.ts'

describe('icon sprite', () => {
  it('shares one mask URL and selects each icon by position', () => {
    const plugin = iconSprite()
    const load = plugin.load
    if (typeof load !== 'function') throw new Error('Missing sprite loader')
    const css = load.call({ addWatchFile() {} } as never, '\0virtual:remixicon.css')
    expect(typeof css).toBe('string')
    const source = String(css)
    expect(source.match(/url\(/gu)).toHaveLength(1)
    expect(source).toContain('url("/static/app/remixicon.svg")')
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
})
