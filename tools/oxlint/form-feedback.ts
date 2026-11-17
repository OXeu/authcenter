import { parse } from '@vue/compiler-dom'
import ts from 'typescript'

const feedbackWords = new Set([
  'error',
  'errors',
  'success',
  'failure',
  'feedback',
  'notice',
  'notification',
  'flash',
  'message',
  'messages',
  'submitted',
  'result',
])

function feedbackName(name: string, validation: boolean): boolean {
  const words = name
    .replace(/([a-z])([A-Z])/gu, '$1 $2')
    .toLowerCase()
    .split(/\W|_/u)
  if (validation && name === 'validationError') return false
  if (['failed', 'succeeded', 'saved'].includes(words.at(-1) ?? ''))
    return (
      words.length === 1 ||
      words.some((word) => /^(is|has|form|submit|submission|request|save)$/u.test(word))
    )
  if (['status', 'response'].includes(words.at(-1) ?? ''))
    return words.some((word) => /^(form|submit|submission|request|mutation)$/u.test(word))
  return words.some((word) => feedbackWords.has(word))
}

export function checkFormFeedback(template: string, script = ''): string[] {
  const variables = new Map<string, ts.Expression>()
  const file = ts.createSourceFile('component.ts', script, ts.ScriptTarget.Latest, true)
  function collect(node: ts.Node): void {
    if (ts.isVariableDeclaration(node) && ts.isIdentifier(node.name) && node.initializer)
      variables.set(node.name.text, node.initializer)
    ts.forEachChild(node, collect)
  }
  collect(file)
  function feedback(node: ts.Node, validation: boolean, seen = new Set<string>()): boolean {
    if (ts.isIdentifier(node)) {
      if (feedbackName(node.text, validation)) return true
      const value = variables.get(node.text)
      return Boolean(
        value && !seen.has(node.text) && feedback(value, validation, new Set([...seen, node.text])),
      )
    }
    if (ts.isPropertyAccessExpression(node) || ts.isElementAccessExpression(node)) {
      const name = ts.isPropertyAccessExpression(node) ? node.name : node.argumentExpression
      if (ts.isIdentifier(name) || ts.isStringLiteralLike(name)) {
        if (name.text === 'value') return feedback(node.expression, validation, seen)
        if (validation && node.expression.getText() === 'field' && name.text === 'error')
          return false
        if (['ok', 'status', 'response'].includes(name.text))
          return /^(form|submit|submission|request|response|mutation)$/u.test(
            node.expression.getText(),
          )
        return feedbackName(name.text, validation)
      }
    }
    if (ts.isStringLiteralLike(node))
      return /^(toast\.|error\.|authError\.|login\.codeSent$)/u.test(node.text)
    if (ts.isPropertyAssignment(node)) return feedback(node.initializer, validation, seen)
    return Boolean(ts.forEachChild(node, (child) => feedback(child, validation, seen) || undefined))
  }
  function expression(source: string, validation: boolean): boolean {
    return feedback(
      ts.createSourceFile('expression.ts', `(${source})`, ts.ScriptTarget.Latest, true),
      validation,
    )
  }
  function walk(node: any, dialog = false, field = false, validation = false): boolean {
    if (node.type === 5 && expression(node.content.content, validation)) return true
    if (node.type === 1) {
      const attrs = Object.fromEntries(
        node.props
          .filter((prop: any) => prop.type === 6)
          .map((prop: any) => [prop.name, prop.value?.content ?? '']),
      )
      dialog ||= node.tag === 'dialog'
      field ||= (attrs.class ?? '').split(/\s+/u).includes('field')
      validation =
        dialog && field && ['p', 'span', 'small', 'div', 'label', 'template'].includes(node.tag)
      if (
        /\b(?:(?:form|submit|submission|request|response)-(?:result|status|feedback)|(?:success|error|failure)-message)\b/u.test(
          attrs.class ?? '',
        )
      )
        return true
      for (const prop of node.props) {
        if (prop.type !== 7 || !prop.exp) continue
        if (
          ['if', 'else-if', 'show', 'text', 'html', 'for'].includes(prop.name) ||
          (prop.name === 'bind' && !['key', 'ref', 'disabled'].includes(prop.arg?.content))
        ) {
          if (expression(prop.exp.content, validation)) return true
        }
      }
    }
    return (node.children ?? []).some((child: any) => walk(child, dialog, field, validation))
  }
  return walk(parse(template)) ? ['form-feedback-requires-toast'] : []
}
