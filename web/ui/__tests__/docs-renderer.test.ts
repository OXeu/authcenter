import { Blob } from 'node:buffer'
import { beforeAll, afterAll, describe, it, expect, vi } from 'vite-plus/test'
import { createDocsRenderer } from '../../../tools/render-docs.ts'

let renderer: Awaited<ReturnType<typeof createDocsRenderer>>
beforeAll(async () => {
  vi.stubGlobal('Blob', Blob)
  renderer = await createDocsRenderer()
})
afterAll(() => {
  renderer?.dispose()
  vi.unstubAllGlobals()
})

describe('build-time Shiki rendering', () => {
  it.each([
    ['go', 'func main() { println("hello") }\n'],
    ['json', '{"enabled":true}\n'],
    ['sh', 'curl -H "Authorization: ApiKey $KEY" https://example.com\n'],
  ])('highlights %s with external styles for both themes', async (lang, code) => {
    const html = await renderer.render('```' + lang + '\n' + code + '```')
    const parsed = new DOMParser().parseFromString(html, 'text/html')
    expect(parsed.querySelector('pre.shiki code')?.textContent).toBe(code)
    expect(html).toContain('shiki-token-')
    expect(parsed.querySelector('[style]')).toBeNull()
    expect(parsed.querySelector('script')).toBeNull()
    expect(renderer.css()).toContain('--shiki-light:')
    expect(renderer.css()).toContain('--shiki-dark:')
  })
  it.each(['unknown-language', ''])('preserves and escapes code with language %s', async (lang) => {
    const code = '<script>alert("x")</script> & <b>code</b>\n'
    const html = await renderer.render('```' + lang + '\n' + code + '```')
    const parsed = new DOMParser().parseFromString(html, 'text/html')
    expect(parsed.querySelector('pre.shiki code')?.textContent).toBe(code)
    expect(parsed.querySelector('script, b')).toBeNull()
  })
  it('renders indented code through Shiki too', async () => {
    const html = await renderer.render('    plain code\n')
    expect(html).toContain('class="shiki ')
    expect(
      new DOMParser().parseFromString(html, 'text/html').querySelector('code')?.textContent,
    ).toBe('plain code\n')
  })
  it('keeps raw HTML inert and refuses JavaScript links', async () => {
    const html = await renderer.render('<script>alert(1)</script>\n\n[unsafe](javascript:alert(1))')
    const parsed = new DOMParser().parseFromString(html, 'text/html')
    expect(parsed.querySelector('script, a')).toBeNull()
  })
  it('retains Markdown tables and navigation links', async () => {
    const html = await renderer.render(
      '| Name | Value |\n| --- | --- |\n| A | B |\n\n[SDK](/docs/sdk)',
    )
    const parsed = new DOMParser().parseFromString(html, 'text/html')
    expect(parsed.querySelectorAll('table td')).toHaveLength(2)
    expect(parsed.querySelector('a')?.getAttribute('href')).toBe('/docs/sdk')
  })
})
