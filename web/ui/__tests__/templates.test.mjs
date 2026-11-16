import { describe, it, expect } from 'vite-plus/test'
import { readFileSync, readdirSync } from 'node:fs'
import { parse } from '@vue/compiler-sfc'
import { checkTemplate } from '../../../tools/oxlint/templates.js'
import { fallbackMessages } from '../src/i18n.js'
const locales = ['en', 'zh-CN'].map((lang) =>
  JSON.parse(readFileSync(`internal/i18n/locales/${lang}.json`, 'utf8')),
)
describe('Vue template rules', () => {
  it.each([
    ['<input />', 'input-outside-dialog'],
    ['<svg />', 'inline-icon'],
    ['<p role="alert">error</p>', 'inline-message'],
    ['<div v-html="value" />', 'unsafe-html'],
    ['<a href="javascript:alert(1)">link</a>', 'unsafe-url'],
    ['<button />', 'unnamed-control'],
    ['<button @click="document.querySelector(\'x\')">x</button>', 'raw-dom'],
  ])('rejects %s', (source, rule) => expect(checkTemplate(source)).toContain(rule))
  it('accepts inputs in native dialogs', () =>
    expect(checkTemplate('<dialog><label>Name<input /></label></dialog>')).toEqual([]))
  for (const file of readdirSync('web/ui/src').filter((name) => name.endsWith('.vue'))) {
    it(`validates ${file}`, () => {
      const source = readFileSync(`web/ui/src/${file}`, 'utf8')
      const { descriptor } = parse(source)
      expect(checkTemplate(descriptor.template.content)).toEqual([])
      expect(source.split('\n').filter((line) => line.trim()).length).toBeLessThanOrEqual(400)
      for (const match of source.matchAll(/msg\('([^']+)'/gu)) {
        for (const [index, locale] of locales.entries())
          expect(
            locale[match[1]] ?? fallbackMessages[index ? 'zh-CN' : 'en'][match[1]],
            match[1],
          ).toBeTruthy()
      }
    })
  }
})
