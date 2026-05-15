<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import {
  getMeta,
  authRoom,
  listMessages,
  sendMessage,
  deleteMessage,
  type Message,
} from '../api/clip'
import { useRoomStore } from '../stores/room'

const route = useRoute()
const store = useRoomStore()

const code = computed(() => String(route.params.code || '').toUpperCase())

const loading = ref(true)
const notFound = ref(false)
const needPassword = ref(false)
const passwordInput = ref('')
const authError = ref('')

const wsStatus = ref<'idle' | 'connecting' | 'open' | 'closed'>('idle')
const copiedTip = ref('')

// ---- 消息列表 ----
const messages = ref<Message[]>([])
const hasMore = ref(false)
const loadingMore = ref(false)

// ---- 输入区 ----
const inputContent = ref('')
const sending = ref(false)
const inputRef = ref<HTMLTextAreaElement | null>(null)

let clientId = Math.random().toString(36).slice(2, 10)
let ws: WebSocket | null = null
let reconnectTimer: number | null = null
let reconnectDelay = 1000

function buildWsUrl(token: string): string {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${proto}//${location.host}/api/ws/${code.value}?token=${encodeURIComponent(token)}`
}

function connect(token: string) {
  wsStatus.value = 'connecting'
  const w = new WebSocket(buildWsUrl(token))
  ws = w

  w.onopen = () => {
    wsStatus.value = 'open'
    reconnectDelay = 1000
  }

  w.onmessage = (ev: MessageEvent) => {
    let msg: any
    try {
      msg = JSON.parse(ev.data)
    } catch {
      return
    }
    if (msg.type === 'message_created' && msg.message) {
      const m = msg.message as Message
      // 去重：可能本端已通过 HTTP 拿到
      if (!messages.value.find(x => x.id === m.id)) {
        messages.value.unshift(m)
      }
    } else if (msg.type === 'message_deleted') {
      messages.value = messages.value.filter(x => x.id !== msg.id)
    } else if (msg.type === 'error') {
      console.warn('ws error:', msg.error)
    }
  }

  w.onclose = () => {
    wsStatus.value = 'closed'
    ws = null
    scheduleReconnect(token)
  }

  w.onerror = () => {
    try { w.close() } catch {}
  }
}

function scheduleReconnect(token: string) {
  if (reconnectTimer !== null) return
  reconnectTimer = window.setTimeout(() => {
    reconnectTimer = null
    connect(token)
    reconnectDelay = Math.min(reconnectDelay * 2, 30000)
  }, reconnectDelay)
}

async function init() {
  loading.value = true
  notFound.value = false
  needPassword.value = false
  try {
    const meta = await getMeta(code.value)
    let token = store.getToken(code.value)
    if (!token) {
      if (meta.hasPassword) {
        loading.value = false
        needPassword.value = true
        return
      }
      const r = await authRoom(code.value, '')
      token = r.token
      store.setToken(code.value, token)
    }
    await loadFirstPage()
    connect(token)
    nextTick(() => inputRef.value?.focus())
  } catch (e: any) {
    const status = e?.response?.status
    if (status === 404) notFound.value = true
    else if (status === 401) {
      store.clearToken(code.value)
      needPassword.value = true
    } else {
      console.error(e)
    }
  } finally {
    loading.value = false
  }
}

async function loadFirstPage() {
  try {
    const r = await listMessages(code.value, 0, 50)
    messages.value = r.items
    hasMore.value = r.hasMore
  } catch (e: any) {
    if (e?.response?.status === 401) {
      store.clearToken(code.value)
      needPassword.value = true
    } else {
      throw e
    }
  }
}

async function loadMore() {
  if (loadingMore.value || !hasMore.value) return
  const tail = messages.value[messages.value.length - 1]
  if (!tail) return
  loadingMore.value = true
  try {
    const r = await listMessages(code.value, tail.id, 50)
    messages.value.push(...r.items)
    hasMore.value = r.hasMore
  } catch (e) {
    console.error(e)
  } finally {
    loadingMore.value = false
  }
}

