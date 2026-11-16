<script setup>
import { computed, ref } from 'vue'
import { api } from './core.js'
import { createPage } from './page.js'

/**
 * @typedef {import('./data.js').PageData} PageData
 */

const props = defineProps({
  data: { type: Object, required: true },
  msg: { type: Function, required: true },
  title: { type: String, required: true },
  subtitle: { type: String, required: true },
})

const data = /** @type {PageData} */ (props.data)
const msg = /** @type {(id: string, vars?: Record<string, string>) => string} */ (props.msg)
const open = ref(false)
const roleLabel = computed(() => (data.me === null ? '' : msg(`role.${data.me.role}`)))
/** @returns {Promise<void>} */
async function logout() {
  try {
    await api('/logout', { method: 'POST', csrf: data.csrf })
    location.assign('/login')
  } catch (err) {
    createPage(data).report(err)
  }
}
</script>

<template>
  <a class="sr-only" href="#main">{{ msg('nav.skip') }}</a>
  <header class="flex items-center gap-4 border-b px-6 py-3">
    <a class="text-sm font-semibold" href="/projects">{{ msg('app.name') }}</a>
    <nav class="flex flex-1 gap-1" :aria-label="msg('nav.main')">
      <a v-if="data.loggedIn" class="btn-ghost" href="/projects">{{ msg('nav.projects') }}</a>
      <a v-if="data.isAdmin" class="btn-ghost" href="/users">{{ msg('nav.users') }}</a>
    </nav>
    <div class="relative" @keydown.escape="open = false">
      <button
        class="btn-ghost gap-1"
        type="button"
        :aria-expanded="open ? 'true' : 'false'"
        @click="open = !open"
      >
        <i class="ri-global-line" aria-hidden="true"></i><span>{{ data.languageLabel }}</span
        ><i class="ri-arrow-down-s-line" aria-hidden="true"></i>
      </button>
      <ul
        v-show="open"
        class="absolute right-0 z-30 mt-1 w-40 rounded-md border bg-card p-1"
        :aria-label="msg('language')"
      >
        <li v-for="language in data.languages" :key="language.tag" @click="open = false">
          <a
            class="block rounded-sm px-2 py-1 text-sm hover:bg-muted"
            :href="`/language?lang=${language.tag}`"
            :aria-current="language.tag === data.lang ? 'true' : undefined"
          >
            <i v-if="language.tag === data.lang" class="ri-check-line mr-1" aria-hidden="true"></i
            >{{ language.label }}
          </a>
        </li>
      </ul>
    </div>
    <template v-if="data.loggedIn && data.me">
      <p class="text-sm text-muted-foreground">
        {{ data.me.name }} <span class="badge">{{ roleLabel }}</span>
      </p>
      <form @submit.prevent="logout">
        <button class="btn-outline" type="submit">
          <i class="ri-logout-box-r-line mr-1" aria-hidden="true"></i>{{ msg('nav.signOut') }}
        </button>
      </form>
    </template>
  </header>
  <main id="main" class="mx-auto max-w-6xl px-6 py-8">
    <h1 class="text-xl font-semibold">{{ title }}</h1>
    <p v-if="subtitle" class="text-sm text-muted-foreground">{{ subtitle }}</p>
    <slot />
  </main>
</template>
