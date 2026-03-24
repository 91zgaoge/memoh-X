<template>
  <div class="space-y-6">
    <!-- Header -->
    <div class="flex items-center justify-between">
      <div>
        <h3 class="text-lg font-semibold">
          {{ channelItem.meta.display_name }}
        </h3>
        <p class="text-sm text-muted-foreground">
          {{ channelItem.meta.type }}
        </p>
      </div>
      <Badge :variant="channelItem.configured ? 'default' : 'secondary'">
        {{ channelItem.configured ? $t('bots.channels.configured') : $t('bots.channels.notConfigured') }}
      </Badge>
    </div>

    <Separator />

    <!-- WeChat: Special display with Webhook URL and API Key -->
    <div
      v-if="channelItem.meta.type === 'wechat'"
      class="space-y-4"
    >
      <h4 class="text-sm font-medium">
        {{ $t('bots.channels.wechatConfig') }}
      </h4>

      <!-- Webhook URL -->
      <div class="space-y-2">
        <Label>{{ $t('bots.channels.webhookUrl') }}</Label>
        <div class="flex items-center gap-2">
          <code class="flex-1 rounded-md border border-border bg-muted/50 px-3 py-2 font-mono text-sm break-all select-all">
            {{ wechatWebhookUrl }}
          </code>
          <Button
            variant="outline"
            size="sm"
            @click="copyToClipboard(wechatWebhookUrl, $t('bots.channels.webhookUrlCopied'))"
          >
            <FontAwesomeIcon
              :icon="['fas', 'copy']"
              class="size-3.5"
            />
          </Button>
        </div>
        <p class="text-xs text-muted-foreground">
          {{ $t('bots.channels.webhookUrlDesc') }}
        </p>
      </div>

      <!-- API Key -->
      <div class="space-y-2">
        <Label>{{ $t('bots.channels.apiKey') }}</Label>
        <div class="flex items-center gap-2">
          <code class="flex-1 rounded-md border border-border bg-muted/50 px-3 py-2 font-mono text-sm break-all select-all">
            {{ wechatApiKeyVisible ? (wechatApiKey || '...') : '••••••••••••••••' }}
          </code>
          <Button
            variant="outline"
            size="sm"
            @click="wechatApiKeyVisible = !wechatApiKeyVisible"
          >
            <FontAwesomeIcon
              :icon="['fas', wechatApiKeyVisible ? 'eye-slash' : 'eye']"
              class="size-3.5"
            />
          </Button>
          <Button
            variant="outline"
            size="sm"
            :disabled="!wechatApiKey"
            @click="copyToClipboard(wechatApiKey || '', $t('bots.channels.apiKeyCopied'))"
          >
            <FontAwesomeIcon
              :icon="['fas', 'copy']"
              class="size-3.5"
            />
          </Button>
        </div>
        <p class="text-xs text-muted-foreground">
          {{ $t('bots.channels.apiKeyDesc') }}
        </p>
      </div>

      <!-- Copy config as JSON -->
      <div class="flex gap-2">
        <Button
          variant="outline"
          size="sm"
          :disabled="!wechatApiKey"
          @click="copyWeChatConfig"
        >
          <FontAwesomeIcon
            :icon="['fas', 'file-code']"
            class="mr-2 size-3.5"
          />
          {{ $t('bots.channels.copyConfig') }}
        </Button>
      </div>

      <!-- Quick start guide -->
      <div class="rounded-md bg-muted p-3 text-sm space-y-2">
        <p class="font-medium">
          {{ $t('bots.channels.quickStartGuide') }}
        </p>
        <ol class="list-decimal list-inside space-y-1 text-muted-foreground">
          <li>{{ $t('bots.channels.step1') }}</li>
          <li>{{ $t('bots.channels.step2') }}</li>
          <li>{{ $t('bots.channels.step3') }}</li>
        </ol>
      </div>

      <!-- Generate API Key if not exists -->
      <div
        v-if="!wechatApiKey"
        class="flex justify-end"
      >
        <Button
          :disabled="isGeneratingKey"
          @click="generateApiKey"
        >
          <Spinner v-if="isGeneratingKey" />
          {{ $t('bots.channels.generateApiKey') }}
        </Button>
      </div>
    </div>

    <!-- WeChat Personal (weixin): Bridge + QR code -->
    <div
      v-else-if="channelItem.meta.type === 'weixin'"
      class="space-y-4"
    >
      <h4 class="text-sm font-medium">
        {{ $t('bots.channels.weixinConfig') }}
      </h4>

      <!-- API Key -->
      <div class="space-y-2">
        <Label>{{ $t('bots.channels.apiKey') }}</Label>
        <div class="flex items-center gap-2">
          <code class="flex-1 rounded-md border border-border bg-muted/50 px-3 py-2 font-mono text-sm break-all select-all">
            {{ weixinApiKeyVisible ? (weixinApiKey || '...') : '••••••••••••••••' }}
          </code>
          <Button
            variant="outline"
            size="sm"
            @click="weixinApiKeyVisible = !weixinApiKeyVisible"
          >
            <FontAwesomeIcon
              :icon="['fas', weixinApiKeyVisible ? 'eye-slash' : 'eye']"
              class="size-3.5"
            />
          </Button>
          <Button
            variant="outline"
            size="sm"
            :disabled="!weixinApiKey"
            @click="copyToClipboard(weixinApiKey || '', $t('bots.channels.apiKeyCopied'))"
          >
            <FontAwesomeIcon
              :icon="['fas', 'copy']"
              class="size-3.5"
            />
          </Button>
        </div>
      </div>

      <!-- Bridge status -->
      <div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
        <div class="flex items-center gap-2 text-sm">
          <span class="text-muted-foreground">{{ $t('bots.channels.weixinBridgeStatus') }}</span>
          <span
            class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
            :class="weixinBridgeStatusClass"
          >
            {{ weixinBridgeStatusLabel }}
          </span>
        </div>
        <Button
          variant="ghost"
          size="sm"
          :disabled="isRefreshingStatus"
          @click="refreshWeixinStatus"
        >
          <FontAwesomeIcon
            :icon="['fas', 'rotate']"
            class="size-3.5"
            :class="{ 'animate-spin': isRefreshingStatus }"
          />
        </Button>
      </div>

      <!-- Start Bridge button (shown only when bridge is stopped) -->
      <div
        v-if="weixinBridgeStatus === 'stopped'"
        class="space-y-1"
      >
        <Button
          class="w-full"
          :disabled="!weixinApiKey || isStartingBridge"
          @click="startWeixinBridge"
        >
          <FontAwesomeIcon
            :icon="['fas', 'play']"
            class="mr-2 size-3.5"
          />
          {{ isStartingBridge ? $t('bots.channels.weixinBridgeStarting') : $t('bots.channels.weixinStartBridge') }}
        </Button>
      </div>

      <!-- Stop Bridge button (shown when bridge is not stopped - including unknown, running, etc.) -->
      <div
        v-else
        class="space-y-1"
      >
        <Button
          variant="destructive"
          class="w-full"
          :disabled="isStoppingBridge"
          @click="stopWeixinBridge"
        >
          <FontAwesomeIcon
            :icon="['fas', 'stop']"
            class="mr-2 size-3.5"
          />
          {{ isStoppingBridge ? $t('bots.channels.weixinBridgeStopping') : $t('bots.channels.weixinStopBridge') }}
        </Button>
      </div>

      <!-- Scan QR code button -->
      <div class="space-y-1">
        <Button
          class="w-full"
          :disabled="!weixinApiKey || weixinBridgeStatus === 'unknown' || weixinBridgeStatus === 'stopped'"
          @click="openWeixinQRCode"
        >
          <FontAwesomeIcon
            :icon="['fas', 'qrcode']"
            class="mr-2 size-3.5"
          />
          {{ $t('bots.channels.weixinScanQRCode') }}
        </Button>
        <p class="text-xs text-muted-foreground text-center">
          {{ $t('bots.channels.weixinScanQRCodeDesc') }}
        </p>
      </div>

      <!-- Setup instructions -->
      <div class="rounded-md bg-muted p-3 text-sm space-y-2">
        <p class="font-medium">
          {{ $t('bots.channels.weixinQuickStart') }}
        </p>
        <ol class="list-decimal list-inside space-y-1 text-muted-foreground">
          <li>{{ $t('bots.channels.weixinStep1') }}</li>
          <li>{{ $t('bots.channels.weixinStep2') }}</li>
          <li>{{ $t('bots.channels.weixinStep3') }}</li>
        </ol>
      </div>

      <!-- Generate API Key if not exists -->
      <div
        v-if="!weixinApiKey"
        class="flex justify-end"
      >
        <Button
          :disabled="isGeneratingKey"
          @click="generateWeixinApiKey"
        >
          <Spinner v-if="isGeneratingKey" />
          {{ $t('bots.channels.generateApiKey') }}
        </Button>
      </div>
    </div>

    <!-- Credentials form (dynamic from config_schema) -->
    <div
      v-else
      class="space-y-4"
    >
      <h4 class="text-sm font-medium">
        {{ $t('bots.channels.credentials') }}
      </h4>

      <div
        v-for="(field, key) in orderedFields"
        :key="key"
        class="space-y-2"
      >
        <Label>
          {{ field.title || key }}
          <span
            v-if="!field.required"
            class="text-xs text-muted-foreground ml-1"
          >({{ $t('common.optional') }})</span>
        </Label>
        <p
          v-if="field.description"
          class="text-xs text-muted-foreground"
        >
          {{ field.description }}
        </p>

        <!-- Secret field -->
        <div
          v-if="field.type === 'secret'"
          class="relative"
        >
          <Input
            v-model="form.credentials[key]"
            :type="visibleSecrets[key] ? 'text' : 'password'"
            :placeholder="field.example ? String(field.example) : ''"
          />
          <button
            type="button"
            class="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
            @click="visibleSecrets[key] = !visibleSecrets[key]"
          >
            <FontAwesomeIcon
              :icon="['fas', visibleSecrets[key] ? 'eye-slash' : 'eye']"
              class="size-3.5"
            />
          </button>
        </div>

        <!-- Boolean field -->
        <Switch
          v-else-if="field.type === 'bool'"
          :model-value="!!form.credentials[key]"
          @update:model-value="(val) => form.credentials[key] = !!val"
        />

        <!-- Number field -->
        <Input
          v-else-if="field.type === 'number'"
          v-model.number="form.credentials[key]"
          type="number"
          :placeholder="field.example ? String(field.example) : ''"
        />

        <!-- Enum field -->
        <Select
          v-else-if="field.type === 'enum' && field.enum"
          :model-value="String(form.credentials[key] || '')"
          @update:model-value="(val) => form.credentials[key] = val"
        >
          <SelectTrigger>
            <SelectValue :placeholder="field.title" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem
              v-for="opt in field.enum"
              :key="opt"
              :value="opt"
            >
              {{ opt }}
            </SelectItem>
          </SelectContent>
        </Select>

        <!-- String field (default) -->
        <Input
          v-else
          v-model="form.credentials[key]"
          type="text"
          :placeholder="field.example ? String(field.example) : ''"
        />
      </div>
    </div>

    <Separator />

    <!-- Status (not for WeChat) -->
    <div
      v-if="channelItem.meta.type !== 'wechat' && channelItem.meta.type !== 'weixin'"
      class="flex items-center justify-between"
    >
      <Label>{{ $t('common.status') }}</Label>
      <Switch
        :model-value="form.status === 'active'"
        @update:model-value="(val) => form.status = val ? 'active' : 'inactive'"
      />
    </div>

    <!-- Save (not for WeChat) -->
    <div
      v-if="channelItem.meta.type !== 'wechat' && channelItem.meta.type !== 'weixin'"
      class="flex justify-end"
    >
      <Button
        :disabled="isLoading"
        @click="handleSave"
      >
        <Spinner v-if="isLoading" />
        {{ $t('bots.channels.save') }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  Badge,
  Button,
  Input,
  Label,
  Separator,
  Switch,
  Spinner,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from '@memoh/ui'
import { reactive, watch, computed, ref } from 'vue'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { useMutation, useQueryCache } from '@pinia/colada'
import { putBotsByIdChannelByPlatform } from '@memoh/sdk'
import type { HandlersChannelMeta, ChannelChannelConfig, ChannelFieldSchema } from '@memoh/sdk'
import type { Ref } from 'vue'

interface BotChannelItem {
  meta: HandlersChannelMeta
  config: ChannelChannelConfig | null
  configured: boolean
}

const props = defineProps<{
  botId: string
  channelItem: BotChannelItem
}>()

const emit = defineEmits<{
  saved: []
}>()

const { t } = useI18n()
const botIdRef = computed(() => props.botId) as Ref<string>
const queryCache = useQueryCache()
const { mutateAsync: upsertChannel, isLoading } = useMutation({
  mutation: async ({ platform, data }: { platform: string; data: Record<string, unknown> }) => {
    const { data: result } = await putBotsByIdChannelByPlatform({
      path: { id: botIdRef.value, platform },
      body: data as any,
      throwOnError: true,
    })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['bot-channels', botIdRef.value] }),
})