async function refresh() {
  await loadFirstPage()
}

async function doAuth() {
  authError.value = ''
  try {
    const r = await authRoom(code.value, passwordInput.value)
    store.setToken(code.value, r.token)
    needPassword.value = false
    await loadFirstPage()
    connect(r.token)
    nextTick(() => inputRef.value?.focus())
  } catch (e: any) {
    authError.value = e?.response?.status === 401 ? '密码错误' : '登录失败'
  }
}

// ---- 发送 ----
async function doSend() {
  const text = inputContent.value
  if (!text.trim() || sending.value) return
  sending.value = true
  try {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type: 'send', content: text, contentType: 'text' }))
      // 服务端会回推 message_created，前端那时再插入
    } else {
      // 降级走 HTTP
      const m = await sendMessage(code.value, text, 'text')
      if (!messages.value.find(x => x.id === m.id)) {
        messages.value.unshift(m)
      }
    }
    inputContent.value = ''
    nextTick(() => inputRef.value?.focus())
  } catch (e: any) {
    flashTip('发送失败')
  } finally {
    sending.value = false
  }
}

function clearInput() {
  inputContent.value = ''
  inputRef.value?.focus()
}

function onInputKeydown(e: KeyboardEvent) {
  // ⌘/Ctrl + Enter 发送
  if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
    e.preventDefault()
    doSend()
  }
}

// ---- 删除确认 ----
const pendingDelete = ref<Message | null>(null)

function confirmDelete(m: Message) {
  pendingDelete.value = m
}

function cancelDelete() {
  pendingDelete.value = null
}

async function doDelete() {
  const m = pendingDelete.value
  if (!m) return
  pendingDelete.value = null
  // 乐观移除
  const backup = messages.value
  messages.value = messages.value.filter(x => x.id !== m.id)
  try {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type: 'delete', id: m.id }))
    } else {
      await deleteMessage(code.value, m.id)
    }
  } catch (e) {
    messages.value = backup
    flashTip('删除失败')
  }
}

// ---- 复制 ----
function flashTip(text: string) {
  copiedTip.value = text
  setTimeout(() => { copiedTip.value = '' }, 1500)
}

async function copyText(text: string, label = '已复制') {
  try {
    await navigator.clipboard.writeText(text)
    flashTip(label)
  } catch {
    // 回退
    const ta = document.createElement('textarea')
    ta.value = text
    ta.style.position = 'fixed'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.select()
    try {
      document.execCommand('copy')
      flashTip(label)
    } catch {
      flashTip('复制失败')
    } finally {
      document.body.removeChild(ta)
    }
  }
}

async function copyMessage(m: Message) { await copyText(m.content, '已复制') }
async function copyCode() { await copyText(code.value, '已复制短码') }
async function copyLink() { await copyText(location.href, '已复制链接') }

function formatTime(s?: string) {
  if (!s) return ''
  return s.replace('T', ' ').replace(/\..+$/, '').slice(0, 16)
}

onMounted(init)
onUnmounted(() => {
  if (ws) { ws.onclose = null; try { ws.close() } catch {} ws = null }
  if (reconnectTimer !== null) window.clearTimeout(reconnectTimer)
})
</script>

