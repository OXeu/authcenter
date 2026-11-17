import { parse } from 'postcss'

export function checkDropdownStyles(source: string, requireBase = false): string[] {
  const errors: string[] = []
  let fixed = false
  parse(source).walkRules((rule) => {
    if (!/\.dropdown-(?:panel|content|options)(?![\w-])/u.test(rule.selector)) return
    if (rule.selector.includes(':has(')) errors.push('dropdown-no-layout-toggle')
    rule.walkDecls((declaration) => {
      if (declaration.prop === 'position') {
        if (declaration.value !== 'fixed') errors.push('dropdown-no-position-override')
        if (rule.selector === '.dropdown-panel' && declaration.value === 'fixed') fixed = true
      }
      if (declaration.prop === 'all') errors.push('dropdown-no-position-override')
    })
    rule.walkAtRules('apply', (apply) => {
      if (
        apply.params
          .split(/\s+/u)
          .some((name) => /(?:^|:|!)(?:static|relative|absolute|sticky)(?:!|$)/u.test(name))
      )
        errors.push('dropdown-no-position-override')
    })
  })
  if (requireBase && !fixed) errors.push('dropdown-requires-fixed-style')
  return [...new Set(errors)]
}
