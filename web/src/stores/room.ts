import { defineStore } from 'pinia'

const STORAGE_KEY = 'webclip.tokens'

function load(): Record<string, string> {
  try {
    return JSON.parse(sessionStorage.getItem(STORAGE_KEY) || '{}')
  } catch {
    return {}
  }
}

function save(tokens: Record<string, string>) {
  sessionStorage.setItem(STORAGE_KEY, JSON.stringify(tokens))
}

export const useRoomStore = defineStore('room', {
  state: () => ({
    tokens: load() as Record<string, string>,
  }),
  actions: {
    setToken(code: string, token: string) {
      this.tokens[code] = token
      save(this.tokens)
    },
    clearToken(code: string) {
      delete this.tokens[code]
      save(this.tokens)
    },
    getToken(code: string): string | undefined {
      return this.tokens[code]
    },
  },
})
