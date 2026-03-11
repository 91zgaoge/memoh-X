<template>
  <form @submit="editProvider">
    <div class="**:[input]:mt-3 **:[input]:mb-4">
      <section>
        <h4 class="scroll-m-20 font-semibold tracking-tight">
          {{ $t('common.name') }}
        </h4>
        <FormField
          v-slot="{ componentField }"
          name="name"
        >
          <FormItem>
            <FormControl>
              <Input
                type="text"
                :placeholder="$t('common.namePlaceholder')"
                v-bind="componentField"
              />
            </FormControl>
          </FormItem>
        </FormField>
      </section>

      <section>
        <h4 class="scroll-m-20 font-semibold tracking-tight">
          {{ $t('common.type') }}
        </h4>
        <p class="text-xs text-muted-foreground mt-1">{{ getProviderLabel(provider?.client_type || '') }}</p>
      </section>

      <section>
        <h4 class="scroll-m-20 font-semibold tracking-tight">
          {{ $t('provider.apiKey') }}
        </h4>
        <p class="text-xs text-muted-foreground mt-1">{{ $t('provider.apiKeyHint') }}</p>
        <FormField
          v-slot="{ componentField }"
          name="api_key"
        >
          <FormItem>
            <FormControl>
              <Input
                type="password"
                :placeholder="props.provider?.api_key || $t('provider.apiKeyPlaceholder')"
                v-bind="componentField"
              />
            </FormControl>
          </FormItem>
        </FormField>
      </section>

      <section>
        <h4 class="scroll-m-20 font-semibold tracking-tight">
          {{ $t('provider.url') }}
        </h4>
        <p class="text-xs text-muted-foreground mt-1">{{ $t('provider.urlHint') }}</p>
        <FormField
          v-slot="{ componentField }"
          name="base_url"
        >
          <FormItem>
            <FormControl>
              <Input
                type="text"
                :placeholder="$t('provider.urlPlaceholder')"
                v-bind="componentField"
              />
            </FormControl>
          </FormItem>
        </FormField>
      </section>
    </div>

    <section class="flex justify-between items-center mt-4 gap-4">
      <Button
        type="button"
        variant="outline"
        :disabled="testLoading || !props.provider?.id"
        @click="runTest"
      >
        <Spinner v-if="testLoading" />
        <FontAwesomeIcon
          v-else
          :icon="['fas', 'rotate']"
        />
        {{ $t('provider.testConnection') }}
      </Button>

      <div class="flex gap-4">
        <ConfirmPopover
          :message="$t('provider.deleteConfirm')"
          :loading="deleteLoading"
          @confirm="$emit('delete')"
        >
          <template #trigger>
            <Button variant="outline">
              <FontAwesomeIcon :icon="['far', 'trash-can']" />
            </Button>
          </template>
        </ConfirmPopover>

        <Button
          type="submit"
          :disabled="!hasChanges || !form.meta.value.valid"
        >
          <Spinner v-if="editLoading" />
          {{ $t('provider.saveChanges') }}
        </Button>
      </div>
    </section>

    <section
      v-if="testResult"
      class="mt-4 rounded-lg border p-4 space-y-3 text-sm"
    >
      <div class="flex items-center gap-2">
        <span
          class="inline-block size-2 rounded-full"
          :class="testResult.reachable ? 'bg-green-500' : 'bg-red-500'"
        />
        <span class="font-medium">
          {{ testResult.reachable ? $t('provider.reachable') : $t('provider.unreachable') }}
        </span>
        <span
          v-if="testResult.latency_ms"
          class="text-muted-foreground"
        >
          {{ testResult.latency_ms }}ms
        </span>
      </div>

      <div
        v-if="testResult.message"
        class="text-muted-foreground text-xs"
      >
        {{ testResult.message }}
      </div>

      <div
        v-if="testError"
        class="text-destructive text-xs"
      >
        {{ testError }}
      </div>
    </section>
  </form>
</template>

<script setup lang="ts">
import {
  Input,
  Button,
  FormControl,
  FormField,
  FormItem,
  Spinner,
} from '@memoh/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import { computed, ref, watch } from 'vue'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useForm } from 'vee-validate'
import type { ProvidersGetResponse } from '@memoh/sdk'
import { getProviderLabel } from '@/data/model-catalog'
interface TestResponse {
  reachable: boolean
  latency_ms?: number
  message?: string
}

const props = defineProps<{
  provider: Partial<ProvidersGetResponse> | undefined
  editLoading: boolean
  deleteLoading: boolean
}>()

const emit = defineEmits<{
  submit: [values: Record<string, unknown>]
  delete: []
}>()

const testLoading = ref(false)
const testResult = ref<TestResponse | null>(null)
const testError = ref('')

async function runTest() {
  if (!props.provider?.id) return
  testLoading.value = true
  testResult.value = null
  testError.value = ''
  try {
    const token = localStorage.getItem('token')
    const res = await fetch(`/api/providers/${props.provider.id}/test`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': token ? `Bearer ${token}` : '',
      },
    })
    if (res.ok) {
      testResult.value = await res.json()
    } else {
      const err = await res.text()
      testError.value = err || 'Test failed'
    }
  } catch (err: unknown) {
    testError.value = err instanceof Error ? err.message : 'Test failed'
  } finally {
    testLoading.value = false
  }
}

watch(() => props.provider?.id, (newId) => {
  if (newId) {
    runTest()
  }
}, { immediate: true })

const providerSchema = toTypedSchema(z.object({
  name: z.string().min(1),
  base_url: z.string().min(1),
  client_type: z.string().min(1),
  api_key: z.string().optional(),
  metadata: z.object({
    additionalProp1: z.object({}),
  }),
}))

const form = useForm({
  validationSchema: providerSchema,
})

watch(() => props.provider, (newVal) => {
  if (newVal) {
    form.setValues({
      name: newVal.name,
      base_url: newVal.base_url,
      client_type: newVal.client_type,
      api_key: '',
    })
  }
}, { immediate: true })

const hasChanges = computed(() => {
  const raw = props.provider
  const baseChanged = JSON.stringify({
    name: form.values.name,
    base_url: form.values.base_url,
    client_type: form.values.client_type,
    metadata: form.values.metadata,
  }) !== JSON.stringify({
    name: raw?.name,
    base_url: raw?.base_url,
    client_type: raw?.client_type,
    metadata: { additionalProp1: {} },
  })

  const apiKeyChanged = Boolean(form.values.api_key && form.values.api_key.trim() !== '')
  return baseChanged || apiKeyChanged
})

const editProvider = form.handleSubmit(async (value) => {
  const payload: Record<string, unknown> = {
    name: value.name,
    base_url: value.base_url,
    client_type: value.client_type,
    metadata: value.metadata,
  }
  if (value.api_key && value.api_key.trim() !== '') {
    payload.api_key = value.api_key
  }
  emit('submit', payload)
})
</script>
