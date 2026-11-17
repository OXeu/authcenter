<script setup lang="ts">
import Button from '@oxeu/center-common/ui/Button.vue'
import { computed, defineAsyncComponent, onMounted, watch } from 'vue'
import LoginPanel from './LoginPanel.vue'
import ConsentPage from './ConsentPage.vue'
import AccountPage from './AccountPage.vue'
import ToastViewport from './ToastViewport.vue'
import PageHeader from './PageHeader.vue'
import { createPage } from './page.ts'
import { useTheme } from './theme.ts'
import { mountToasts, notify, setToastTheme } from './toast.ts'

const WorkspacePage = defineAsyncComponent(() => import('./WorkspacePage.vue'))
const KeysPage = defineAsyncComponent(() => import('./KeysPage.vue'))
const SettingsPage = defineAsyncComponent(() => import('./SettingsPage.vue'))

const props = defineProps({ data: { type: Object, required: true } })
const data = props.data as import('./data.ts').PageData
const { theme, toggleTheme } = useTheme()
watch(theme, (value) => setToastTheme(value))
const isDocs = computed(() => data.page === 'docs')
const isWorkspace = computed(() => ['applications', 'users'].includes(data.page))
const docsLinks = [
  { page: 'index', href: '/docs/', label: 'docs.overview' },
  { page: 'api', href: '/docs/api', label: 'docs.api' },
  { page: 'mcp', href: '/docs/mcp', label: 'docs.mcp' },
  { page: 'configuration', href: '/docs/configuration', label: 'docs.configuration' },
  { page: 'security', href: '/docs/security', label: 'docs.security' },
]
const rawDocument = computed(() => `/docs/${data.docPage ?? 'index'}.md`)
const page = createPage(data)
const { msg } = page
const title = computed(
  () => data.project?.name ?? msg(data.page === 'login' ? 'login.title' : data.title),
)
onMounted(() => {
  mountToasts(data.lang, data.nonce, theme.value)
  if (data.flash) notify(data.flash.level, data.flash.message)
  if (data.page === 'login' && !data.oauthEnabled && !data.localLogin)
    notify('info', msg('login.notConfigured'))
})
</script>
<template>
  <div
    :data-theme="theme"
    class="bg-background text-foreground"
    :class="{ 'min-h-screen': !isDocs }"
  >
    <ToastViewport />
    <PageHeader
      :data="data"
      :msg="msg"
      :title="title"
      :theme="theme"
      :header-only="isDocs"
      subtitle=""
      @toggle-theme="toggleTheme"
    >
      <LoginPanel v-if="data.page === 'login'" :data="data" :page="page" />
      <ConsentPage v-if="data.page === 'consent'" :data="data" :page="page" />
      <AccountPage v-if="data.page === 'account'" :data="data" :page="page" />
      <KeysPage v-if="data.page === 'keys'" :page="page" />
      <SettingsPage v-if="data.page === 'settings'" :page="page" />
      <WorkspacePage v-if="isWorkspace" :data="data" :page="page" :title="title" />
    </PageHeader>
    <nav
      v-if="isDocs"
      class="mx-auto flex max-w-6xl flex-wrap gap-2 px-4 py-4 sm:px-6 sm:py-6"
      :aria-label="msg('docs.navigation')"
    >
      <Button
        v-for="link in docsLinks"
        :key="link.page"
        :href="link.href"
        :data-active="data.docPage === link.page"
        :aria-current="data.docPage === link.page ? 'page' : undefined"
        variant="ghost"
        size="sm"
        >{{ msg(link.label) }}</Button
      >
      <Button class="ml-auto" :href="rawDocument" variant="ghost">{{ msg('docs.raw') }}</Button>
    </nav>
  </div>
</template>
