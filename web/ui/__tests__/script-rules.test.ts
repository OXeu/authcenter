import { it, expect, vi } from 'vite-plus/test'
import plugin from '../../../tools/oxlint/configcenter.ts'

it('rejects document queries outside the bootstrap and HTML sinks everywhere', () => {
  const report = vi.fn()
  const rule = plugin.rules['no-page-dom-access'] as unknown as {
    create(context: { filename: string; report: typeof report }): {
      CallExpression(node: unknown): void
      MemberExpression(node: unknown): void
    }
  }
  const callee = {
    type: 'MemberExpression',
    object: { type: 'Identifier', name: 'document' },
    property: { type: 'Identifier', name: 'querySelector' },
  }
  rule.create({ filename: '/web/ui/src/App.vue', report }).CallExpression({ callee })
  expect(report).toHaveBeenCalledWith(expect.objectContaining({ messageId: 'rawDom' }))
  report.mockClear()
  const bootstrap = rule.create({ filename: '/web/ui/src/main.ts', report })
  bootstrap.CallExpression({ callee })
  expect(report).not.toHaveBeenCalled()
  bootstrap.MemberExpression({ property: { type: 'Identifier', name: 'innerHTML' } })
  expect(report).toHaveBeenCalledWith(expect.objectContaining({ messageId: 'rawDom' }))
})
