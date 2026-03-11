<template>
  <div class="p-4">
    <section class="flex justify-between items-center">
      <h4 class="scroll-m-20 tracking-tight">
        {{ curProvider?.name }}
      </h4>
    </section>
    <Separator class="mt-4 mb-6" />

    <ProviderForm
      :provider="curProvider"
      :edit-loading="editLoading"
      :delete-loading="deleteLoading"
      @submit="changeProvider"
      @delete="deleteProvider"
    />

    <Separator class="mt-4 mb-6" />

    <section class="flex justify-between items-center mb-4">
      <h4 class="scroll-m-20 font-semibold tracking-tight">
        {{ $t('models.title') }}
      </h4>
      <div class="flex gap-2">
        <Button
          type="button"
          variant="outline"
          :disabled="importLoading || !curProvider?.id"
          @click="runImportModels"
        >
          <Spinner v-if="importLoading" class="size-4 mr-2" />
          <FontAwesomeIcon
            v-else
            :icon="['fas', 'cloud-arrow-down']"
            class="mr-2"
          />
          {{ $t('models.importFromProvider') }}
        </Button>
      </div>
    </section>

    <!-- Import Result -->
    <section
      v-if="importResult"
      class="mb-4 rounded-lg border p-4 space-y-3 text-sm"
    >
      <div class="flex items-center gap-2">
        <span
          class="inline-block size-2 rounded-full"
          :class="importResult.imported > 0 ? 'bg-green-500' : 'bg-yellow-500'"
        />
        <span class="font-medium">
          {{ $t('models.importedCount', { count: importResult.imported }) }}
        </span>
      </div>
      <div
        v-if="importResult.models && importResult.models.length > 0"
        class="text-xs text-muted-foreground"
      >
        {{ importResult.models.join(', ') }}
      </div>
      <div
        v-if="importResult.errors && importResult.errors.length > 0"
        class="text-destructive text-xs"
      >
        <div v-for="(err, idx) in importResult.errors.slice(0, 3)" :key="idx">
          {{ err }}
        </div>
        <div v-if="importResult.errors.length > 3">
          {{ $t('models.moreErrors', { count: importResult.errors.length - 3 }) }}
        </div>
      </div>
    </section>

    <ModelList
      :provider-id="curProvider?.id"
      :models="modelDataList"
      :delete-model-loading="deleteModelLoading"
      @edit="handleEditModel"
      @delete="deleteModel"
    />
  </div>
</template>

<script setup lang="ts">
import { Separator, Button, Spinner } from '@memoh/ui'
import ProviderForm from './components/provider-form.vue'
import ModelList from './components/model-list.vue'
import { computed, inject, provide, reactive, ref, toRef, watch } from 'vue'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import { putProvidersById, deleteProvidersById, getProvidersByIdModels, deleteModelsModelByModelId } from '@memoh/sdk'
import type { ModelsGetResponse, ProvidersGetResponse } from '@memoh/sdk'

interface ImportModelsResponse {
  imported: number
  models: string[]
  errors?: string[]
}

// ---- Model 编辑状态（provide 给 CreateModel） ----
const openModel = reactive<{
  state: boolean
  title: 'title' | 'edit'
  curState: ModelsGetResponse | null
}>({
  state: false,
  title: 'title',
  curState: null,
})

provide('openModel', toRef(openModel, 'state'))
provide('openModelTitle', toRef(openModel, 'title'))
provide('openModelState', toRef(openModel, 'curState'))

function handleEditModel(model: ModelsGetResponse) {
  openModel.state = true
  openModel.title = 'edit'
  openModel.curState = { ...model }
}

// ---- 当前 Provider ----
const curProvider = inject('curProvider', ref<ProvidersGetResponse>())
const curProviderId = computed(() => curProvider.value?.id)

// ---- API Hooks ----
const queryCache = useQueryCache()

const { mutate: deleteProvider, isLoading: deleteLoading } = useMutation({
  mutation: async () => {
    if (!curProviderId.value) return
    await deleteProvidersById({ path: { id: curProviderId.value }, throwOnError: true })
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['providers'] }),
})

const { mutate: changeProvider, isLoading: editLoading } = useMutation({
  mutation: async (data: Record<string, unknown>) => {
    if (!curProviderId.value) return
    const { data: result } = await putProvidersById({
      path: { id: curProviderId.value },
      body: data as any,
      throwOnError: true,
    })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['providers'] }),
})

const { mutate: deleteModel, isLoading: deleteModelLoading } = useMutation({
  mutation: async (modelName: string) => {
    await deleteModelsModelByModelId({ path: { modelId: modelName }, throwOnError: true })
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['provider-models'] }),
})

const { data: modelDataList } = useQuery({
  key: () => ['provider-models', curProviderId.value ?? ''],
  query: async () => {
    if (!curProviderId.value) return []
    const { data } = await getProvidersByIdModels({
      path: { id: curProviderId.value },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!curProviderId.value,
})

watch(curProvider, () => {
  queryCache.invalidateQueries({ key: ['provider-models'] })
}, { immediate: true })

// ---- Import Models from Provider ----
const importLoading = ref(false)
const importResult = ref<ImportModelsResponse | null>(null)

async function runImportModels() {
  if (!curProviderId.value) return
  importLoading.value = true
  importResult.value = null
  try {
    const token = localStorage.getItem('token')
    const res = await fetch(`/api/providers/${curProviderId.value}/import-models`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': token ? `Bearer ${token}` : '',
      },
      body: JSON.stringify({ type: 'chat' }),
    })
    if (res.ok) {
      importResult.value = await res.json()
      // Refresh model list after import
      queryCache.invalidateQueries({ key: ['provider-models'] })
    } else {
      const err = await res.text()
      importResult.value = { imported: 0, models: [], errors: [err || 'Import failed'] }
    }
  } catch (err: unknown) {
    importResult.value = { imported: 0, models: [], errors: [err instanceof Error ? err.message : 'Import failed'] }
  } finally {
    importLoading.value = false
  }
}
</script>
