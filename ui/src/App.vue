<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { Activity, ShieldCheck, Workflow } from '@lucide/vue'

import { BridgeClient, type CallableInterface as HostInterface, type HostInit, type HostTheme } from '@latticenet/plugin-bridge'

let bridge: BridgeClient | null = null
const bootstrapError = ref('')

try {
  bridge = new BridgeClient({ window, expectedPluginId: 'example.lattice-plugin', expectedRoutes: ['reference'], idPrefix: 'template' })
} catch (cause: unknown) {
  bootstrapError.value = cause instanceof Error ? cause.message : 'bridge bootstrap failed'
}

const hostStatus = ref('Waiting for host initialization')
const hostInit = ref<HostInit | null>(null)
const hostInterfaces = ref<HostInterface[]>([])
const theme = ref<HostTheme | null>(null)
const nodeId = ref('node-a')
const publicPorts = ref('80,443')
const plan = ref('')
const error = ref('')
const loading = ref(false)

const appearance = computed(() => theme.value?.colorScheme ?? 'system')
const portSummary = computed(() =>
  publicPorts.value
    .split(',')
    .map((value) => value.trim())
    .filter(Boolean)
    .join(', ')
)
const canPlan = computed(() =>
  hostInterfaces.value.some(
    (entry) =>
      entry.service === 'example.lattice-plugin/reference' && entry.methods.includes('plan')
  )
)

if (bridge) {
  bridge.init
    .then((payload) => {
      hostInit.value = payload
      hostInterfaces.value = payload.interfaces
      theme.value = {
        colorScheme: payload.colorScheme,
        designTokens: payload.designTokens
      }
      hostStatus.value = 'Host connected'
    })
    .catch((cause: unknown) => {
      const message = cause instanceof Error ? cause.message : 'bridge initialization failed'
      hostStatus.value = message
      error.value = message
    })
} else {
  hostStatus.value = 'Bridge bootstrap failed'
  error.value = bootstrapError.value
}

let resizeObserver: ResizeObserver | null = null
let unsubscribeTheme: (() => void) | undefined

function syncTheme() {
  theme.value = bridge?.theme ?? null
}

function resizeToContent() {
  bridge?.resize(document.documentElement.scrollHeight)
}

async function requestPlan() {
  if (!bridge) {
    error.value = bootstrapError.value
    return
  }
  if (!canPlan.value) {
    error.value = 'The host did not expose the plan method for this session.'
    return
  }
  loading.value = true
  error.value = ''
  try {
    const ports = publicPorts.value
      .split(',')
      .map((value) => Number.parseInt(value.trim(), 10))
      .filter((value) => Number.isFinite(value))

    const request = bridge.call<{ plan?: string; result?: unknown }>(
      'example.lattice-plugin/reference',
      'plan',
      {
        node_id: nodeId.value,
        public_tcp: ports
      }
    )
    const result = await request.promise
    plan.value =
      typeof result === 'object' && result && 'plan' in result && typeof result.plan === 'string'
        ? result.plan
        : JSON.stringify(result, null, 2)
    hostStatus.value = 'Dry-run plan received'
  } catch (cause: unknown) {
    const message = cause instanceof Error ? cause.message : 'request failed'
    error.value = message
    hostStatus.value = 'Host call failed'
  } finally {
    loading.value = false
    await nextTick()
    resizeToContent()
  }
}

onMounted(() => {
  syncTheme()
  unsubscribeTheme = bridge?.subscribeTheme((nextTheme) => {
    theme.value = nextTheme
  })
  resizeObserver = new ResizeObserver(() => {
    resizeToContent()
  })
  resizeObserver.observe(document.body)
  void nextTick().then(() => resizeToContent())
})

onBeforeUnmount(() => {
  unsubscribeTheme?.()
  resizeObserver?.disconnect()
  bridge?.dispose('ui unmounted')
})
</script>

<template>
  <main class="shell" :data-appearance="appearance">
    <section class="hero">
      <div class="hero-copy">
        <p class="eyebrow">Bundle v2 Reference</p>
        <h1>Self-contained plugin with deterministic packaging</h1>
        <p class="lede">
          This example keeps runtime, UI, and package boundaries explicit so a host can
          verify ownership, isolate execution, and reproduce identical artifacts.
        </p>
      </div>
      <div class="hero-status">
        <span class="pill">
          <Activity :size="16" />
          {{ hostStatus }}
        </span>
        <span class="pill pill-secondary">
          <ShieldCheck :size="16" />
          Appearance: {{ appearance }}
        </span>
      </div>
    </section>

    <section class="grid">
      <article class="panel">
        <header class="panel-header">
          <Workflow :size="18" />
          <div>
            <h2>Dry-run planner</h2>
            <p>Example bridge call using host-routed `example.plan`.</p>
          </div>
        </header>

        <label class="field">
          <span>Node id</span>
          <input v-model="nodeId" type="text" autocomplete="off" />
        </label>

        <label class="field">
          <span>Public TCP ports</span>
          <input v-model="publicPorts" type="text" autocomplete="off" />
          <small>Comma-separated ports: {{ portSummary || 'none' }}</small>
        </label>

        <button
          v-if="canPlan"
          class="action"
          type="button"
          :disabled="loading"
          @click="requestPlan"
        >
          {{ loading ? 'Requesting plan…' : 'Generate dry-run plan' }}
        </button>
        <p v-else class="notice">
          The host did not expose `example.lattice-plugin/reference.plan` for this session.
        </p>

        <p v-if="bootstrapError" class="notice notice-error">{{ bootstrapError }}</p>
        <p v-if="error" class="notice notice-error">{{ error }}</p>
      </article>

      <article class="panel">
        <header class="panel-header">
          <ShieldCheck :size="18" />
          <div>
            <h2>Sandbox state</h2>
            <p>Host metadata delivered through bridge `init` and `theme` messages.</p>
          </div>
        </header>

        <pre class="code-block">{{ JSON.stringify(hostInit, null, 2) }}</pre>
      </article>
    </section>

    <section class="panel">
      <header class="panel-header">
        <Activity :size="18" />
        <div>
          <h2>Last dry-run output</h2>
          <p>Plan content returned from the system runtime through the host bridge.</p>
        </div>
      </header>

      <pre class="code-block">{{ plan || 'No plan requested yet.' }}</pre>
    </section>
  </main>
</template>
