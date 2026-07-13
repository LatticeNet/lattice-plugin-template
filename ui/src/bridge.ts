const DEFAULT_TIMEOUT_MS = 15_000

export type HostTheme = {
  colorScheme?: 'light' | 'dark' | 'system'
  designTokens?: Record<string, unknown>
}

export type HostInterface = {
  service: string
  methods: string[]
}

export type HostInitPayload = HostTheme & {
  version: string
  pluginId: string
  pluginVersion: string
  pluginRoute: string
  locale: string
  interfaces: HostInterface[]
}

type HostInitMessage = HostInitPayload & {
  type: 'lattice.host.init'
  nonce: string
}

type HostResultMessage = {
  type: 'lattice.host.result'
  nonce: string
  id: string
  result: unknown
}

type HostErrorMessage = {
  type: 'lattice.host.error'
  nonce: string
  id?: string
  code: string
  message: string
}

type HostThemeMessage = HostTheme & {
  type: 'lattice.host.theme'
  nonce: string
}

type HostDisposeMessage = {
  type: 'lattice.host.dispose'
  nonce: string
}

type HostMessage =
  | HostInitMessage
  | HostResultMessage
  | HostErrorMessage
  | HostThemeMessage
  | HostDisposeMessage

type PendingRequest<T> = {
  id: string
  promise: Promise<T>
  cancel: (reason?: string) => void
}

type Deferred<T> = {
  promise: Promise<T>
  resolve: (value: T | PromiseLike<T>) => void
  reject: (reason?: unknown) => void
}

type InflightRequest = {
  resolve: (value: unknown) => void
  reject: (reason?: unknown) => void
  timer: ReturnType<typeof setTimeout> | null
}

export class BridgeError extends Error {
  constructor(message: string) {
    super(message)
    this.name = new.target.name
  }
}

export class BridgeRemoteError extends BridgeError {
  readonly code: string

  constructor(code: string, message: string) {
    super(message)
    this.code = code
  }
}

export class BridgeCancelledError extends BridgeError {}

export class BridgeTimeoutError extends BridgeError {}

export class BridgeDisposedError extends BridgeError {}

export class BridgeClient {
  readonly nonce: string
  readonly init: Promise<HostInitPayload>
  theme: HostTheme | null = null

  private readonly win: Window
  private readonly initDeferred: Deferred<HostInitPayload>
  private readonly inflight = new Map<string, InflightRequest>()
  private readonly themeListeners = new Set<(theme: HostTheme) => void>()
  private readonly onMessageBound: (event: MessageEvent) => void
  private disposed = false
  private requestSeq = 0
  private initResolved = false

  constructor(win: Window) {
    this.win = win
    this.nonce = readNonce(win)
    this.initDeferred = createDeferred<HostInitPayload>()
    this.init = this.initDeferred.promise
    this.init.catch(() => {})
    this.onMessageBound = (event: MessageEvent) => this.onMessage(event)
    this.win.addEventListener('message', this.onMessageBound)
    this.post({
      type: 'lattice.plugin.ready',
      nonce: this.nonce
    })
  }

  call<T>(
    service: string,
    method: string,
    payload: unknown,
    options?: { timeoutMs?: number }
  ): PendingRequest<T> {
    if (this.disposed) {
      throw new BridgeDisposedError('bridge has been disposed')
    }

    const id = `req-${++this.requestSeq}`
    const timeoutMs = options?.timeoutMs ?? DEFAULT_TIMEOUT_MS
    const deferred = createDeferred<T>()
    deferred.promise.catch(() => {})

    const timer =
      timeoutMs > 0
        ? setTimeout(() => {
            this.post({
              type: 'lattice.plugin.cancel',
              nonce: this.nonce,
              id
            })
            this.finishRequest(id, new BridgeTimeoutError(`timed out after ${timeoutMs}ms`))
          }, timeoutMs)
        : null

    this.inflight.set(id, {
      resolve: deferred.resolve as (value: unknown) => void,
      reject: deferred.reject,
      timer
    })

    this.post({
      type: 'lattice.plugin.call',
      nonce: this.nonce,
      id,
      service,
      method,
      payload
    })

    return {
      id,
      promise: deferred.promise,
      cancel: (reason = 'request cancelled') => {
        this.post({
          type: 'lattice.plugin.cancel',
          nonce: this.nonce,
          id
        })
        this.finishRequest(id, new BridgeCancelledError(reason))
      }
    }
  }

