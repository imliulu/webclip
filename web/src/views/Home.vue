<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import {
  createClip,
  listRooms,
  authRoom,
  patchRoom,
  deleteRoom,
  type RoomItem,
} from '../api/clip'
import { useRoomStore } from '../stores/room'

const router = useRouter()
const store = useRoomStore()

// ---- 列表 ----
const rooms = ref<RoomItem[]>([])
const listLoading = ref(false)
const listError = ref('')

const publicCount = computed(() => rooms.value.filter(r => !r.hasPassword).length)
const privateCount = computed(() => rooms.value.filter(r => r.hasPassword).length)

// ---- 创建对话框 ----
type CreateState = {
  open: boolean
  name: string
  password: string
  busy: boolean
  error: string
}
const createState = ref<CreateState>(emptyCreate())
function emptyCreate(): CreateState {
  return { open: false, name: '', password: '', busy: false, error: '' }
}

// ---- 加入对话框 ----
type JoinState = {
  open: boolean
  code: string
  error: string
}
const joinState = ref<JoinState>(emptyJoin())
function emptyJoin(): JoinState {
  return { open: false, code: '', error: '' }
}

// ---- 编辑对话框 ----
type EditState = {
  open: boolean
  room: RoomItem | null
  name: string
  password: string
  changePassword: boolean
  authPassword: string
  busy: boolean
  error: string
}
const edit = ref<EditState>(emptyEdit())
function emptyEdit(): EditState {
  return {
    open: false, room: null, name: '', password: '',
    changePassword: false, authPassword: '', busy: false, error: '',
  }
}

// ---- 删除对话框 ----
type DelState = {
  open: boolean
  room: RoomItem | null
  authPassword: string
  busy: boolean
  error: string
}
const del = ref<DelState>(emptyDel())
function emptyDel(): DelState {
  return { open: false, room: null, authPassword: '', busy: false, error: '' }
}

async function loadList() {
  listLoading.value = true
  listError.value = ''
  try {
    rooms.value = await listRooms()
  } catch (e: any) {
    listError.value = e?.message || '加载失败'
  } finally {
    listLoading.value = false
  }
}

function enterRoom(r: RoomItem) {
  router.push(`/c/${r.code}`)
}

function formatTime(s?: string) {
  if (!s) return ''
  return s.replace('T', ' ').replace(/\..+$/, '').slice(0, 16)
}

// ---- 创建 ----
function openCreate() { createState.value = emptyCreate(); createState.value.open = true }
function closeCreate() { createState.value = emptyCreate() }

async function submitCreate() {
  const c = createState.value
  c.busy = true
  c.error = ''
  try {
    const r = await createClip(c.name.trim(), c.password)
    store.setToken(r.code, r.token)
    router.push(`/c/${r.code}`)
  } catch (e: any) {
    c.error = e?.message || '创建失败'
  } finally {
    c.busy = false
  }
}

// ---- 加入 ----
function openJoin() { joinState.value = emptyJoin(); joinState.value.open = true }
function closeJoin() { joinState.value = emptyJoin() }
function submitJoin() {
  const code = joinState.value.code.trim().toUpperCase()
  if (!code) {
    joinState.value.error = '请输入 6 位短码'
    return
  }
  closeJoin()
  router.push(`/c/${code}`)
}

// ---- 修改 ----
function openEdit(r: RoomItem) {
  edit.value = {
    open: true, room: r, name: r.name || '',
    password: '', changePassword: false, authPassword: '',
    busy: false, error: '',
  }
}
function closeEdit() { edit.value = emptyEdit() }

async function submitEdit() {
  const e = edit.value
  if (!e.room) return
  e.busy = true
  e.error = ''
  try {
    let token = store.getToken(e.room.code)
    if (!token) {
      const pwd = e.room.hasPassword ? e.authPassword : ''
      const r = await authRoom(e.room.code, pwd)
      token = r.token
      store.setToken(e.room.code, token)
    }
    const payload: { name?: string; password?: string } = {}
    payload.name = e.name.trim()
    if (e.changePassword) payload.password = e.password
    await patchRoom(e.room.code, payload)
    closeEdit()
    await loadList()
  } catch (err: any) {
    if (err?.response?.status === 401) {
      store.clearToken(e.room.code)
      e.error = e.room.hasPassword ? '密码错误' : '认证失败'
    } else {
      e.error = err?.message || '保存失败'
    }
  } finally {
    e.busy = false
  }
}

