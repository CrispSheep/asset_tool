<script setup lang="ts">
import { ref, watch, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'
import { InfoFilled } from '@element-plus/icons-vue'
import {
  RunHttpx, PauseJob, ResumeJob, CancelJob,
  GetSetting, SetSetting,
} from '../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'

const props = defineProps<{ projectId: number }>()
const emit = defineEmits<{ probed: [] }>()
const visible = defineModel<boolean>('visible', { default: false })

const httpxPath = ref('')
const threads = ref(50)
const timeout = ref(5)
const retries = ref(1)
const rateLimit = ref(0)
const probeTitle = ref(true)
const probeTech = ref(false)
const probeServer = ref(true)
const probeCL = ref(false)
const probeIP = ref(false)
const probeCDN = ref(false)
const followRedir = ref(true)
const matchCodes = ref('')
const filterCodes = ref('')
const onlyUnprobed = ref(true)
const onlyIP = ref(false)
const onlyDomain = ref(false)
const skipDnsFailed = ref(true)

watch(onlyIP, v => { if (v) onlyDomain.value = false })
watch(onlyDomain, v => { if (v) onlyIP.value = false })

const running = ref(false)
const paused = ref(false)
const total = ref(0)
const processed = ref(0)
const alive = ref(0)
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
  if (log.value.length > 500) log.value.splice(0, log.value.length - 500)
  setTimeout(() => {
    if (logEl.value) logEl.value.scrollTop = logEl.value.scrollHeight
  }, 10)
}

async function loadSettings() {
  httpxPath.value = await GetSetting('httpx_path')
}
loadSettings()

EventsOn('httpx:start', (data: any) => {
  total.value = data.total
  appendLog(`[i] 共 ${data.total} 个目标待探活`)
})
EventsOn('httpx:progress', (data: any) => {
  processed.value = data.processed
  alive.value = data.alive
  const code = data.status_code != null ? data.status_code : '-'
  appendLog(`[${code}] ${data.host}`)
})
EventsOn('httpx:done', (data: any) => {
  running.value = false
  paused.value = false
  if (timerHandle) { clearInterval(timerHandle); timerHandle = null }
  appendLog(`\n完成：${data.total} 个目标，存活 ${data.alive} 个，用时 ${elapsed.value}`)
  emit('probed')
})
EventsOn('httpx:error', (msg: string) => {
  running.value = false
  if (timerHandle) { clearInterval(timerHandle); timerHandle = null }
  ElMessage.error(msg)
  appendLog(`[!] ${msg}`)
})

onBeforeUnmount(() => {
  EventsOff('httpx:start')
  EventsOff('httpx:progress')
  EventsOff('httpx:done')
  EventsOff('httpx:error')
  if (timerHandle) clearInterval(timerHandle)
})

