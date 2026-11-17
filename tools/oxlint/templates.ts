import { parse } from '@vue/compiler-dom'

/** @param {string} source @returns {string[]} */
export function checkTemplate(source: string): string[] {
  const errors: string[] = []
  const emoji = /\p{Extended_Pictographic}/u
  /** @param {any} node @param {boolean} dialog */
  function walk(node: any, dialog: boolean = false) {
    if (node.type === 2 && emoji.test(node.content)) errors.push('inline-icon')
    if (node.type === 1) {
      const attrs = Object.fromEntries(
        node.props
          .filter((p: any) => p.type === 6)
          .map((p: any) => [p.name, p.value?.content ?? '']),
      )
      dialog ||=
        node.tag === 'dialog' ||
        ['dialog', 'alertdialog'].includes(attrs.role) ||
        attrs['aria-modal'] === 'true'
      if (
        ['input', 'select', 'textarea', 'DropdownSelect', 'dropdown-select'].includes(node.tag) &&
        !dialog
      )
        errors.push('input-outside-dialog')
      const element = node.tag.toLowerCase()
      const dynamicTag = node.props.find((p: any) => p.arg?.content === 'is')?.exp?.content
      if (
        ['select', 'option', 'optgroup', 'selector'].includes(element) ||
        attrs.is?.toLowerCase() === 'select' ||
        /^['"]select['"]$/iu.test(dynamicTag ?? '')
      )
        errors.push('no-native-select: use DropdownSelect instead')
      if (node.tag === 'svg') errors.push('inline-icon')
      if (['alert', 'status', 'log', 'tooltip'].includes(attrs.role)) errors.push('inline-message')
      if (
        /\b(alert|notice|callout|tooltip|popover|inline-error|toast-container)\b/u.test(
          attrs.class ?? '',
        )
      )
        errors.push('inline-message')
      if (Object.keys(attrs).some((name) => /tooltip|popover|^data-toggle/u.test(name)))
        errors.push('inline-message')
      if (attrs.href?.startsWith('javascript:')) errors.push('unsafe-url')
      if (attrs.target === '_blank' && !attrs.rel?.split(' ').includes('noopener'))
        errors.push('unsafe-link')
      for (const prop of node.props.filter((p: any) => p.type === 7)) {
        if ((prop.exp?.content.length ?? 0) > 50) errors.push('long-expression')
        if (prop.name === 'html') errors.push('unsafe-html')
        if (/document\.|innerHTML|outerHTML|insertAdjacentHTML/u.test(prop.exp?.content ?? ''))
          errors.push('raw-dom')
      }
      if (
        ['button', 'a'].includes(node.tag) &&
        !node.children.length &&
        !attrs['aria-label'] &&
        !node.props.some((p: any) => p.arg?.content === 'aria-label')
      )
        errors.push('unnamed-control')
    }
    for (const child of node.children ?? []) walk(child, dialog)
  }
  walk(parse(source))
  return errors
}
