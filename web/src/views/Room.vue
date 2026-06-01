<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import {
  getMeta,
  authRoom,
  listMessages,
  sendMessage,
  deleteMessage,
  uploadFile,
  getFileDownloadUrl,
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

// ---- 标签页 ----
type TabName = 'text' | 'file'
const activeTab = ref<TabName>('text')

// ---- 文本消息 ----
const textMessages = ref<Message[]>([])
const textHasMore = ref(false)
const textLoadingMore = ref(false)

// ---- 文件消息 ----
const fileMessages = ref<Message[]>([])
const fileHasMore = ref(false)
const fileLoadingMore = ref(false)

// ---- 输入区 ----
const inputContent = ref('')
const sending = ref(false)
const inputRef = ref<HTMLTextAreaElement | null>(null)

// ---- 文件上传 ----
const uploading = ref(false)
const uploadProgress = ref(0)
const uploadFileName = ref('')
const uploadError = ref('')
const fileInputRef = ref<HTMLInputElement | null>(null)

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
      // 按 contentType 路由到对应标签页列表
      if (m.contentType === 'file') {
        if (!fileMessages.value.find(x => x.id === m.id)) {
          fileMessages.value.unshift(m)
        }
      } else {
        if (!textMessages.value.find(x => x.id === m.id)) {
          textMessages.value.unshift(m)
        }
      }
    } else if (msg.type === 'message_deleted') {
      textMessages.value = textMessages.value.filter(x => x.id !== msg.id)
      fileMessages.value = fileMessages.value.filter(x => x.id !== msg.id)
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
    await loadTextFirstPage()
    await loadFileFirstPage()
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

async function loadTextFirstPage() {
  try {
    const r = await listMessages(code.value, 0, 50, 'text')
    textMessages.value = r.items
    textHasMore.value = r.hasMore
  } catch (e: any) {
    if (e?.response?.status === 401) {
      store.clearToken(code.value)
      needPassword.value = true
    } else {
      throw e
    }
  }
}

async function loadFileFirstPage() {
  try {
    const r = await listMessages(code.value, 0, 50, 'file')
    fileMessages.value = r.items
    fileHasMore.value = r.hasMore
  } catch (e: any) {
    if (e?.response?.status === 401) {
      store.clearToken(code.value)
      needPassword.value = true
    } else {
      throw e
    }
  }
}

async function loadMoreText() {
  if (textLoadingMore.value || !textHasMore.value) return
  const tail = textMessages.value[textMessages.value.length - 1]
  if (!tail) return
  textLoadingMore.value = true
  try {
    const r = await listMessages(code.value, tail.id, 50, 'text')
    textMessages.value.push(...r.items)
    textHasMore.value = r.hasMore
  } catch (e) {
    console.error(e)
  } finally {
    textLoadingMore.value = false
  }
}

async function loadMoreFile() {
  if (fileLoadingMore.value || !fileHasMore.value) return
  const tail = fileMessages.value[fileMessages.value.length - 1]
  if (!tail) return
  fileLoadingMore.value = true
  try {
    const r = await listMessages(code.value, tail.id, 50, 'file')
    fileMessages.value.push(...r.items)
    fileHasMore.value = r.hasMore
  } catch (e) {
    console.error(e)
  } finally {
    fileLoadingMore.value = false
  }
}

async function refresh() {
  if (activeTab.value === 'text') await loadTextFirstPage()
  else await loadFileFirstPage()
}

async function doAuth() {
  authError.value = ''
  try {
    const r = await authRoom(code.value, passwordInput.value)
    store.setToken(code.value, r.token)
    needPassword.value = false
    await loadTextFirstPage()
    await loadFileFirstPage()
    connect(r.token)
    nextTick(() => inputRef.value?.focus())
  } catch (e: any) {
    authError.value = e?.response?.status === 401 ? '密码错误' : '登录失败'
  }
}

// ---- 文本发送 ----
async function doSend() {
  const text = inputContent.value
  if (!text.trim() || sending.value) return
  sending.value = true
  try {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type: 'send', content: text, contentType: 'text' }))
    } else {
      const m = await sendMessage(code.value, text, 'text')
      if (!textMessages.value.find(x => x.id === m.id)) {
        textMessages.value.unshift(m)
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
  if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
    e.preventDefault()
    doSend()
  }
}

// ---- 文件上传 ----
function triggerFileInput() {
  fileInputRef.value?.click()
}

async function onFileSelected(e: Event) {
  // 兼容两种来源：input change 事件取 input.files，drop 事件取 dataTransfer.files
  let file: File | undefined
  const dt = (e as DragEvent).dataTransfer
  if (dt) {
    file = dt.files?.[0]
  } else {
    const input = e.target as HTMLInputElement
    file = input.files?.[0]
    input.value = '' // reset，允许重复选择同一文件
  }
  if (!file) return

  uploading.value = true
  uploadProgress.value = 0
  uploadFileName.value = file.name
  uploadError.value = ''

  try {
    // 后端代理上传
    const msg = await uploadFile(code.value, file)
    // 上传成功，消息由后端通过 WS 广播，前端去重追加
    if (!fileMessages.value.find(x => x.id === msg.id)) {
      fileMessages.value.unshift(msg)
    }
    uploadProgress.value = 100
  } catch (e: any) {
    uploadError.value = e?.response?.data?.message || '上传失败，请重试'
    console.error('file upload error:', e)
  } finally {
    uploading.value = false
  }
}