// ---- 删除 ----
function openDelete(r: RoomItem) {
  del.value = { open: true, room: r, authPassword: '', busy: false, error: '' }
}
function closeDelete() { del.value = emptyDel() }

async function confirmDelete() {
  const d = del.value
  if (!d.room) return
  d.busy = true
  d.error = ''
  try {
    let token = store.getToken(d.room.code)
    if (!token) {
      const pwd = d.room.hasPassword ? d.authPassword : ''
      const r = await authRoom(d.room.code, pwd)
      token = r.token
      store.setToken(d.room.code, token)
    }
    await deleteRoom(d.room.code)
    store.clearToken(d.room.code)
    closeDelete()
    await loadList()
  } catch (err: any) {
    if (err?.response?.status === 401) {
      store.clearToken(d.room.code)
      d.error = d.room.hasPassword ? '密码错误' : '认证失败'
    } else {
      d.error = err?.message || '删除失败'
    }
  } finally {
    d.busy = false
  }
}

onMounted(loadList)
</script>

<template>
  <div class="home">
    <header class="topbar">
      <div class="brand">
        <h1>WebClip</h1>
        <p class="sub">通过 6 位短码跨设备共享剪贴内容，支持多端实时同步</p>
      </div>
      <div class="actions">
        <button class="btn ghost" @click="openJoin">加入房间</button>
        <button class="btn primary" @click="openCreate">+ 创建房间</button>
      </div>
    </header>

    <section class="rooms">
      <div class="rooms-head">
        <h2>所有房间</h2>
        <span class="meta">公开 {{ publicCount }} · 私有 {{ privateCount }}</span>
        <button class="btn outline refresh" @click="loadList" :disabled="listLoading">
          {{ listLoading ? '加载中…' : '刷新' }}
        </button>
      </div>
      <p v-if="listError" class="err">{{ listError }}</p>
      <p v-else-if="!listLoading && rooms.length === 0" class="empty">
        暂无房间，点击右上角"创建房间"开始
      </p>
      <ul v-else class="room-list">
        <li v-for="r in rooms" :key="r.code" class="room-item">
          <div class="ri-main">
            <span class="ri-code">{{ r.code }}</span>
            <span class="ri-name">{{ r.name || '未命名' }}</span>
            <span class="badge" :class="r.hasPassword ? 'priv' : 'pub'">
              {{ r.hasPassword ? '🔒 私有' : '🌐 公开' }}
            </span>
          </div>
          <div class="ri-meta">更新于 {{ formatTime(r.updatedAt) }}</div>
          <div class="ri-actions">
            <button class="btn outline" @click="enterRoom(r)">进入</button>
            <button class="btn outline" @click="openEdit(r)">修改</button>
            <button class="btn outline danger" @click="openDelete(r)">删除</button>
          </div>
        </li>
      </ul>
    </section>

    <!-- 创建对话框 -->
    <div v-if="createState.open" class="modal-mask" @click.self="closeCreate">
      <div class="modal">
        <h3>创建新房间</h3>
        <label>
          <span>房间名称（可留空）</span>
          <input v-model="createState.name" maxlength="30" placeholder="给房间起个名字，便于识别"
            @keyup.enter="submitCreate" />
        </label>
        <label>
          <span>房间密码（可留空）</span>
          <input v-model="createState.password" type="password"
            placeholder="不填则任何人可直接进入"
            @keyup.enter="submitCreate" />
        </label>
        <p v-if="createState.error" class="err">{{ createState.error }}</p>
        <div class="modal-actions">
          <button class="btn ghost" @click="closeCreate" :disabled="createState.busy">取消</button>
          <button class="btn primary" @click="submitCreate" :disabled="createState.busy">
            {{ createState.busy ? '创建中…' : '创建并进入' }}
          </button>
        </div>
      </div>
    </div>

    <!-- 加入对话框 -->
    <div v-if="joinState.open" class="modal-mask" @click.self="closeJoin">
      <div class="modal">
        <h3>加入已有房间</h3>
        <label>
          <span>6 位短码</span>
          <input
            v-model="joinState.code"
            placeholder="例如 ABC123"
            maxlength="6"
            class="code-input"
            @keyup.enter="submitJoin"
          />
        </label>
        <p v-if="joinState.error" class="err">{{ joinState.error }}</p>
        <div class="modal-actions">
          <button class="btn ghost" @click="closeJoin">取消</button>
          <button class="btn primary" @click="submitJoin">进入</button>
        </div>
      </div>
    </div>

    <!-- 编辑对话框 -->
    <div v-if="edit.open" class="modal-mask" @click.self="closeEdit">
      <div class="modal">
        <h3>修改房间 <span class="code-tag">{{ edit.room?.code }}</span></h3>
        <label>
          <span>房间名称</span>
          <input v-model="edit.name" maxlength="30" placeholder="房间名称" />
        </label>
        <label v-if="edit.room?.hasPassword">
          <span>原密码（验证身份）</span>
          <input v-model="edit.authPassword" type="password" placeholder="请输入当前房间密码" />
        </label>
        <label class="check">
          <input type="checkbox" v-model="edit.changePassword" />
          <span>修改密码</span>
        </label>
        <label v-if="edit.changePassword">
          <span>新密码（留空表示取消密码保护）</span>
          <input v-model="edit.password" type="password" placeholder="留空则改为公开房间" />
        </label>
        <p v-if="edit.error" class="err">{{ edit.error }}</p>
        <div class="modal-actions">
          <button class="btn ghost" @click="closeEdit" :disabled="edit.busy">取消</button>
          <button class="btn primary" @click="submitEdit" :disabled="edit.busy">
            {{ edit.busy ? '保存中…' : '保存' }}
          </button>
        </div>
      </div>
    </div>

    <!-- 删除对话框（二次确认） -->
    <div v-if="del.open" class="modal-mask" @click.self="closeDelete">
      <div class="modal">
        <h3>删除房间</h3>
        <p class="warn">
          确定要删除房间
          <span class="code-tag">{{ del.room?.code }}</span>
          <strong v-if="del.room?.name"> ({{ del.room?.name }})</strong>
          吗？此操作不可撤销，房间内所有内容将被永久删除。
        </p>
        <label v-if="del.room?.hasPassword">
          <span>请输入房间密码以确认删除</span>
          <input v-model="del.authPassword" type="password" placeholder="房间密码" />
        </label>
        <p v-if="del.error" class="err">{{ del.error }}</p>
        <div class="modal-actions">
          <button class="btn ghost" @click="closeDelete" :disabled="del.busy">取消</button>
          <button class="btn danger-solid" @click="confirmDelete" :disabled="del.busy">
            {{ del.busy ? '删除中…' : '确认删除' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.home { max-width: 960px; margin: 32px auto; padding: 0 16px; }

/* 顶部栏 */
.topbar {
  display: flex; align-items: center; justify-content: space-between;
  gap: 16px; margin-bottom: 24px; flex-wrap: wrap;
}
.brand h1 { margin: 0 0 4px; font-size: 26px; font-weight: 600; }
.brand .sub { margin: 0; color: #6b7280; font-size: 13px; }
.actions { display: flex; gap: 8px; }

/* 通用按钮 */
.btn { padding: 8px 16px; border-radius: 6px; cursor: pointer; font-size: 14px; font-weight: 500; border: 1px solid transparent; transition: background .15s, border-color .15s, color .15s; }
.btn:disabled { opacity: 0.6; cursor: not-allowed; }
.btn.primary { background: #2563eb; color: white; }
.btn.primary:hover:not(:disabled) { background: #1d4ed8; }
.btn.ghost { background: white; color: #374151; border-color: #d1d5db; }
.btn.ghost:hover:not(:disabled) { background: #f3f4f6; }
.btn.outline { background: white; color: #111; border-color: #d1d5db; padding: 6px 12px; font-size: 13px; font-weight: 400; }
.btn.outline:hover:not(:disabled) { background: #f3f4f6; }
.btn.outline.danger { color: #dc2626; border-color: #fecaca; }
.btn.outline.danger:hover:not(:disabled) { background: #fef2f2; }
.btn.danger-solid { background: #dc2626; color: white; }
.btn.danger-solid:hover:not(:disabled) { background: #b91c1c; }

/* 房间列表 */
.rooms { background: white; border: 1px solid #e5e7eb; border-radius: 10px; padding: 24px; box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04); }
.rooms-head { display: flex; align-items: center; gap: 12px; margin-bottom: 12px; }
.rooms-head h2 { margin: 0; font-size: 18px; font-weight: 500; }
.rooms-head .meta { color: #6b7280; font-size: 13px; }
.rooms-head .refresh { margin-left: auto; }
.empty { color: #6b7280; text-align: center; padding: 32px 0; }
.room-list { list-style: none; padding: 0; margin: 0; }
.room-item { display: grid; grid-template-columns: 1fr auto; grid-template-rows: auto auto; gap: 4px 12px; padding: 14px 0; border-top: 1px solid #f3f4f6; align-items: center; }
.room-item:first-child { border-top: none; }
.ri-main { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.ri-code { font-family: ui-monospace, monospace; background: #f3f4f6; padding: 3px 10px; border-radius: 4px; letter-spacing: 2px; font-weight: 600; font-size: 13px; }
.ri-name { font-weight: 500; color: #111827; }
.ri-meta { grid-column: 1; color: #9ca3af; font-size: 12px; }
.ri-actions { grid-column: 2; grid-row: 1 / span 2; display: flex; gap: 6px; }
.badge { font-size: 12px; padding: 2px 8px; border-radius: 999px; }
.badge.pub { background: #ecfdf5; color: #059669; border: 1px solid #a7f3d0; }
.badge.priv { background: #fff7ed; color: #c2410c; border: 1px solid #fed7aa; }

.err { color: #dc2626; margin: 8px 0; font-size: 13px; }

/* 模态框 */
.modal-mask {
  position: fixed; inset: 0; background: rgba(17, 24, 39, 0.5);
  display: flex; align-items: center; justify-content: center; z-index: 50; padding: 16px;
}
.modal { background: white; border-radius: 10px; padding: 24px; width: 100%; max-width: 420px; box-shadow: 0 10px 30px rgba(0,0,0,0.2); }
.modal h3 { margin: 0 0 16px; font-size: 16px; font-weight: 600; }
.modal label { display: block; margin-bottom: 12px; }
.modal label span { display: block; margin-bottom: 6px; color: #374151; font-size: 13px; }
.modal input[type=text], .modal input:not([type]), .modal input[type=password] {
  width: 100%; padding: 10px 12px; border: 1px solid #d1d5db; border-radius: 6px; font-size: 14px; outline: none; box-sizing: border-box;
}
.modal input:focus { border-color: #2563eb; box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.15); }
.modal input.code-input { text-transform: uppercase; letter-spacing: 4px; font-family: ui-monospace, monospace; }
.modal label.check { display: flex; align-items: center; gap: 8px; }
.modal label.check span { margin: 0; }
.modal label.check input { width: auto; }
.modal .warn { color: #374151; font-size: 14px; line-height: 1.6; margin: 0 0 12px; }
.modal-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 8px; }
.code-tag { font-family: ui-monospace, monospace; background: #f3f4f6; padding: 2px 8px; border-radius: 4px; letter-spacing: 2px; font-weight: 600; font-size: 13px; }

@media (max-width: 520px) {
  .topbar { align-items: flex-start; }
  .actions { width: 100%; }
  .actions .btn { flex: 1; }
  .room-item { grid-template-columns: 1fr; }
  .ri-actions { grid-column: 1; grid-row: auto; }
}
</style>
