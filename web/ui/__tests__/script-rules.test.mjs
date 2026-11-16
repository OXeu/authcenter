import { it, expect, vi } from 'vite-plus/test'
import plugin from '../../../tools/oxlint/configcenter.js'

it('rejects document queries outside the bootstrap and HTML sinks everywhere', () => {
  const report = vi.fn()
  const rule = plugin.rules['no-page-dom-access']
  const callee = {
    type: 'MemberExpression',
    object: { type: 'Identifier', name: 'document' },
    property: { type: 'Identifier', name: 'querySelector' },
  }
  rule.create({ filename: '/web/ui/src/App.vue', report }).CallExpression({ callee })
  expect(report).toHaveBeenCalledWith(expect.objectContaining({ messageId: 'rawDom' }))
  report.mockClear()
  const bootstrap = rule.create({ filename: '/web/ui/src/main.js', report })
  bootstrap.CallExpression({ callee })
  expect(report).not.toHaveBeenCalled()
  bootstrap.MemberExpression({ property: { type: 'Identifier', name: 'innerHTML' } })
  expect(report).toHaveBeenCalledWith(expect.objectContaining({ messageId: 'rawDom' }))
})
