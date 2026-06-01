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
    <!-- 顶部品牌栏 -->
    <header class="topbar">
      <div class="brand">
        <div class="logo-mark" aria-hidden="true">
          <span class="logo-bar"></span>
          <span class="logo-bar"></span>
          <span class="logo-bar"></span>
        </div>
        <div class="brand-text">
          <h1>WEBCLIP<span class="brand-dot">.</span></h1>
          <p class="sub">// 6 位短码 · 跨设备共享 · 多端实时同步</p>
        </div>
      </div>
      <div class="actions">
        <button class="btn ghost" @click="openJoin">加入房间</button>
        <button class="btn primary" @click="openCreate">＋ 创建房间</button>
      </div>
    </header>

    <!-- 房间清单 -->
    <section class="rooms">
      <div class="rooms-head">
        <h2><span class="hash">#</span> 所有房间</h2>
        <span class="meta">
          <span class="tally pub">公开 {{ publicCount }}</span>
          <span class="tally priv">私有 {{ privateCount }}</span>
        </span>
        <button class="btn outline refresh" @click="loadList" :disabled="listLoading">
          {{ listLoading ? '加载中…' : '↻ 刷新' }}
        </button>
      </div>

      <p v-if="listError" class="err banner">{{ listError }}</p>
      <p v-else-if="!listLoading && rooms.length === 0" class="empty">
        <span class="empty-mark">∅</span>
        暂无房间 — 点击右上角「创建房间」开始
      </p>

      <ul v-else class="room-list">
        <li
          v-for="(r, i) in rooms"
          :key="r.code"
          class="room-item"
          :style="{ animationDelay: Math.min(i, 12) * 40 + 'ms' }"
          @click="enterRoom(r)"
        >
          <div class="ri-left">
            <span class="ri-code">{{ r.code }}</span>
            <span class="badge" :class="r.hasPassword ? 'priv' : 'pub'">
              {{ r.hasPassword ? '🔒 私有' : '🌐 公开' }}
            </span>
          </div>
          <div class="ri-body">
            <span class="ri-name">{{ r.name || '未命名' }}</span>
            <span class="ri-meta">更新于 {{ formatTime(r.updatedAt) }}</span>
          </div>
          <div class="ri-actions" @click.stop>
            <button class="btn outline" @click="enterRoom(r)">进入 →</button>
            <button class="btn outline" @click="openEdit(r)">修改</button>
            <button class="btn outline danger" @click="openDelete(r)">删除</button>
          </div>
        </li>
      </ul>
    </section>

    <!-- 创建对话框 -->
    <div v-if="createState.open" class="modal-mask" @click.self="closeCreate">
      <div class="modal">
        <h3><span class="m-tag">NEW</span> 创建新房间</h3>
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
        <h3><span class="m-tag">JOIN</span> 加入已有房间</h3>
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
        <h3><span class="m-tag">EDIT</span> 修改房间 <span class="code-tag">{{ edit.room?.code }}</span></h3>
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
        <h3><span class="m-tag danger">DEL</span> 删除房间</h3>
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
.home {
  max-width: 880px;
  margin: 0 auto;
  padding: 56px 20px 80px;
}

