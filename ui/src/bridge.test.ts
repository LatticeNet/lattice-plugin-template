import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import {
  BridgeCancelledError,
  BridgeClient,
  BridgeDisposedError,
  BridgeTimeoutError
} from './bridge'

function dispatchHostMessage(
  data: Record<string, unknown>,
  source: MessageEventSource | null = window.parent
) {
  window.dispatchEvent(
    new MessageEvent('message', {
      data: {
        nonce: 'nonce-123',
        ...data
      },
      source
    })
  )
}

describe('BridgeClient', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
    window.location.hash = '#lattice_nonce=nonce-123'
  })

  afterEach(() => {
    window.location.hash = ''
  })

  test('retries ready until host init and then stops', () => {
    vi.useFakeTimers()
    const parentSpy = vi.spyOn(window.parent, 'postMessage')

    const bridge = new BridgeClient(window)

    expect(bridge.nonce).toBe('nonce-123')
    expect(parentSpy).toHaveBeenCalledWith(
      {
        type: 'lattice.plugin.ready',
        nonce: 'nonce-123'
      },
      '*'
    )
    vi.advanceTimersByTime(500)
    expect(parentSpy).toHaveBeenCalledTimes(2)

    dispatchHostMessage({
      type: 'lattice.host.init',
      version: '1',
      pluginId: 'example.lattice-plugin',
      pluginVersion: '0.2.1-alpha.3',
      pluginRoute: 'reference',
      locale: 'en-US',
      interfaces: [],
      colorScheme: 'light',
      designTokens: {}
    })
    vi.advanceTimersByTime(1_000)
    expect(parentSpy).toHaveBeenCalledTimes(2)
    bridge.dispose()

    window.location.hash = ''
    expect(() => new BridgeClient(window)).toThrow(/lattice_nonce/)
  })

  test('assigns monotonic request ids to outbound calls', () => {
    const parentSpy = vi.spyOn(window.parent, 'postMessage')
    const bridge = new BridgeClient(window)

    const first = bridge.call('example.lattice-plugin/reference', 'describe', {})
    const second = bridge.call('example.lattice-plugin/reference', 'plan', { ports: [80, 443] })

    expect(first.id).toBe('req-1')
    expect(second.id).toBe('req-2')
    expect(parentSpy).toHaveBeenNthCalledWith(
      2,
      {
        type: 'lattice.plugin.call',
        nonce: 'nonce-123',
        id: 'req-1',
        service: 'example.lattice-plugin/reference',
        method: 'describe',
        payload: {}
      },
      '*'
    )
    expect(parentSpy).toHaveBeenNthCalledWith(
      3,
      {
        type: 'lattice.plugin.call',
        nonce: 'nonce-123',
        id: 'req-2',
        service: 'example.lattice-plugin/reference',
        method: 'plan',
        payload: { ports: [80, 443] }
      },
      '*'
    )
  })

  test('stops ready retries after the host rejects initialization', async () => {
    vi.useFakeTimers()
    const parentSpy = vi.spyOn(window.parent, 'postMessage')
    const bridge = new BridgeClient(window)

    dispatchHostMessage({
      type: 'lattice.host.error',
      code: 'denied',
      message: 'Initialization denied'
    })

    await expect(bridge.init).rejects.toMatchObject({
      name: 'BridgeRemoteError',
      code: 'denied',
      message: 'Initialization denied'
    })
    vi.advanceTimersByTime(1_000)
    expect(parentSpy).toHaveBeenCalledTimes(1)
    bridge.dispose()
  })

  test('routes host init, result, error, and theme messages only from parent with matching nonce', async () => {
    const bridge = new BridgeClient(window)
    const first = bridge.call('example.lattice-plugin/reference', 'describe', {})
    const second = bridge.call('example.lattice-plugin/reference', 'plan', { ports: [80, 443] })

    const initPromise = bridge.init

    dispatchHostMessage({
      type: 'lattice.host.init',
      version: '1',
      pluginId: 'example.lattice-plugin',
      pluginVersion: '0.2.1-alpha.3',
      pluginRoute: 'reference',
      locale: 'en-US',
      interfaces: [
        {
          service: 'example.lattice-plugin/reference',
          methods: ['describe', 'plan']
        }
      ],
      colorScheme: 'dark',
      designTokens: { accent: '#0ea5e9' }
    })
    dispatchHostMessage({
      type: 'lattice.host.theme',
      colorScheme: 'dark',
      designTokens: { accent: '#0ea5e9' }
    })
    dispatchHostMessage(
      {
        type: 'lattice.host.result',
        id: first.id,
        result: { ignored: true }
      },
      null
    )
    window.dispatchEvent(
      new MessageEvent('message', {
        data: {
          type: 'lattice.host.result',
          nonce: 'wrong-nonce',
          id: first.id,
          result: { ignored: true }
        },
        source: window.parent
      })
    )
    dispatchHostMessage({
      type: 'lattice.host.result',
      id: second.id,
      result: { plan: 'ok' }
    })
    dispatchHostMessage({
      type: 'lattice.host.error',
      id: first.id,
      code: 'boom',
      message: 'host exploded'
    })

    await expect(initPromise).resolves.toEqual({
      version: '1',
      pluginId: 'example.lattice-plugin',
      pluginVersion: '0.2.1-alpha.3',
      pluginRoute: 'reference',
      locale: 'en-US',
      interfaces: [
        {
          service: 'example.lattice-plugin/reference',
          methods: ['describe', 'plan']
        }
      ],
      colorScheme: 'dark',
      designTokens: { accent: '#0ea5e9' }
    })
    await expect(second.promise).resolves.toEqual({ plan: 'ok' })
    await expect(first.promise).rejects.toMatchObject({
      name: 'BridgeRemoteError',
      code: 'boom',
      message: 'host exploded'
    })
    expect(bridge.theme).toEqual({
      colorScheme: 'dark',
      designTokens: { accent: '#0ea5e9' }
    })
  })

  test('cancels pending calls and notifies the host', async () => {
    const parentSpy = vi.spyOn(window.parent, 'postMessage')
    const bridge = new BridgeClient(window)
    const request = bridge.call('example.lattice-plugin/reference', 'plan', { ports: [80, 443] })

    request.cancel('user cancelled')

    await expect(request.promise).rejects.toBeInstanceOf(BridgeCancelledError)
    expect(parentSpy).toHaveBeenLastCalledWith(
      {
        type: 'lattice.plugin.cancel',
        nonce: 'nonce-123',
        id: request.id
      },
      '*'
    )
  })

  test('sends resize events using the exact host wire shape', () => {
    const parentSpy = vi.spyOn(window.parent, 'postMessage')
    const bridge = new BridgeClient(window)

    bridge.resize(123.2)

    expect(parentSpy).toHaveBeenLastCalledWith(
      {
        type: 'lattice.plugin.resize',
        nonce: 'nonce-123',
        height: 124
      },
      '*'
    )
  })

  test('times out pending calls and sends a cancel message', async () => {
    vi.useFakeTimers()
    const parentSpy = vi.spyOn(window.parent, 'postMessage')
    const bridge = new BridgeClient(window)
    const request = bridge.call(
      'example.lattice-plugin/reference',
      'plan',
      { ports: [80, 443] },
      { timeoutMs: 250 }
    )

    await vi.advanceTimersByTimeAsync(250)

    await expect(request.promise).rejects.toBeInstanceOf(BridgeTimeoutError)
    expect(parentSpy).toHaveBeenLastCalledWith(
      {
        type: 'lattice.plugin.cancel',
        nonce: 'nonce-123',
        id: request.id
      },
      '*'
    )
  })

  test('rejects in-flight calls on host dispose and refuses new calls afterward', async () => {
    const bridge = new BridgeClient(window)
    const request = bridge.call('example.lattice-plugin/reference', 'plan', { ports: [80, 443] })

    dispatchHostMessage({
      type: 'lattice.host.dispose'
    })

    await expect(request.promise).rejects.toBeInstanceOf(BridgeDisposedError)
    expect(() => bridge.call('example.lattice-plugin/reference', 'describe', {})).toThrow(/disposed/i)
  })
})