<template>
  <div class="room">
    <header class="bar">
      <router-link to="/" class="back">← 返回</router-link>
      <h1>房间 <span class="code">{{ code }}</span></h1>
      <span class="status" :class="wsStatus">
        <template v-if="wsStatus === 'open'">● 已连接</template>
        <template v-else-if="wsStatus === 'connecting'">● 连接中…</template>
        <template v-else>● 已断开</template>
      </span>
    </header>

    <div v-if="loading" class="hint">加载中…</div>

    <div v-else-if="notFound" class="hint err">
      房间 <span class="code">{{ code }}</span> 不存在或已过期。
      <router-link to="/">返回首页</router-link>
    </div>

    <div v-else-if="needPassword" class="card pwd">
      <h2>该房间需要密码</h2>
      <input v-model="passwordInput" type="password" placeholder="请输入房间密码" @keyup.enter="doAuth" />
      <button @click="doAuth">进入</button>
      <p v-if="authError" class="err">{{ authError }}</p>
    </div>

    <template v-else>
      <!-- 输入区 -->
      <section class="input-card">
        <textarea
          ref="inputRef"
          v-model="inputContent"
          placeholder="粘贴或输入内容…   ⌘/Ctrl + Enter 发送"
          @keydown="onInputKeydown"
        ></textarea>
        <div class="input-toolbar">
          <button class="ghost" @click="copyCode">复制短码</button>
          <button class="ghost" @click="copyLink">复制链接</button>
          <span class="spacer"></span>
          <span v-if="copiedTip" class="tip">{{ copiedTip }}</span>
          <button class="ghost" @click="clearInput" :disabled="!inputContent">清空</button>
          <button class="primary" @click="doSend" :disabled="sending || !inputContent.trim()">
            {{ sending ? '发送中…' : '发送 ⌘↵' }}
          </button>
        </div>
      </section>

      <!-- 列表头 -->
      <div class="list-head">
        <span class="title">最近 {{ messages.length }} 条 · 倒序展示</span>
        <button class="ghost" @click="refresh">刷新</button>
      </div>

      <p v-if="messages.length === 0" class="empty">暂无消息，发送第一条吧</p>

      <ul class="msg-list">
        <li v-for="m in messages" :key="m.id" class="msg-row">
          <span class="msg-time">{{ formatTime(m.createdAt) }}</span>
          <pre class="msg-body">{{ m.content }}</pre>
          <div class="msg-actions">
            <button class="act-btn copy" @click="copyMessage(m)">复制</button>
            <button class="act-btn del" @click="confirmDelete(m)">删除</button>
          </div>
        </li>
      </ul>

      <div v-if="hasMore" class="more">
        <button class="ghost" @click="loadMore" :disabled="loadingMore">
          {{ loadingMore ? '加载中…' : '↓ 加载更多' }}
        </button>
      </div>
    </template>

    <!-- 删除确认弹窗 -->
    <div v-if="pendingDelete" class="modal-mask" @click.self="cancelDelete">
      <div class="modal">
        <h3>确认删除</h3>
        <p>删除后无法恢复，确定要删除这条消息吗？</p>
        <div class="modal-actions">
          <button class="ghost" @click="cancelDelete">取消</button>
          <button class="act-btn del" @click="doDelete">删除</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.room { max-width: 760px; margin: 24px auto; padding: 0 16px; }

