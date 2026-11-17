import type { RouteRecordRaw } from 'vue-router'
import type { Component } from 'vue'

export function pageRoutes(component: Component): RouteRecordRaw[] {
  return [
    ...['/applications', '/account', '/users', '/settings', '/keys'].map((path) => ({
      path,
      component,
    })),
    { path: '/:pathMatch(.*)*', component, meta: { document: true } },
  ]
}
