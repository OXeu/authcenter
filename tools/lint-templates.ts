import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { parse } from '@vue/compiler-sfc'
import { checkTemplate } from './oxlint/templates.ts'
import { checkDangerTemplate } from './oxlint/danger.ts'

function check(directory: string): void {
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) check(path)
    else if (path.endsWith('.vue')) {
      const { descriptor } = parse(readFileSync(path, 'utf8'))
      const source = descriptor.template?.content ?? ''
      const errors = [...checkTemplate(source), ...checkDangerTemplate(source, path)]
      if (errors.length) throw new Error(`${path}: ${errors.join(', ')}`)
    }
  }
}
check('web/ui/src')
