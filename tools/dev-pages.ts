import { devPages as commonPages } from '@oxeu/center-common/tools/dev-pages.ts'

export const pageOptions = {
  backend: 'http://127.0.0.1:8081',
  home: '/applications',
  routes: /^\/(?:applications|account|keys|users|settings|login|docs(?:\/[^?]*)?)(?:\?|$)/u,
}

export function devPages() {
  return commonPages(pageOptions)
}