// ---- Form state ----

const form = reactive<{
  credentials: Record<string, unknown>
  status: string
}>({
  credentials: {},
  status: 'inactive',
})

const visibleSecrets = reactive<Record<string, boolean>>({})

// ---- WeChat specific ----

const wechatApiKeyVisible = ref(false)
const wechatApiKey = computed<string | null>(() => {
  if (props.channelItem.meta.type !== 'wechat') return null
  const creds = props.channelItem.config?.credentials
  return creds?.api_key as string ?? null
})

const wechatWebhookUrl = computed(() => {
  const baseUrl = window.location.origin
  return `${baseUrl}/api/channels/wechat/webhook/${botIdRef.value}`
})

const isGeneratingKey = ref(false)

async function generateApiKey() {
  isGeneratingKey.value = true
  try {
    const token = localStorage.getItem('token') || ''
    const response = await fetch(`/api/bots/${botIdRef.value}/preauth_keys`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${token}`,
      },
      body: JSON.stringify({ ttl_seconds: 365 * 24 * 60 * 60 }),
    })

    if (!response.ok) {
      throw new Error('Failed to generate API key')
    }

    const data = await response.json()
    const apiKey = data.token || data.Token

    if (!apiKey) {
      throw new Error('API key not found in response')
    }

    await upsertChannel({
      platform: 'wechat',
      data: {
        credentials: { api_key: apiKey },
        status: 'active',
      },
    })

    toast.success(t('bots.channels.apiKeyGenerated'))
    emit('saved')
  } catch {
    toast.error(t('bots.channels.apiKeyGenerateFailed'))
  } finally {
    isGeneratingKey.value = false
  }
}

function copyToClipboard(text: string, successMsg: string) {
  navigator.clipboard.writeText(text).then(() => {
    toast.success(successMsg)
  }).catch(() => {
    toast.error(t('common.copyFailed'))
  })
}

function copyWeChatConfig() {
  const config = {
    webhook_url: wechatWebhookUrl.value,
    api_key: wechatApiKey.value,
    bot_id: botIdRef.value,
  }
  copyToClipboard(JSON.stringify(config, null, 2), t('bots.channels.configCopied'))
}

// ---- WeChat Personal (weixin) specific ----

const weixinApiKeyVisible = ref(false)
const weixinApiKey = computed<string | null>(() => {
  if (props.channelItem.meta.type !== 'weixin') return null
  const creds = props.channelItem.config?.credentials
  return creds?.api_key as string ?? null
})

const weixinBridgeUrl = computed(() => `/api/weixin-bridge/${botIdRef.value}`)
const weixinBridgeStatus = ref<string>('unknown')
const isRefreshingStatus = ref(false)
const isStartingBridge = ref(false)
const isStoppingBridge = ref(false)

const weixinBridgeStatusLabel = computed(() => {
  const map: Record<string, string> = {
    connected: t('bots.channels.weixinBridgeStatusConnected'),
    connecting: t('bots.channels.weixinBridgeStatusConnecting'),
    disconnected: t('bots.channels.weixinBridgeStatusDisconnected'),
    pending: t('bots.channels.weixinBridgeStatusPending'),
  }
  return map[weixinBridgeStatus.value] ?? t('bots.channels.weixinBridgeStatusUnknown')
})

const weixinBridgeStatusClass = computed(() => {
  switch (weixinBridgeStatus.value) {
    case 'connected':
      return 'bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300'
    case 'connecting':
      return 'bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300'
    case 'disconnected':
      return 'bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-300'
    default:
      return 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400'
  }
})

async function refreshWeixinStatus() {
  isRefreshingStatus.value = true
  try {
    const token = localStorage.getItem('token') || ''
    const resp = await fetch(`${weixinBridgeUrl.value}/info`, {
      headers: {
        'Authorization': `Bearer ${token}`,
      },
      signal: AbortSignal.timeout(5000),
    })
    if (resp.ok) {
      const data = await resp.json()
      // info endpoint returns { success: true, info: { status: 'running' | 'stopped' | ... } }
      weixinBridgeStatus.value = data.info?.status ?? 'unknown'
    } else if (resp.status === 404) {
      // Bridge not found means it's not running
      weixinBridgeStatus.value = 'stopped'
    } else {
      weixinBridgeStatus.value = 'unknown'
    }
  } catch {
    weixinBridgeStatus.value = 'unknown'
  } finally {
    isRefreshingStatus.value = false
  }
}

function openWeixinQRCode() {
  window.open(`${weixinBridgeUrl.value}/qrcode`, '_blank')
}

async function generateWeixinApiKey() {
  isGeneratingKey.value = true
  try {
    const token = localStorage.getItem('token') || ''
    const response = await fetch(`/api/bots/${botIdRef.value}/preauth_keys`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${token}`,
      },
      body: JSON.stringify({ ttl_seconds: 365 * 24 * 60 * 60 }),
    })

    if (!response.ok) throw new Error('Failed to generate API key')

    const data = await response.json()
    const apiKey = data.token || data.Token
    if (!apiKey) throw new Error('API key not found in response')

    await upsertChannel({
      platform: 'weixin',
      data: {
        credentials: { api_key: apiKey },
        status: 'active',
      },
    })

    toast.success(t('bots.channels.apiKeyGenerated'))
    emit('saved')
  } catch {
    toast.error(t('bots.channels.apiKeyGenerateFailed'))
  } finally {
    isGeneratingKey.value = false
  }
}

