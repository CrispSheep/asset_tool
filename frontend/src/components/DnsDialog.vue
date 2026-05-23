<script setup lang="ts">
import { ref, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'
import {
  RunDns, PauseJob, ResumeJob, CancelJob,
} from '../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'

const props = defineProps<{ projectId: number }>()
const emit = defineEmits<{ resolved: [] }>()
const visible = defineModel<boolean>('visible', { default: false })

const concurrency = ref(20)
const timeout = ref(5)
const dnsServer = ref('')

const running = ref(false)
const paused = ref(false)
const total = ref(0)
const processed = ref(0)
const resolvedCount = ref(0)
const failedCount = ref(0)
const newIPCount = ref(0)
const log = ref<string[]>([])
const logEl = ref<HTMLElement | null>(null)
const jobId = ref('')

const startedAt = ref(0)
const elapsed = ref('00:00')
let timerHandle: number | null = null

function pad(n: number) { return n.toString().padStart(2, '0') }
function fmt(ms: number) {
  const s = Math.floor(ms / 1000)
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  return h > 0 ? `${pad(h)}:${pad(m)}:${pad(sec)}` : `${pad(m)}:${pad(sec)}`
}

function appendLog(line: string) {
  log.value.push(line)
  if (log.value.length > 1000) log.value.splice(0, log.value.length - 1000)
  setTimeout(() => {
    if (logEl.value) logEl.value.scrollTop = logEl.value.scrollHeight
  }, 10)
}

EventsOn('dns:start', (data: any) => {
  total.value = data.total
  appendLog(`[i] 共 ${data.total} 个域名待解析`)
})
EventsOn('dns:progress', (data: any) => {
  processed.value = data.processed
  resolvedCount.value = data.resolved
  failedCount.value = data.failed
  newIPCount.value = data.new_ip
  if (data.success) {
    const ips = (data.ips as string[]).join(', ')
    appendLog(`[✓] ${data.domain} → ${ips}`)
  } else {
    appendLog(`[✗] ${data.domain} → ${data.error}`)
  }
})
EventsOn('dns:done', (data: any) => {
  running.value = false
  paused.value = false
  if (timerHandle) { clearInterval(timerHandle); timerHandle = null }
  appendLog(`\n完成：${data.total} 个域名，成功 ${data.resolved}，失败 ${data.failed}，新增 IP ${data.new_ip}，用时 ${elapsed.value}`)
  if (data.cancelled) appendLog('[i] 任务已被取消')
  emit('resolved')
})
EventsOn('dns:error', (msg: string) => {
  running.value = false
  if (timerHandle) { clearInterval(timerHandle); timerHandle = null }
  ElMessage.error(msg)
  appendLog(`[!] ${msg}`)
})

onBeforeUnmount(() => {
  EventsOff('dns:start')
  EventsOff('dns:progress')
  EventsOff('dns:done')
  EventsOff('dns:error')
  if (timerHandle) clearInterval(timerHandle)
})

async function start() {
  log.value = []
  processed.value = 0
  resolvedCount.value = 0
  failedCount.value = 0
  newIPCount.value = 0
  total.value = 0
  paused.value = false
  running.value = true
  startedAt.value = Date.now()
  elapsed.value = '00:00'
  if (timerHandle) clearInterval(timerHandle)
  timerHandle = window.setInterval(() => {
    elapsed.value = fmt(Date.now() - startedAt.value)
  }, 1000)

  try {
    jobId.value = await RunDns(props.projectId, {
      concurrency: concurrency.value,
      timeout: timeout.value,
      dns_server: dnsServer.value.trim(),
    })
  } catch (e: any) {
    running.value = false
    if (timerHandle) { clearInterval(timerHandle); timerHandle = null }
    ElMessage.error('启动失败: ' + e)
  }
}

function togglePause() {
  if (paused.value) {
    ResumeJob(jobId.value)
    paused.value = false
    appendLog('[i] 已继续')
  } else {
    PauseJob(jobId.value)
    paused.value = true
    appendLog('[i] 已暂停')
  }
}

function stop() {
  if (jobId.value) CancelJob(jobId.value)
}

function close() {
  if (running.value) {
    ElMessage.warning('解析进行中，可点「收起」后台运行，或先停止')
    return
  }
  visible.value = false
}

function hide() {
  visible.value = false
}
</script>

<template>
  <el-dialog
    v-model="visible"
    title="DNS 批量解析"
    width="65%"
    top="5vh"
    draggable
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    :show-close="!running"
    class="dns-dialog"
  >
    <el-alert
      type="info"
      :closable="false"
      show-icon
      style="margin-bottom: 14px"
    >
      <template #title>
        批量解析项目中所有域名的 A 记录，解析成功的 IP 自动加入资产列表（source=dns），
        解析失败的域名标记 "DNS无效" 标签，方便后续过滤。
      </template>
    </el-alert>

    <el-form label-width="110px" size="default">
      <el-form-item label="并发数">
        <el-input-number v-model="concurrency" :min="1" :max="200" />
        <span class="form-hint">同时解析的域名数量</span>
      </el-form-item>
      <el-form-item label="超时(秒)">
        <el-input-number v-model="timeout" :min="1" :max="30" />
        <span class="form-hint">每个域名的解析超时</span>
      </el-form-item>
      <el-form-item label="DNS 服务器">
        <el-input v-model="dnsServer" placeholder="留空使用系统默认，如 8.8.8.8 或 114.114.114.114" />
      </el-form-item>
    </el-form>

    <!-- 进度 + 用时 -->
    <div v-if="running || total > 0" class="progress-area">
      <el-progress
        :percentage="total === 0 ? 0 : Math.floor((processed / total) * 100)"
        :status="!running ? 'success' : ''"
      />
      <div class="meta">
        <span class="time">⏱ {{ elapsed }}</span>
        <span class="muted">
          {{ processed }} / {{ total }} |
          <span style="color: var(--success)">成功 {{ resolvedCount }}</span> |
          <span style="color: var(--danger)">失败 {{ failedCount }}</span> |
          <span style="color: var(--accent)">新增 IP {{ newIPCount }}</span>
        </span>
      </div>
    </div>

    <div ref="logEl" class="log">
      <div v-for="(l, i) in log" :key="i" class="log-line"
        :class="{ 'log-success': l.startsWith('[✓]'), 'log-fail': l.startsWith('[✗]') }"
      >{{ l }}</div>
    </div>

    <template #footer>
      <el-button @click="close" :disabled="running">关闭</el-button>
      <el-button v-if="running" @click="hide">收起后台</el-button>
      <el-button v-if="running" @click="togglePause">
        {{ paused ? '▶ 继续' : '⏸ 暂停' }}
      </el-button>
      <el-button v-if="running" type="danger" @click="stop">停止</el-button>
      <el-button v-else type="primary" @click="start">开始解析</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.form-hint {
  margin-left: 12px;
  color: var(--text-muted);
  font-size: 12px;
}
.progress-area {
  margin-top: 12px;
  padding: 8px 12px;
  background: var(--bg-surface);
  border-radius: 6px;
}
.meta {
  display: flex;
  justify-content: space-between;
  margin-top: 4px;
  font-size: 12px;
}
.time { color: var(--accent); font-weight: 600; }
.muted { color: var(--text-muted); }
.log {
  margin-top: 10px;
  flex: 1;
  min-height: 120px;
  overflow-y: auto;
  background: var(--bg-log);
  color: var(--text-regular);
  font-family: Consolas, monospace;
  font-size: 12px;
  padding: 8px 10px;
  border-radius: 6px;
  border: 1px solid var(--border);
}
.log-line {
  white-space: pre-wrap;
  line-height: 1.4;
}
.log-success { color: var(--success); }
.log-fail { color: var(--danger); }
</style>

<style>
.dns-dialog {
  height: 90vh;
  display: flex;
  flex-direction: column;
  margin: 0 !important;
}
.dns-dialog .el-dialog__body {
  flex: 1;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  padding-top: 10px;
  padding-bottom: 10px;
}
</style>
