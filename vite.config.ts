import { devPages } from './tools/dev-pages.ts'
import tailwind from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite-plus'

export default defineConfig({
  plugins: [vue(), tailwind(), devPages()],
  base: '/',
  cacheDir: '.tmp/vite',
  server: {
    proxy: {
      '^/(api|mcp|auth|oauth|language|login|logout|static)': 'http://127.0.0.1:8080',
    },
  },
  build: {
    outDir: '.tmp/ui',
    emptyOutDir: true,
    manifest: false,
    sourcemap: false,
    rollupOptions: {
      input: 'web/ui/src/main.ts',
      output: { entryFileNames: 'main.js', assetFileNames: '[name][extname]' },
    },
  },
  test: {
    environment: 'jsdom',
    include: ['web/ui/__tests__/**/*.test.ts', 'tools/oxlint/**/*.test.ts'],
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
    jsPlugins: ['vite-plus/oxlint-plugin', './tools/oxlint/configcenter.ts'],

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