// Stop weixin-bridge container for this bot
async function stopWeixinBridge() {
  isStoppingBridge.value = true
  try {
    const token = localStorage.getItem('token') || ''
    const response = await fetch(`/api/weixin-bridge/${botIdRef.value}`, {
      method: 'DELETE',
      headers: {
        'Authorization': `Bearer ${token}`,
      },
    })

    const data = await response.json()

    if (!response.ok) {
      // Even if not found in backend memory, we treat it as success
      // since the container might already be gone
      if (response.status === 404) {
        weixinBridgeStatus.value = 'stopped'
        toast.success(t('bots.channels.weixinBridgeStopped'))
        return
      }
      throw new Error(data.error || data.message || 'Failed to stop bridge')
    }

    weixinBridgeStatus.value = 'stopped'
    toast.success(t('bots.channels.weixinBridgeStopped'))
  } catch (err: any) {
    toast.error(err?.message || t('bots.channels.weixinBridgeStopFailed'))
  } finally {
    isStoppingBridge.value = false
  }
}

// Start weixin-bridge container for this bot
async function startWeixinBridge() {
  if (!weixinApiKey.value) {
    toast.error(t('bots.channels.weixinApiKeyRequired'))
    return
  }

  isStartingBridge.value = true
  let pollInterval: NodeJS.Timeout | null = null

  try {
    const token = localStorage.getItem('token') || ''
    const response = await fetch('/api/weixin-bridge', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${token}`,
      },
      body: JSON.stringify({
        bot_id: botIdRef.value,
        api_key: weixinApiKey.value,
      }),
    })

    const data = await response.json()

    if (!response.ok || !data.success) {
      throw new Error(data.error || data.message || 'Failed to start bridge')
    }

    // Start polling for status
    let pollCount = 0
    const maxPolls = 30 // 30 seconds max

    pollInterval = setInterval(async () => {
      pollCount++

      try {
        const token = localStorage.getItem('token') || ''
        const statusResp = await fetch(`${weixinBridgeUrl.value}/info`, {
          headers: {
            'Authorization': `Bearer ${token}`,
          },
          signal: AbortSignal.timeout(5000),
        })

        if (statusResp.ok) {
          const statusData = await statusResp.json()
          weixinBridgeStatus.value = statusData.info?.status ?? 'unknown'

          // Check if bridge is running
          if (statusData.info?.status === 'running') {
            if (pollInterval) {
              clearInterval(pollInterval)
              pollInterval = null
            }
            isStartingBridge.value = false
            toast.success(t('bots.channels.weixinBridgeStarted'))
          }
        }
      } catch {
        // Ignore polling errors
      }

      // Timeout after max polls
      if (pollCount >= maxPolls) {
        if (pollInterval) {
          clearInterval(pollInterval)
          pollInterval = null
        }
        isStartingBridge.value = false
        toast.error(t('bots.channels.weixinBridgeStartTimeout'))
      }
    }, 1000)

  } catch (err: any) {
    toast.error(err?.message || t('bots.channels.weixinBridgeStartFailed'))
    isStartingBridge.value = false
    if (pollInterval) {
      clearInterval(pollInterval)
    }
  }
}

// 切换到 weixin 渠道时自动刷新 bridge 状态
watch(
  () => props.channelItem.meta.type,
  (type) => {
    if (type === 'weixin') refreshWeixinStatus()
  },
  { immediate: true },
)

// Schema fields sorted: required first
const orderedFields = computed(() => {
  const fields = props.channelItem.meta.config_schema?.fields ?? {}
  const entries = Object.entries(fields)
  entries.sort(([, a], [, b]) => {
    if (a.required && !b.required) return -1
    if (!a.required && b.required) return 1
    return 0
  })
  return Object.fromEntries(entries) as Record<string, ChannelFieldSchema>
})

// 初始化表单
function initForm() {
  const schema = props.channelItem.meta.config_schema?.fields ?? {}
  const existingCredentials = props.channelItem.config?.credentials ?? {}

  const creds: Record<string, unknown> = {}
  for (const key of Object.keys(schema)) {
    creds[key] = existingCredentials[key] ?? ''
  }
  form.credentials = creds
  const rawStatus = props.channelItem.config?.status ?? 'inactive'
  form.status = (rawStatus === 'active' || rawStatus === 'verified') ? 'active' : 'inactive'
}

watch(
  () => props.channelItem,
  () => initForm(),
  { immediate: true },
)

// 客户端校验必填字段
function validateRequired(): boolean {
  const schema = props.channelItem.meta.config_schema?.fields ?? {}
  for (const [key, field] of Object.entries(schema)) {
    if (field.required) {
      const val = form.credentials[key]
      if (!val || (typeof val === 'string' && val.trim() === '')) {
        toast.error(t('bots.channels.requiredField', { field: field.title || key }))
        return false
      }
    }
  }
  return true
}

async function handleSave() {
  if (!validateRequired()) return

  try {
    const credentials: Record<string, unknown> = {}
    for (const [key, val] of Object.entries(form.credentials)) {
      if (val !== '' && val !== undefined && val !== null) {
        credentials[key] = val
      }
    }

    await upsertChannel({
      platform: props.channelItem.meta.type,
      data: {
        credentials,
        status: form.status,
      },
    })
    toast.success(t('bots.channels.saveSuccess'))
    emit('saved')
  } catch (err) {
    let detail = ''
    if (err instanceof Error) {
      detail = err.message
    }
    toast.error(detail ? `${t('bots.channels.saveFailed')}: ${detail}` : t('bots.channels.saveFailed'))
  }
}
</script>