async function start() {
  if (!httpxPath.value.trim()) {
    ElMessage.warning('请先配置 httpx 路径')
    return
  }
  await SetSetting('httpx_path', httpxPath.value.trim())

  log.value = []
  processed.value = 0
  alive.value = 0
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
    jobId.value = await RunHttpx(props.projectId, {
      httpx_path: httpxPath.value.trim(),
      threads: threads.value,
      timeout: timeout.value,
      retries: retries.value,
      rate_limit: rateLimit.value,
      probe_title: probeTitle.value,
      probe_tech: probeTech.value,
      probe_server: probeServer.value,
      probe_content_length: probeCL.value,
      probe_ip: probeIP.value,
      probe_cdn: probeCDN.value,
      follow_redirects: followRedir.value,
      match_codes: matchCodes.value,
      filter_codes: filterCodes.value,
      only_unprobed: onlyUnprobed.value,
      only_ip: onlyIP.value,
      only_domain: onlyDomain.value,
      skip_dns_failed: skipDnsFailed.value,
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
    ElMessage.warning('扫描进行中，先停止或等待结束')
    return
  }
  visible.value = false
}

function pickPath() {
  ElMessage.info('请在输入框中粘贴 httpx 可执行文件的完整路径')
}
</script>

<template>
  <el-dialog
    v-model="visible"
    title="httpx 探活"
    width="60%"
    top="5vh"
    draggable
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    :show-close="!running"
    class="httpx-dialog"
  >
    <el-form label-width="100px" size="default">
      <el-form-item label="httpx 路径">
        <el-input v-model="httpxPath" placeholder="C:\Tools\httpx.exe">
          <template #append>
            <el-button @click="pickPath">说明</el-button>
          </template>
        </el-input>
      </el-form-item>
    </el-form>

    <el-divider content-position="left">探测选项</el-divider>
    <div class="checkbox-row">
      <el-checkbox v-model="probeTitle">title</el-checkbox>
      <el-checkbox v-model="probeTech">tech-detect</el-checkbox>
      <el-checkbox v-model="probeServer">server</el-checkbox>
      <el-checkbox v-model="probeCL">content-length</el-checkbox>
      <el-checkbox v-model="probeIP">ip</el-checkbox>
      <el-checkbox v-model="probeCDN">cdn</el-checkbox>
      <el-checkbox v-model="followRedir">follow-redirects</el-checkbox>
    </div>

    <el-divider content-position="left">速率与超时</el-divider>
    <el-form label-width="100px" inline>
      <el-form-item label="threads"><el-input-number v-model="threads" :min="1" :max="500" /></el-form-item>
      <el-form-item label="timeout(s)"><el-input-number v-model="timeout" :min="1" :max="60" /></el-form-item>
      <el-form-item label="retries"><el-input-number v-model="retries" :min="0" :max="10" /></el-form-item>
      <el-form-item label="rate-limit"><el-input-number v-model="rateLimit" :min="0" :max="10000" /></el-form-item>
    </el-form>

    <el-divider content-position="left">过滤（可选）</el-divider>
    <el-form label-width="100px">
      <el-form-item label="-mc 匹配"><el-input v-model="matchCodes" placeholder="如 200,301" /></el-form-item>
      <el-form-item label="-fc 排除"><el-input v-model="filterCodes" placeholder="如 404,403" /></el-form-item>
    </el-form>

    <el-divider content-position="left">探活范围</el-divider>
    <div style="padding: 0 12px 8px">
      <el-checkbox v-model="onlyUnprobed">
        仅探活未探测的资产（增量模式，跳过已有 alive/dead 状态的）
      </el-checkbox>
      <div class="checkbox-row" style="margin-top: 8px">
        <el-checkbox v-model="onlyIP">
          仅探活 IP 资产
          <el-tooltip content="只对 IP 类型资产做探活"><el-icon><InfoFilled /></el-icon></el-tooltip>
        </el-checkbox>
        <el-checkbox v-model="onlyDomain">
          仅探活域名资产
          <el-tooltip content="只对域名类型资产做探活"><el-icon><InfoFilled /></el-icon></el-tooltip>
        </el-checkbox>
        <el-checkbox v-model="skipDnsFailed">
          跳过 DNS 无效域名
          <el-tooltip content="跳过标记为 &quot;DNS无效&quot; 的域名"><el-icon><InfoFilled /></el-icon></el-tooltip>
        </el-checkbox>
      </div>
    </div>

    <!-- 进度 + 用时 -->
    <div v-if="running || total > 0" class="progress-area">
      <el-progress
        :percentage="total === 0 ? 0 : Math.floor((processed / total) * 100)"
        :status="!running ? 'success' : ''"
      />
      <div class="meta">
        <span class="time">⏱ {{ elapsed }}</span>
        <span class="muted">{{ processed }} / {{ total }} | 存活 {{ alive }}</span>
      </div>
    </div>

    <div ref="logEl" class="log">
      <div v-for="(l, i) in log" :key="i" class="log-line">{{ l }}</div>
    </div>

    <template #footer>
      <el-button @click="close" :disabled="running">关闭</el-button>
      <el-button v-if="running" @click="togglePause">
        {{ paused ? '▶ 继续' : '⏸ 暂停' }}
      </el-button>
      <el-button v-if="running" type="danger" @click="stop">停止</el-button>
      <el-button v-else type="primary" @click="start">开始探活</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.checkbox-row {
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
  padding: 4px 8px 8px;
}
.progress-area {
  margin-top: 12px;
  padding: 8px 12px;
  background: #2a2d36;
  border-radius: 6px;
}
.meta {
  display: flex;
  justify-content: space-between;
  margin-top: 4px;
  font-size: 12px;
}
.time { color: #1890ff; font-weight: 600; }
.muted { color: #aaaaaa; }
.log {
  margin-top: 10px;
  flex: 1;
  min-height: 120px;
  overflow-y: auto;
  background: #1a1c22;
  color: #d4d4d4;
  font-family: Consolas, monospace;
  font-size: 12px;
  padding: 8px 10px;
  border-radius: 6px;
  border: 1px solid #3a3e4a;
}
.log-line {
  white-space: pre-wrap;
  line-height: 1.4;
}
</style>

<style>
.httpx-dialog {
  height: 90vh;
  display: flex;
  flex-direction: column;
  margin: 0 !important;
}
.httpx-dialog .el-dialog__body {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  padding-top: 10px;
  padding-bottom: 10px;
}
</style>
