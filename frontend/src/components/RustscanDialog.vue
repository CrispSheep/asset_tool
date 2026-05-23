<script setup lang="ts">
import { ref, watch, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'
import {
  RunRustscan, PauseJob, ResumeJob, CancelJob,
  GetSetting, SetSetting,
} from '../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'

const props = defineProps<{ projectId: number }>()
const emit = defineEmits<{ scanned: [] }>()
const visible = defineModel<boolean>('visible', { default: false })

const path = ref('')
const ports = ref('1-65535')
const ulimit = ref(5000)
const batch = ref(4500)
const timeout = ref(3000)
const tries = ref(1)
const onlyIP = ref(true)
const onlyDomain = ref(false)
const onlyAlive = ref(false)
const noBanner = ref(true)
const skipDnsFailed = ref(true)
const useIPPorts = ref(false)
const domainNetwork = ref('')

watch(onlyIP, v => { if (v) onlyDomain.value = false })
watch(onlyDomain, v => { if (v) onlyIP.value = false })

const running = ref(false)
const paused = ref(false)
const total = ref(0)
const processed = ref(0)
const newCount = ref(0)
const log = ref<string[]>([])
const logEl = ref<HTMLElement | null>(null)
const jobId = ref('')

const startedAt = ref(0)
const elapsed = ref('00:00')
const eta = ref('')
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

async function loadSettings() {
  path.value = await GetSetting('rustscan_path')
  const p = await GetSetting('rustscan_ports')
  if (p) ports.value = p
}
loadSettings()

EventsOn('rustscan:start', (data: any) => {
  total.value = data.total
  appendLog(`[i] 共 ${data.total} 个目标待扫描`)
})
EventsOn('rustscan:log', (line: string) => appendLog(line))
EventsOn('rustscan:progress', (data: any) => {
  processed.value = data.done
  newCount.value += data.new || 0
  if (data.done > 0 && data.done < data.total) {
    const avg = (Date.now() - startedAt.value) / data.done
    eta.value = '剩余约 ' + fmt(avg * (data.total - data.done))
  }
})
EventsOn('rustscan:done', (data: any) => {
  running.value = false
  paused.value = false
  if (timerHandle) { clearInterval(timerHandle); timerHandle = null }
  eta.value = ''
  appendLog(`\n完成：扫描 ${data.done}/${data.total} 个目标，新增 ${data.new} 条端口资产，用时 ${elapsed.value}`)
  emit('scanned')
})
EventsOn('rustscan:error', (msg: string) => {
  running.value = false
  if (timerHandle) { clearInterval(timerHandle); timerHandle = null }
  ElMessage.error(msg)
  appendLog(`[!] ${msg}`)
})

onBeforeUnmount(() => {
  EventsOff('rustscan:start')
  EventsOff('rustscan:log')
  EventsOff('rustscan:progress')
  EventsOff('rustscan:done')
  EventsOff('rustscan:error')
  if (timerHandle) clearInterval(timerHandle)
})

async function start() {
  if (!path.value.trim()) {
    ElMessage.warning('请先配置 rustscan 路径')
    return
  }
  await SetSetting('rustscan_path', path.value.trim())
  await SetSetting('rustscan_ports', ports.value.trim())

  log.value = []
  processed.value = 0
  total.value = 0
  newCount.value = 0
  paused.value = false
  running.value = true
  startedAt.value = Date.now()
  elapsed.value = '00:00'
  if (timerHandle) clearInterval(timerHandle)
  timerHandle = window.setInterval(() => {
    elapsed.value = fmt(Date.now() - startedAt.value)
  }, 1000)

  try {
    jobId.value = await RunRustscan(props.projectId, {
      rustscan_path: path.value.trim(),
      ports: ports.value.trim(),
      ulimit: ulimit.value,
      batch_size: batch.value,
      timeout: timeout.value,
      tries: tries.value,
      no_cdn: false,
      only_ip: onlyIP.value,
      only_domain: onlyDomain.value,
      only_alive: onlyAlive.value,
      no_banner: noBanner.value,
      skip_dns_failed: skipDnsFailed.value,
      use_ip_ports: useIPPorts.value,
      domain_network: domainNetwork.value,
    })
  } catch (e: any) {
    running.value = false
    if (timerHandle) { clearInterval(timerHandle); timerHandle = null }
    ElMessage.error('启动失败: ' + e)
  }
}

function togglePause() {
  if (!running.value) return
  if (paused.value) {
    ResumeJob(jobId.value)
    paused.value = false
    appendLog('[i] 已继续')
  } else {
    PauseJob(jobId.value)
    paused.value = true
    appendLog('[i] 已暂停（当前 host 扫完后停止）')
  }
}

function stop() {
  if (jobId.value) CancelJob(jobId.value)
}

function close() {
  if (running.value) {
    ElMessage.warning('扫描进行中，可点「📥 收起」让它在后台跑，或先停止')
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
    title="端口扫描 (rustscan)"
    width="65%"
    top="5vh"
    draggable
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    :show-close="!running"
    class="rustscan-dialog"
  >
    <el-form label-width="120px">
      <el-form-item label="rustscan 路径">
        <el-input v-model="path" placeholder="C:\Tools\rustscan.exe" />
      </el-form-item>
      <el-form-item label="端口范围">
        <el-input v-model="ports" placeholder="范围 1-65535 / 列表 80,443">
          <template #append>常用：80,443,8080,8443</template>
        </el-input>
      </el-form-item>
    </el-form>

    <el-divider content-position="left">扫描参数</el-divider>
    <el-form label-width="120px" inline>
      <el-form-item>
        <template #label>
          ulimit
          <el-tooltip content="文件描述符上限（同时打开的 socket 数）。Windows 下只是并发上限，越大越快但更易丢包">
            <el-icon><InfoFilled /></el-icon>
          </el-tooltip>
        </template>
        <el-input-number v-model="ulimit" :min="1000" :max="100000" :step="1000" />
      </el-form-item>
      <el-form-item>
        <template #label>
          batch-size
          <el-tooltip content="每批同时发出的端口探测数。必须 ≤ ulimit。值越大越快，但目标可能丢包">
            <el-icon><InfoFilled /></el-icon>
          </el-tooltip>
        </template>
        <el-input-number v-model="batch" :min="100" :max="65535" :step="500" />
      </el-form-item>
      <el-form-item>
        <template #label>
          timeout
          <el-tooltip content="单端口连接超时（毫秒）。内网可调到 500，外网/跨境调到 3000~5000">
            <el-icon><InfoFilled /></el-icon>
          </el-tooltip>
        </template>
        <el-input-number v-model="timeout" :min="100" :max="60000" :step="500" />
      </el-form-item>
      <el-form-item>
        <template #label>
          tries
          <el-tooltip content="重试次数。0 表示不重试，可加快扫描但更易漏报">
            <el-icon><InfoFilled /></el-icon>
          </el-tooltip>
        </template>
        <el-input-number v-model="tries" :min="0" :max="10" />
      </el-form-item>
    </el-form>

    <el-divider content-position="left">扫描范围</el-divider>
    <div class="checkbox-row">
      <el-checkbox v-model="onlyIP">
        仅扫 IP 资产
        <el-tooltip content="域名常被 CDN 干扰，只扫 IP 更准确">
          <el-icon><InfoFilled /></el-icon>
        </el-tooltip>
      </el-checkbox>
      <el-checkbox v-model="onlyDomain">
        仅扫域名资产
        <el-tooltip content="先扫 IP 端口后再单独扫域名端口时使用">
          <el-icon><InfoFilled /></el-icon>
        </el-tooltip>
      </el-checkbox>
      <el-checkbox v-model="onlyAlive">
        仅扫已存活资产 (httpx alive)
        <el-tooltip content="只对 httpx 探活后标记为 alive 的资产做端口扫描，缩小范围">
          <el-icon><InfoFilled /></el-icon>
        </el-tooltip>
      </el-checkbox>
      <el-checkbox v-model="noBanner">
        accessible 模式
        <el-tooltip content="关闭 rustscan 的 ANSI 彩色横幅，输出更干净，便于解析">
          <el-icon><InfoFilled /></el-icon>
        </el-tooltip>
      </el-checkbox>
      <el-checkbox v-model="skipDnsFailed">
        跳过 DNS 无效域名
        <el-tooltip content="跳过标记为 &quot;DNS无效&quot; 的域名，避免浪费时间">
          <el-icon><InfoFilled /></el-icon>
        </el-tooltip>
      </el-checkbox>
      <el-checkbox v-model="useIPPorts">
        使用 IP 已发现端口
        <el-tooltip content="用 IP 资产已扫到的端口列表作为扫描范围，适合先扫 IP 再扫域名的场景">
          <el-icon><InfoFilled /></el-icon>
        </el-tooltip>
      </el-checkbox>
    </div>
    <div v-if="onlyDomain" style="padding: 4px 8px 8px; display: flex; align-items: center; gap: 8px">
      <span style="font-size: 13px; color: var(--text-muted)">域名网络：</span>
      <el-select v-model="domainNetwork" size="small" style="width: 180px">
        <el-option label="全部域名" value="" />
        <el-option label="仅外网域名" value="extranet" />
        <el-option label="仅内网域名" value="intranet" />
      </el-select>
      <el-tooltip content="根据 DNS 解析出的 IP 判断域名指向内网还是外网">
        <el-icon style="color: var(--text-muted)"><InfoFilled /></el-icon>
      </el-tooltip>
    </div>

    <!-- 进度 -->
    <div v-if="running || total > 0" class="progress-area">
      <el-progress
        :percentage="total === 0 ? 0 : Math.floor((processed / total) * 100)"
        :status="!running ? 'success' : ''"
      />
      <div class="meta">
        <span class="time">⏱ {{ elapsed }}</span>
        <span class="muted">{{ processed }} / {{ total }} | 新增 {{ newCount }} 条</span>
        <span class="eta">{{ eta }}</span>
      </div>
    </div>

    <div ref="logEl" class="log">
      <div v-for="(l, i) in log" :key="i" class="log-line">{{ l }}</div>
    </div>

    <template #footer>
      <el-button @click="close" :disabled="running">关闭</el-button>
      <el-button v-if="running" @click="hide">📥 收起后台</el-button>
      <el-button v-if="running" @click="togglePause">
        {{ paused ? '▶ 继续' : '⏸ 暂停' }}
      </el-button>
      <el-button v-if="running" type="danger" @click="stop">停止</el-button>
      <el-button v-else type="primary" @click="start">开始扫描</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.checkbox-row {
  display: flex;
  flex-wrap: wrap;
  gap: 18px;
  padding: 4px 8px 8px;
}
.checkbox-row .el-icon {
  margin-left: 4px;
  color: var(--text-muted);
  vertical-align: -2px;
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
.eta { color: var(--text-dimmed); }
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
</style>

<style>
.rustscan-dialog {
  height: 90vh;
  display: flex;
  flex-direction: column;
  margin: 0 !important;
}
.rustscan-dialog .el-dialog__body {
  flex: 1;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  padding-top: 10px;
  padding-bottom: 10px;
}
</style>
