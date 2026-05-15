import axios from 'axios'
import { useRoomStore } from '../stores/room'

const http = axios.create({ baseURL: '/api', timeout: 10000 })

// 自动注入该房间的 Bearer token
http.interceptors.request.use((cfg) => {
  const store = useRoomStore()
  const url = cfg.url || ''
  const m = url.match(/^\/clip\/([^/]+)/)
  if (m && !/\/meta$|\/auth$/.test(url)) {
    const token = store.getToken(m[1])
    if (token) {
      cfg.headers = cfg.headers || {}
      ;(cfg.headers as any)['Authorization'] = `Bearer ${token}`
    }
  }
  return cfg
})

export interface ClipData {
  code: string
  content: string
  contentType: string
  updatedAt: string
  expireAt?: string
}

export interface MetaData {
  code: string
  name: string
  hasPassword: boolean
  expireAt: string
  updatedAt: string
}

export interface RoomItem {
  code: string
  name: string
  hasPassword: boolean
  createdAt: string
  updatedAt: string
  expireAt: string
}

export async function createClip(name: string, password: string) {
  const { data } = await http.post('/clip', { name, password })
  return data.data as { code: string; name: string; hasPassword: boolean; token: string }
}

export async function getMeta(code: string) {
  const { data } = await http.get(`/clip/${code}/meta`)
  return data.data as MetaData
}

export async function authRoom(code: string, password: string) {
  const { data } = await http.post(`/clip/${code}/auth`, { password })
  return data.data as { token: string }
}

export async function getContent(code: string) {
  const { data } = await http.get(`/clip/${code}`)
  return data.data as ClipData
}

export async function putContent(code: string, content: string, contentType = 'text') {
  const { data } = await http.put(`/clip/${code}`, { content, contentType })
  return data.data as ClipData
}

export async function listRooms() {
  const { data } = await http.get('/rooms')
  return data.data as RoomItem[]
}

export async function patchRoom(code: string, payload: { name?: string; password?: string }) {
  const { data } = await http.patch(`/clip/${code}`, payload)
  return data.data as { code: string; name: string; hasPassword: boolean; updatedAt: string }
}

export async function deleteRoom(code: string) {
  const { data } = await http.delete(`/clip/${code}`)
  return data.data as { code: string; deleted: boolean }
}

// ---- 消息接口 ----

export interface Message {
  id: number
  content: string
  contentType: string
  createdAt: string
}

export async function listMessages(code: string, beforeId = 0, limit = 50) {
  const { data } = await http.get(`/clip/${code}/messages`, {
    params: { beforeId, limit },
  })
  return data.data as { items: Message[]; hasMore: boolean }
}

export async function sendMessage(code: string, content: string, contentType = 'text') {
  const { data } = await http.post(`/clip/${code}/messages`, { content, contentType })
  return data.data as Message
}

export async function deleteMessage(code: string, id: number) {
  const { data } = await http.delete(`/clip/${code}/messages/${id}`)
  return data.data as { id: number; deleted: boolean }
}
