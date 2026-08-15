import { nextTick, ref } from 'vue'

export interface ConfirmOptions {
  title?: string
  message: string
  confirmText?: string
  cancelText?: string
  danger?: boolean
}

export interface PromptOptions {
  title?: string
  message?: string
  defaultValue?: string
  placeholder?: string
  confirmText?: string
  cancelText?: string
  danger?: boolean
}

type ConfirmRequest = {
  id: number
  kind: 'confirm'
  options: ConfirmOptions
  resolve: (value: boolean) => void
}

type PromptRequest = {
  id: number
  kind: 'prompt'
  options: PromptOptions
  resolve: (value: string | null) => void
}

export type DialogRequest = ConfirmRequest | PromptRequest

const activeRequest = ref<DialogRequest | null>(null)
const pendingRequests: DialogRequest[] = []
let requestId = 0

function drainQueue() {
  if (!activeRequest.value && pendingRequests.length > 0) {
    activeRequest.value = pendingRequests.shift() ?? null
  }
}

function settleActive(value: boolean | string | null) {
  const request = activeRequest.value
  if (!request) return

  activeRequest.value = null
  if (request.kind === 'confirm') {
    request.resolve(Boolean(value))
  } else {
    request.resolve(typeof value === 'string' ? value : null)
  }

  void nextTick(drainQueue)
}

function confirmDialog(options: ConfirmOptions): Promise<boolean> {
  return new Promise((resolve) => {
    pendingRequests.push({ id: ++requestId, kind: 'confirm', options, resolve })
    drainQueue()
  })
}

function promptDialog(options: PromptOptions): Promise<string | null> {
  return new Promise((resolve) => {
    pendingRequests.push({ id: ++requestId, kind: 'prompt', options, resolve })
    drainQueue()
  })
}

export function useDialogService() {
  return {
    confirm: confirmDialog,
    prompt: promptDialog,
  }
}

export function useDialogHost() {
  return {
    activeRequest,
    settle: settleActive,
  }
}
