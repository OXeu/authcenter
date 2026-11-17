<script setup lang="ts">
import Button from '@oxeu/center-common/ui/Button.vue'
import { navigateDocument } from '@oxeu/center-common/ui/navigation.ts'
import { ref } from 'vue'
import { api } from './core.ts'
import type { PageData } from './data.ts'
import type { PageContext } from './page.ts'
const props = defineProps<{ data: PageData; page: PageContext }>()
const { msg } = props.page
const authorization = props.data.authorization!
const busy = ref(false)
const logo = `/applications/${encodeURIComponent(authorization.client.id)}/logo`
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
  <section class="card mt-4 max-w-xl space-y-4">
    <img
      v-if="!authorization.client.dynamic"
      :src="logo"
      alt=""
      width="64"
      height="64"
      class="rounded-sm"
    />
    <h2 class="text-lg font-semibold">{{ authorization.client.name }}</h2>
    <p v-if="authorization.client.homepage" class="break-all">
      {{ authorization.client.homepage }}
    </p>
    <p>{{ msg('oauth.requestAccess') }}</p>
    <ul class="list-disc pl-5">
      <li v-for="scope in authorization.scopes" :key="scope">{{ msg(`oauth.scope.${scope}`) }}</li>
    </ul>
    <p class="text-sm text-muted-foreground">{{ msg('oauth.account') }}: {{ data.me?.email }}</p>
    <p class="break-all text-sm text-muted-foreground">
      {{ msg('apps.callbacks') }}: {{ authorization.redirect_uri }}
    </p>
    <div class="flex gap-2">
      <Button :disabled="busy" @click="decide(false)" variant="secondary">
        {{ msg('oauth.deny') }}
      </Button>
      <Button :disabled="busy" @click="decide(true)" variant="primary">{{
        msg('oauth.allow')
      }}</Button>
    </div>
  </section>
</template>
