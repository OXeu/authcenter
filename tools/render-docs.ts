import MarkdownIt from 'markdown-it'
import { bundledLanguages, bundledLanguagesAlias, createHighlighter } from 'shiki'
import { transformerStyleToClass } from '@shikijs/transformers'

/** Creates a reusable build-time renderer with external CSS for the site's CSP. */
export async function createDocsRenderer() {
  const highlighter = await createHighlighter({
    themes: ['github-light', 'github-dark'],
    langs: [],
  })
  const styles = transformerStyleToClass({ classPrefix: 'shiki-token-' })
  const languages = { ...bundledLanguages, ...bundledLanguagesAlias }
  const markdown = new MarkdownIt({
    html: false,
    highlight(code, language) {
      const lang = highlighter.getLoadedLanguages().includes(language) ? language : 'text'
      return highlighter.codeToHtml(code, {
        lang,
        themes: { light: 'github-light', dark: 'github-dark' },
        defaultColor: false,
        transformers: [styles],
      })
    },
  })
  markdown.renderer.rules.code_block = (tokens, index) =>
    markdown.options.highlight!(tokens[index].content, 'text', '')
  return {
    async render(source: string): Promise<string> {
      const tokens = markdown.parse(source, {})
      const requested = new Set(
        tokens
          .filter((token) => token.type === 'fence')
          .map((token) => token.info.trim().split(/\s+/u)[0]),
      )
      await highlighter.loadLanguage(
        ...[...requested]
          .filter((lang) => Object.hasOwn(languages, lang))
          .map((lang) => languages[lang as keyof typeof languages]),
      )
      return markdown.renderer.render(tokens, markdown.options, {})
    },
    css: () => styles.getCSS(),
    dispose: () => highlighter.dispose(),
  }
}