  resize(height: number) {
    if (this.disposed) {
      return
    }
    this.post({
      type: 'lattice.plugin.resize',
      nonce: this.nonce,
      height: Math.max(0, Math.ceil(height))
    })
  }

  subscribeTheme(listener: (theme: HostTheme) => void) {
    this.themeListeners.add(listener)
    return () => {
      this.themeListeners.delete(listener)
    }
  }

  dispose(reason = 'bridge disposed') {
    if (this.disposed) {
      return
    }
    this.disposed = true
    this.win.removeEventListener('message', this.onMessageBound)
    for (const [id, request] of this.inflight.entries()) {
      clearTimer(request.timer)
      request.reject(new BridgeDisposedError(reason))
      this.inflight.delete(id)
    }
    if (!this.initResolved) {
      this.initDeferred.reject(new BridgeDisposedError(reason))
    }
  }

  private onMessage(event: MessageEvent) {
    if (event.source !== this.win.parent) {
      return
    }

    const data = event.data as Partial<HostMessage> | undefined
    if (!isHostMessage(data) || data.nonce !== this.nonce) {
      return
    }

    switch (data.type) {
      case 'lattice.host.init': {
        this.theme = {
          colorScheme: data.colorScheme,
          designTokens: data.designTokens
        }
        this.emitTheme()
        this.initResolved = true
        this.initDeferred.resolve({
          version: data.version,
          pluginId: data.pluginId,
          pluginVersion: data.pluginVersion,
          pluginRoute: data.pluginRoute,
          locale: data.locale,
          interfaces: data.interfaces,
          colorScheme: data.colorScheme,
          designTokens: data.designTokens
        })
        return
      }
      case 'lattice.host.result': {
        const request = this.inflight.get(data.id)
        if (!request) {
          return
        }
        clearTimer(request.timer)
        this.inflight.delete(data.id)
        request.resolve(data.result)
        return
      }
      case 'lattice.host.error': {
        const error = new BridgeRemoteError(data.code, data.message)
        if (data.id) {
          this.finishRequest(data.id, error)
          return
        }
        if (!this.initResolved) {
          this.initDeferred.reject(error)
        }
        for (const id of this.inflight.keys()) {
          this.finishRequest(id, error)
        }
        return
      }
      case 'lattice.host.theme':
        this.theme = {
          colorScheme: data.colorScheme,
          designTokens: data.designTokens
        }
        this.emitTheme()
        return
      case 'lattice.host.dispose':
        this.dispose('host disposed bridge')
        return
    }
  }

  private emitTheme() {
    if (!this.theme) {
      return
    }
    for (const listener of this.themeListeners) {
      listener(this.theme)
    }
  }

  private finishRequest(id: string, error: Error) {
    const request = this.inflight.get(id)
    if (!request) {
      return
    }
    clearTimer(request.timer)
    request.reject(error)
    this.inflight.delete(id)
  }

  private post(message: Record<string, unknown>) {
    this.win.parent.postMessage(message, '*')
  }
}

function createDeferred<T>(): Deferred<T> {
  let resolve!: Deferred<T>['resolve']
  let reject!: Deferred<T>['reject']
  const promise = new Promise<T>((innerResolve, innerReject) => {
    resolve = innerResolve
    reject = innerReject
  })
  return { promise, resolve, reject }
}

function clearTimer(timer: ReturnType<typeof setTimeout> | null) {
  if (timer) {
    clearTimeout(timer)
  }
}

function isHostMessage(value: Partial<HostMessage> | undefined): value is HostMessage {
  return !!value && typeof value === 'object' && typeof value.type === 'string'
}

function readNonce(win: Window): string {
  const params = new URLSearchParams(win.location.hash.replace(/^#/, ''))
  const nonce = params.get('lattice_nonce')
  if (!nonce) {
    throw new BridgeError('missing lattice_nonce in URL fragment')
  }
  return nonce
}
