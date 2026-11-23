<script setup lang="ts">
import ConsentCard from '@oxeu/center-common/ui/ConsentCard.vue'
import { navigateDocument } from '@oxeu/center-common/ui/navigation.ts'
import { computed, ref } from 'vue'
import { api } from './core.ts'
import type { PageData } from './data.ts'
import type { PageContext } from './page.ts'
const props = defineProps<{ data: PageData; page: PageContext }>()
const { msg } = props.page
const authorization = props.data.authorization!
const busy = ref(false)
const logo = computed(() =>
  authorization.client.dynamic
    ? ''
    : `/applications/${encodeURIComponent(authorization.client.id)}/logo`,
)
const scopes = computed(() => authorization.scopes.map((scope) => msg(`oauth.scope.${scope}`)))
const details = computed(() => [
  { label: msg('oauth.account'), value: props.data.me?.email ?? '' },
  { label: msg('oauth.callback'), value: authorization.redirect_uri },
])
async function decide(approve: boolean) {
  if (busy.value) return
  busy.value = true
  try {
    const result = (await api('/oauth/authorize', {
      method: 'POST',
      csrf: props.data.csrf,
      body: { request: authorization.id, approve },
    })) as { redirect: string }
    navigateDocument(result.redirect, 'oauth-consent')
  } catch (error) {
    props.page.report(error)
  } finally {
    busy.value = false
  }
}
</script>
<template>
  <ConsentCard
    :title="authorization.client.name"
    :subtitle="authorization.client.homepage"
    :description="msg('oauth.requestAccess')"
    :logo-src="logo"
    :scopes="scopes"
    :details="details"
    :allow-label="msg('oauth.allow')"
    :deny-label="msg('oauth.deny')"
    :busy="busy"
    deny-first
    @allow="decide(true)"
    @deny="decide(false)"
  />
</template>
