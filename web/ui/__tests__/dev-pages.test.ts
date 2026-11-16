import { it, expect, vi, afterEach } from 'vite-plus/test'
import { devPages } from '../../../tools/dev-pages.ts'
afterEach(() => vi.unstubAllGlobals())
it('proxies authenticated HTML and replaces production assets for hot reload', async () => {
  let handler!: import('vite-plus').Connect.NextHandleFunction
  devPages().configureServer({
    middlewares: {
      use: (fn: import('vite-plus').Connect.NextHandleFunction) => {
        handler = fn
      },
    },
  } as import('vite-plus').ViteDevServer)
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(
      '<head><link rel="stylesheet" href="/static/app/main.css"><script src="/static/app/main.js"></script></head>',
      {
        headers: { 'content-security-policy': "script-src 'self'", 'content-type': 'text/html' },
      },
    ),
  )
  vi.stubGlobal('fetch', fetchMock)
  const headers = new Map()
  const body = await new Promise((resolve, reject) =>
    handler(
      {
        method: 'GET',
        url: '/projects',
        headers: { cookie: 'cc_session=test', 'accept-language': 'zh-CN' },
      } as import('node:http').IncomingMessage,
      {
        setHeader: (key: string, value: string) => headers.set(key, value),
        end: resolve,
      } as unknown as import('node:http').ServerResponse,
      reject,
    ),
  )
  expect(fetchMock).toHaveBeenCalledWith(
    'http://127.0.0.1:8080/projects',
    expect.objectContaining({ headers: { cookie: 'cc_session=test', 'accept-language': 'zh-CN' } }),
  )
  expect(body).toContain('/web/ui/src/main.ts')
  expect(body).toContain('/@vite/client')
  expect(body).not.toContain('/static/app/main.css')
  expect(headers.has('content-security-policy')).toBe(false)
})
