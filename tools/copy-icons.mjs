import { copyFileSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'

const out = 'internal/server/static/icons'
mkdirSync(out, { recursive: true })
const source = 'node_modules/remixicon/fonts/remixicon.css'
const css = readFileSync(source, 'utf8').replace(
  /@font-face \{[\s\S]*?\n\}/u,
  `@font-face {
  font-family: "remixicon";
  src: url("remixicon.woff2") format("woff2");
  font-display: swap;
}`,
)
writeFileSync(`${out}/remixicon.css`, css)
copyFileSync('node_modules/remixicon/fonts/remixicon.woff2', `${out}/remixicon.woff2`)