// ---- 文件下载 ----
async function downloadFile(m: Message) {
  try {
    const { downloadUrl, fileName } = await getFileDownloadUrl(code.value, m.id)
    const a = document.createElement('a')
    a.href = downloadUrl
    a.download = fileName || 'download'
    a.target = '_blank'
    a.rel = 'noopener'
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
  } catch (e) {
    flashTip('下载失败')
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
  const backupText = textMessages.value
  const backupFile = fileMessages.value
  textMessages.value = textMessages.value.filter(x => x.id !== m.id)
  fileMessages.value = fileMessages.value.filter(x => x.id !== m.id)
  try {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type: 'delete', id: m.id }))
    } else {
      await deleteMessage(code.value, m.id)
    }
  } catch (e) {
    textMessages.value = backupText
    fileMessages.value = backupFile
    flashTip('删除失败')
  }
}

// ---- 工具 ----
function flashTip(text: string) {
  copiedTip.value = text
  setTimeout(() => { copiedTip.value = '' }, 1500)
}

async function copyText(text: string, label = '已复制') {
  try {
    await navigator.clipboard.writeText(text)
    flashTip(label)
  } catch {
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

function formatSize(bytes: number): string {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
  return (bytes / (1024 * 1024 * 1024)).toFixed(1) + ' GB'
}

function fileIcon(fileName: string): string {
  const ext = fileName.split('.').pop()?.toLowerCase() || ''
  if (['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg', 'bmp'].includes(ext)) return '🖼'
  if (['mp4', 'mov', 'avi', 'mkv', 'webm'].includes(ext)) return '🎬'
  if (['mp3', 'wav', 'ogg', 'flac', 'aac'].includes(ext)) return '🎵'
  if (['pdf'].includes(ext)) return '📄'
  if (['zip', 'rar', '7z', 'tar', 'gz'].includes(ext)) return '📦'
  if (['doc', 'docx'].includes(ext)) return '📝'
  if (['xls', 'xlsx'].includes(ext)) return '📊'
  if (['ppt', 'pptx'].includes(ext)) return '📑'
  return '📎'
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
      <!-- 标签页切换 -->
      <div class="tabs">
        <button class="tab" :class="{ active: activeTab === 'text' }" @click="activeTab = 'text'">文本</button>
        <button class="tab" :class="{ active: activeTab === 'file' }" @click="activeTab = 'file'">文件</button>
        <span class="spacer"></span>
        <button class="ghost" @click="refresh">刷新</button>
      </div>

      <!-- ==================== 文本标签页 ==================== -->
      <template v-if="activeTab === 'text'">
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

        <!-- 文本列表头 -->
        <div class="list-head">
          <span class="title">最近 {{ textMessages.length }} 条 · 倒序展示</span>
        </div>

        <p v-if="textMessages.length === 0" class="empty">暂无消息，发送第一条吧</p>

        <ul class="msg-list">
          <li v-for="m in textMessages" :key="m.id" class="msg-row">
            <span class="msg-time">{{ formatTime(m.createdAt) }}</span>
            <pre class="msg-body">{{ m.content }}</pre>
            <div class="msg-actions">
              <button class="act-btn copy" @click="copyMessage(m)">复制</button>
              <button class="act-btn del" @click="confirmDelete(m)">删除</button>
            </div>
          </li>
        </ul>

        <div v-if="textHasMore" class="more">
          <button class="ghost" @click="loadMoreText" :disabled="textLoadingMore">
            {{ textLoadingMore ? '加载中…' : '↓ 加载更多' }}
          </button>
        </div>
      </template>

      <!-- ==================== 文件标签页 ==================== -->
      <template v-if="activeTab === 'file'">
        <!-- 上传区 -->
        <section class="upload-card">
          <input ref="fileInputRef" type="file" style="display:none" @change="onFileSelected" />
          <div v-if="!uploading" class="upload-area" @click="triggerFileInput" @dragover.prevent @drop.prevent="onFileSelected">
            <div class="upload-hint">
              <span class="upload-icon">📎</span>
              <span>点击或拖拽文件到此处上传</span>
            </div>
          </div>
          <div v-else class="upload-progress">
            <div class="upload-file-name">{{ uploadFileName }}</div>
            <div class="progress-bar">
              <div class="progress-fill" :style="{ width: uploadProgress + '%' }"></div>
            </div>
            <div class="progress-text">{{ uploadProgress }}%</div>
          </div>
          <p v-if="uploadError" class="err">{{ uploadError }}</p>
        </section>

        <!-- 文件列表头 -->
        <div class="list-head">
          <span class="title">共 {{ fileMessages.length }} 个文件</span>
        </div>

        <p v-if="fileMessages.length === 0" class="empty">暂无文件，上传第一个吧</p>

        <ul class="file-list">
          <li v-for="m in fileMessages" :key="m.id" class="file-card">
            <span class="file-icon">{{ fileIcon(m.fileName || '') }}</span>
            <div class="file-info">
              <div class="file-name">{{ m.fileName || '未知文件' }}</div>
              <div class="file-meta">
                <span>{{ formatSize(m.fileSize || 0) }}</span>
                <span class="file-time">{{ formatTime(m.createdAt) }}</span>
              </div>
            </div>
            <div class="file-actions">
              <button class="act-btn copy" @click="downloadFile(m)">下载</button>
              <button class="act-btn del" @click="confirmDelete(m)">删除</button>
            </div>
          </li>
        </ul>

        <div v-if="fileHasMore" class="more">
          <button class="ghost" @click="loadMoreFile" :disabled="fileLoadingMore">
            {{ fileLoadingMore ? '加载中…' : '↓ 加载更多' }}
          </button>
        </div>
      </template>
    </template>

    <!-- 删除确认弹窗 -->
    <div v-if="pendingDelete" class="modal-mask" @click.self="cancelDelete">
      <div class="modal">
        <h3>确认删除</h3>
        <p>删除后无法恢复，确定要删除{{ pendingDelete.contentType === 'file' ? '该文件' : '这条消息' }}吗？</p>
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

/* 标签页 */
.tabs {
  display: flex; align-items: center; gap: 0; margin-bottom: 16px;
  border-bottom: 2px solid #e5e7eb;
}
.tab {
  padding: 8px 20px; font-size: 14px; font-weight: 500;
  color: #6b7280; background: none; border: none;
  border-bottom: 2px solid transparent; margin-bottom: -2px;
  cursor: pointer; transition: all .15s;
}
.tab:hover { color: #374151; }
.tab.active { color: #2563eb; border-bottom-color: #2563eb; }
.tabs .spacer { flex: 1; }

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

/* 上传区 */
.upload-card {
  background: white; border: 1px solid #e5e7eb; border-radius: 10px;
  padding: 12px; box-shadow: 0 1px 2px rgba(0,0,0,0.04); margin-bottom: 16px;
}
.upload-area {
  border: 2px dashed #d1d5db; border-radius: 8px; padding: 28px 16px;
  text-align: center; cursor: pointer; transition: all .15s;
}
.upload-area:hover { border-color: #2563eb; background: #f8faff; }
.upload-hint { display: flex; flex-direction: column; align-items: center; gap: 8px; color: #6b7280; font-size: 14px; }
.upload-icon { font-size: 28px; }
.upload-progress { padding: 8px 4px; }
.upload-file-name { font-size: 14px; font-weight: 500; color: #111827; margin-bottom: 8px; }
.progress-bar { height: 8px; background: #e5e7eb; border-radius: 4px; overflow: hidden; }
.progress-fill { height: 100%; background: #2563eb; border-radius: 4px; transition: width .2s; }
.progress-text { font-size: 12px; color: #6b7280; margin-top: 4px; text-align: right; }

/* 列表头 */
.list-head { display: flex; align-items: center; gap: 12px; margin: 0 4px 8px; }
.list-head .title { color: #374151; font-size: 14px; font-weight: 500; }
.list-head button { margin-left: auto; }

.empty { color: #9ca3af; text-align: center; padding: 32px 0; }

/* 文本消息列表 */
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

/* 文件列表 */
.file-list { list-style: none; padding: 0; margin: 0; }
.file-card {
  display: flex; align-items: center; gap: 12px;
  padding: 10px 12px; border-bottom: 1px solid #f3f4f6;
  transition: background .1s;
}
.file-card:hover { background: #f9fafb; }
.file-card:last-child { border-bottom: none; }
.file-icon {
  font-size: 28px; flex-shrink: 0; width: 36px; text-align: center;
}
.file-info { flex: 1; min-width: 0; }
.file-name {
  font-size: 14px; font-weight: 500; color: #111827;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.file-meta { display: flex; gap: 12px; font-size: 12px; color: #9ca3af; margin-top: 2px; }
.file-time { font-family: ui-monospace, monospace; }
.file-actions {
  flex-shrink: 0; display: flex; gap: 6px;
  opacity: 0; transition: opacity .15s;
}
.file-card:hover .file-actions { opacity: 1; }

/* 操作按钮 */
.act-btn {
  padding: 3px 10px; border-radius: 5px; cursor: pointer;
  font-size: 12px; line-height: 1.4; border: 1px solid;
  background: white; transition: all .12s;
}
.act-btn.copy { color: #2563eb; border-color: #bfdbfe; }
.act-btn.copy:hover { background: #eff6ff; border-color: #93c5fd; }
.act-btn.del { color: #dc2626; border-color: #fecaca; }
.act-btn.del:hover { background: #fef2f2; border-color: #fca5a5; }

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