.bar { display: flex; align-items: center; gap: 16px; margin-bottom: 16px; }
.bar h1 { margin: 0; font-size: 18px; font-weight: 500; }
.back { color: #2563eb; text-decoration: none; font-size: 14px; }
.code { font-family: ui-monospace, monospace; background: #f3f4f6; padding: 3px 10px; border-radius: 4px; letter-spacing: 3px; font-weight: 600; }
.status { margin-left: auto; font-size: 13px; }
.status.open { color: #059669; }
.status.connecting { color: #d97706; }
.status.closed { color: #dc2626; }
.status.idle { color: #9ca3af; }

.hint { text-align: center; padding: 48px 0; color: #6b7280; }
.hint.err { color: #dc2626; }
.err { color: #dc2626; margin-top: 8px; font-size: 13px; }

/* 通用按钮 */
button { padding: 6px 12px; border-radius: 6px; cursor: pointer; font-size: 13px; border: 1px solid transparent; }
button:disabled { opacity: 0.6; cursor: not-allowed; }
button.primary { background: #2563eb; color: white; border-color: #2563eb; padding: 8px 16px; font-size: 14px; font-weight: 500; }
button.primary:hover:not(:disabled) { background: #1d4ed8; border-color: #1d4ed8; }
button.ghost { background: white; color: #111; border-color: #d1d5db; }
button.ghost:hover:not(:disabled) { background: #f3f4f6; }
button.ghost.danger { color: #dc2626; border-color: #fecaca; }
button.ghost.danger:hover:not(:disabled) { background: #fef2f2; }

/* 密码卡片 */
.card { background: white; border: 1px solid #e5e7eb; border-radius: 10px; padding: 24px; }
.pwd h2 { margin: 0 0 16px; font-size: 16px; }
.pwd input { width: 100%; padding: 10px 12px; border: 1px solid #d1d5db; border-radius: 6px; margin-bottom: 12px; outline: none; box-sizing: border-box; }
.pwd button { padding: 8px 16px; background: #2563eb; color: white; border: none; border-radius: 6px; cursor: pointer; }

/* 输入区 */
.input-card {
  background: white; border: 1px solid #e5e7eb; border-radius: 10px;
  padding: 12px; box-shadow: 0 1px 2px rgba(0,0,0,0.04); margin-bottom: 16px;
}
.input-card textarea {
  width: 100%; min-height: 110px; padding: 10px 12px; box-sizing: border-box;
  border: 1px solid #e5e7eb; border-radius: 6px; resize: vertical;
  font-family: ui-monospace, "SF Mono", Menlo, Consolas, monospace;
  font-size: 14px; line-height: 1.6; outline: none;
}
.input-card textarea:focus { border-color: #2563eb; box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.15); }
.input-toolbar { display: flex; align-items: center; gap: 8px; margin-top: 10px; flex-wrap: wrap; }
.input-toolbar .spacer { flex: 1; }
.input-toolbar .tip { color: #059669; font-size: 12px; }

/* 列表头 */
.list-head { display: flex; align-items: center; gap: 12px; margin: 0 4px 8px; }
.list-head .title { color: #374151; font-size: 14px; font-weight: 500; }
.list-head button { margin-left: auto; }

.empty { color: #9ca3af; text-align: center; padding: 32px 0; }

/* 消息列表 - 紧凑风格 */
.msg-list { list-style: none; padding: 0; margin: 0; }
.msg-row {
  display: flex; align-items: flex-start; gap: 10px;
  padding: 6px 8px; border-bottom: 1px solid #f3f4f6;
  transition: background .1s;
}
.msg-row:hover { background: #f9fafb; }
.msg-row:last-child { border-bottom: none; }
.msg-time {
  flex-shrink: 0; width: 90px; padding-top: 2px;
  color: #9ca3af; font-size: 12px; font-family: ui-monospace, monospace;
}
.msg-body {
  flex: 1; min-width: 0; margin: 0;
  white-space: pre-wrap; word-break: break-word;
  font-family: ui-monospace, "SF Mono", Menlo, Consolas, monospace;
  font-size: 13px; line-height: 1.5; color: #111827;
}
.msg-actions {
  flex-shrink: 0; display: flex; gap: 6px;
  opacity: 0; transition: opacity .15s;
}
.msg-row:hover .msg-actions { opacity: 1; }
.act-btn {
  padding: 3px 10px; border-radius: 5px; cursor: pointer;
  font-size: 12px; line-height: 1.4; border: 1px solid;
  background: white; transition: all .12s;
}
.act-btn.copy {
  color: #2563eb; border-color: #bfdbfe;
}
.act-btn.copy:hover {
  background: #eff6ff; border-color: #93c5fd;
}
.act-btn.del {
  color: #dc2626; border-color: #fecaca;
}
.act-btn.del:hover {
  background: #fef2f2; border-color: #fca5a5;
}

.more { text-align: center; margin: 16px 0; }

/* 删除确认弹窗 */
.modal-mask {
  position: fixed; inset: 0; z-index: 100;
  background: rgba(0,0,0,0.35); display: flex;
  align-items: center; justify-content: center;
}
.modal {
  background: white; border-radius: 12px; padding: 24px 28px;
  min-width: 320px; max-width: 420px; box-shadow: 0 8px 30px rgba(0,0,0,0.15);
}
.modal h3 { margin: 0 0 8px; font-size: 16px; font-weight: 600; }
.modal p { margin: 0 0 20px; color: #6b7280; font-size: 14px; line-height: 1.5; }
.modal-actions { display: flex; justify-content: flex-end; gap: 10px; }
</style>
