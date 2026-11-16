<script setup>
import DropdownMenu from './DropdownMenu.vue'
import { computed } from 'vue'
import { api } from './core.ts'
import { createPage } from './page.ts'

/**
 * @typedef {import('./data.ts').PageData} PageData
 */

const props = defineProps({
  data: { type: Object, required: true },
  msg: { type: Function, required: true },
  title: { type: String, required: true },
  subtitle: { type: String, required: true },
  theme: { type: String, required: true },
})

const emit = defineEmits(['toggle-theme'])
const themeLabel = computed(() => msg(props.theme === 'dark' ? 'theme.toLight' : 'theme.toDark'))
const themeIcon = computed(() => (props.theme === 'dark' ? 'ri-moon-line' : 'ri-sun-line'))

const data = /** @type {PageData} */ (props.data)
const msg = /** @type {(id: string, vars?: Record<string, string>) => string} */ (props.msg)
const avatarInitial = computed(() => {
  const name = data.me?.name.trim() || data.me?.email || ''
  return Array.from(name)[0]?.toLocaleUpperCase() ?? ''
})
const identityLabel = computed(() => `${data.me?.name ?? ''} ${roleLabel.value}`)
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
  <header class="flex flex-wrap items-center gap-4 border-b px-6 py-3">
    <a class="text-sm font-semibold" href="/projects">{{ msg('app.name') }}</a>
    <nav class="flex flex-1 gap-1" :aria-label="msg('nav.main')">
      <a v-if="data.loggedIn" class="btn-ghost" href="/projects">{{ msg('nav.projects') }}</a>
      <a v-if="data.isAdmin" class="btn-ghost" href="/users">{{ msg('nav.users') }}</a>
    </nav>
    <button
      class="btn-ghost btn-icon"
      type="button"
      :aria-label="themeLabel"
      @click="emit('toggle-theme')"
    >
      <i :class="themeIcon" aria-hidden="true"></i>
    </button>
    <DropdownMenu :label="data.languageLabel" compact>
      <template #trigger><i class="ri-global-line" aria-hidden="true"></i></template>
      <a
        v-for="language in data.languages"
        :key="language.tag"
        class="block rounded-sm px-2 py-1 text-sm hover:bg-muted"
        :href="`/language?lang=${language.tag}`"
        :aria-current="language.tag === data.lang ? 'true' : undefined"
        >{{ language.label }}</a
      >
    </DropdownMenu>
    <template v-if="data.loggedIn && data.me">
      <div class="user-avatar" role="img" :aria-label="identityLabel">
        <span v-if="avatarInitial" aria-hidden="true">{{ avatarInitial }}</span>
        <i v-else class="ri-user-line" aria-hidden="true"></i>
        <span class="badge avatar-role" aria-hidden="true">{{ roleLabel }}</span>
      </div>
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
