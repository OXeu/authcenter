import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite-plus'

export default defineConfig({
  plugins: [vue()],
  server: {
    proxy: {
      '^/(api|mcp|auth|oauth|language|login|logout|static)': 'http://127.0.0.1:8080',
    },
  },
  build: {
    outDir: 'internal/server/static/app',
    emptyOutDir: true,
    manifest: true,
    sourcemap: false,
    rollupOptions: {
      input: 'web/ui/index.html',
    },
  },
  test: {
    environment: 'jsdom',
    include: ['web/ui/__tests__/**/*.test.mjs', 'tools/oxlint/**/*.test.mjs'],
    testTimeout: 15000,
  },
  lint: {
    plugins: ['typescript', 'unicorn', 'oxc', 'vue'],
    categories: {
      correctness: 'warn',
      suspicious: 'warn',
      perf: 'warn',
    },
    env: { browser: true, es2024: true },
    ignorePatterns: ['internal/server/static/**', 'sdk/**', 'node_modules/**'],
    jsPlugins: ['vite-plus/oxlint-plugin', './tools/oxlint/configcenter.js'],
    options: {
      typeAware: true,
      typeCheck: true,
    },
    rules: {
      'configcenter/no-page-dom-access': 'error',
      'configcenter/no-inline-icons': 'error',
      'vite-plus/prefer-vite-plus-imports': 'error',
      'no-var': 'error',
      'prefer-const': 'error',
      eqeqeq: 'error',
      'no-eval': 'error',
      'no-implied-eval': 'error',
      'no-new-func': 'off',
      'no-console': ['error', { allow: ['warn', 'error'] }],
      'max-lines': ['error', { max: 400, skipBlankLines: true, skipComments: true }],
    },
  },
  fmt: {
    ignorePatterns: ['internal/server/static/**', 'sdk/**'],
    semi: false,
    singleQuote: true,
  },
})