/* ===== 顶部品牌栏 ===== */
.topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  margin-bottom: 40px;
  flex-wrap: wrap;
  animation: wc-rise 0.5s both;
}
.brand { display: flex; align-items: center; gap: 14px; }
.logo-mark {
  display: flex; flex-direction: column; gap: 3px;
  padding: 10px;
  width: 44px; height: 44px;
  align-items: center; justify-content: center;
  background: linear-gradient(135deg, var(--primary), #7c6cf0);
  border-radius: var(--radius);
  box-shadow: var(--shadow-primary);
}
.logo-bar { display: block; height: 3px; border-radius: 2px; background: rgba(255, 255, 255, 0.95); }
.logo-bar:nth-child(1) { width: 20px; }
.logo-bar:nth-child(2) { width: 12px; background: rgba(255, 255, 255, 0.7); }
.logo-bar:nth-child(3) { width: 16px; }
.brand-text h1 {
  margin: 0;
  font-family: var(--font-sans);
  font-size: 26px;
  font-weight: 700;
  letter-spacing: -0.6px;
  line-height: 1.1;
}
.brand-dot { color: var(--primary); }
.brand .sub {
  margin: 3px 0 0;
  color: var(--text-faint);
  font-size: 13px;
  letter-spacing: 0;
}
.actions { display: flex; gap: 10px; }

/* ===== 通用按钮 ===== */
.btn {
  font-family: var(--font-sans);
  padding: 9px 16px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--surface);
  color: var(--text);
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
  box-shadow: var(--shadow-xs);
  transition: background .15s, border-color .15s, box-shadow .15s, transform .05s, color .15s;
}
.btn:hover:not(:disabled) { background: var(--surface-2); border-color: var(--border-strong); }
.btn:active:not(:disabled) { transform: translateY(1px); }
.btn:disabled { opacity: 0.5; cursor: not-allowed; }
.btn.primary {
  background: var(--primary); color: #fff; border-color: transparent;
  box-shadow: var(--shadow-sm);
}
.btn.primary:hover:not(:disabled) { background: var(--primary-hover); }
.btn.ghost { background: var(--surface); }
.btn.outline {
  padding: 6px 12px; font-size: 13px;
}
.btn.outline.danger { color: var(--danger); border-color: var(--border); }
.btn.outline.danger:hover:not(:disabled) { background: var(--danger-soft); border-color: var(--danger); color: var(--danger); }
.btn.danger-solid { background: var(--danger); color: #fff; border-color: transparent; }
.btn.danger-solid:hover:not(:disabled) { background: var(--danger-hover); }

/* ===== 房间清单 ===== */
.rooms { animation: wc-rise 0.5s both 0.08s; }
.rooms-head {
  display: flex; align-items: center; gap: 12px;
  margin-bottom: 18px; flex-wrap: wrap;
}
.rooms-head h2 {
  margin: 0;
  font-family: var(--font-sans);
  font-size: 17px; font-weight: 650;
}
.rooms-head .hash { color: var(--text-faint); font-weight: 500; }
.rooms-head .meta { display: flex; gap: 8px; }
.tally {
  font-size: 12px; font-weight: 600; padding: 3px 10px;
  border: 1px solid var(--border); border-radius: var(--radius-full);
  color: var(--text-soft); background: var(--surface);
}
.tally.pub { background: var(--success-soft); color: var(--success); border-color: transparent; }
.tally.priv { background: var(--primary-soft); color: var(--primary-hover); border-color: transparent; }
.rooms-head .refresh { margin-left: auto; }

.banner {
  border: 1px solid var(--danger);
  color: var(--danger-hover);
  background: var(--danger-soft);
  padding: 12px 14px; border-radius: var(--radius-sm);
  font-size: 14px;
}
.empty {
  display: flex; flex-direction: column; align-items: center; gap: 12px;
  color: var(--text-soft); text-align: center; padding: 60px 0;
  border: 1px dashed var(--border-strong); border-radius: var(--radius);
  background: var(--surface);
  font-size: 14px;
}
.empty-mark {
  font-size: 28px; color: var(--text-faint);
  width: 56px; height: 56px; line-height: 56px;
  border-radius: var(--radius-full);
  background: var(--surface-2);
}

.room-list { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 10px; }
.room-item {
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: 8px 18px;
  padding: 16px 18px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  box-shadow: var(--shadow-xs);
  cursor: pointer;
  transition: border-color .15s, box-shadow .15s, transform .1s;
  animation: wc-rise 0.45s both;
}
.room-item:hover {
  transform: translateY(-2px);
  border-color: var(--border-strong);
  box-shadow: var(--shadow-md);
}
.ri-left { display: flex; flex-direction: column; gap: 7px; align-items: flex-start; }
.ri-code {
  font-family: var(--font-mono); font-weight: 600; font-size: 14px;
  letter-spacing: 2px;
  background: var(--surface-2); color: var(--text);
  padding: 4px 10px; border-radius: var(--radius-sm);
  border: 1px solid var(--border);
}
.ri-body { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
.ri-name {
  font-family: var(--font-sans); font-weight: 600; font-size: 15px;
  color: var(--text);
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.ri-meta { color: var(--text-faint); font-size: 12px; }
.ri-actions { display: flex; gap: 6px; }
.badge {
  font-size: 12px; font-weight: 600; padding: 3px 9px;
  border-radius: var(--radius-full); border: 1px solid transparent;
  white-space: nowrap;
  display: inline-flex; align-items: center; gap: 4px;
}
.badge.pub { background: var(--success-soft); color: var(--success); }
.badge.priv { background: var(--primary-soft); color: var(--primary-hover); }

.err { color: var(--danger); margin: 8px 0; font-size: 13px; font-weight: 500; }

/* ===== 模态框 ===== */
.modal-mask {
  position: fixed; inset: 0; z-index: 50; padding: 16px;
  background: rgba(15, 23, 42, 0.45);
  backdrop-filter: blur(2px);
  display: flex; align-items: center; justify-content: center;
  animation: wc-fade 0.18s both;
}
.modal {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 26px;
  width: 100%; max-width: 440px;
  box-shadow: var(--shadow-lg);
  animation: wc-pop 0.22s both;
}
.modal h3 {
  margin: 0 0 20px;
  font-family: var(--font-sans);
  font-size: 18px; font-weight: 650;
  display: flex; align-items: center; gap: 10px;
}
.m-tag {
  font-family: var(--font-mono); font-size: 11px; font-weight: 600;
  letter-spacing: 0.5px;
  background: var(--primary-soft); color: var(--primary-hover);
  padding: 3px 9px; border-radius: var(--radius-sm);
}
.m-tag.danger { background: var(--danger-soft); color: var(--danger); }
.modal label { display: block; margin-bottom: 16px; }
.modal label span {
  display: block; margin-bottom: 7px;
  color: var(--text-soft); font-size: 13px; font-weight: 500;
}
.modal input[type=text], .modal input:not([type]), .modal input[type=password] {
  width: 100%; padding: 10px 12px;
  border: 1px solid var(--border-strong); border-radius: var(--radius-sm);
  background: var(--surface);
  font-family: var(--font-sans); font-size: 14px; color: var(--text);
  outline: none; box-sizing: border-box;
  transition: border-color .15s, box-shadow .15s;
}
.modal input:focus { border-color: var(--primary); box-shadow: 0 0 0 3px var(--primary-ring); }
.modal input.code-input {
  text-transform: uppercase; letter-spacing: 6px;
  font-family: var(--font-mono);
  font-weight: 600; text-align: center; font-size: 18px;
}
.modal label.check { display: flex; align-items: center; gap: 8px; cursor: pointer; }
.modal label.check span { margin: 0; }
.modal label.check input { width: auto; accent-color: var(--primary); width: 16px; height: 16px; }
.modal .warn { color: var(--text-soft); font-size: 14px; line-height: 1.65; margin: 0 0 16px; }
.modal-actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 8px; }
.code-tag {
  font-family: var(--font-mono); font-weight: 600;
  background: var(--surface-2); color: var(--text);
  border: 1px solid var(--border);
  padding: 2px 8px; border-radius: var(--radius-sm); letter-spacing: 1px; font-size: 13px;
}

@media (max-width: 560px) {
  .topbar { align-items: flex-start; }
  .actions { width: 100%; }
  .actions .btn { flex: 1; }
  .room-item { grid-template-columns: 1fr; gap: 12px; }
  .ri-actions { width: 100%; }
  .ri-actions .btn { flex: 1; }
}
</style>
