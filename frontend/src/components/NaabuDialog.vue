<script setup lang="ts">
import { ref, watch, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'
import { InfoFilled } from '@element-plus/icons-vue'
import {
  RunNaabu, PauseJob, ResumeJob, CancelJob,
  GetSetting, SetSetting,
} from '../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'

const props = defineProps<{ projectId: number }>()
const emit = defineEmits<{ scanned: [] }>()
const visible = defineModel<boolean>('visible', { default: false })

const path = ref('')
const ports = ref('80,443,8080,8443,3306,3389,6379,22,21,3000,5000,8000,8888,9000')
const rate = ref(2000)
const concurrency = ref(25)
const timeout = ref(1000)
const retries = ref(3)
const scanType = ref<'c' | 's'>('c')
const excludeCDN = ref(false)
const verify = ref(false)
const onlyIP = ref(true)
const onlyDomain = ref(false)
const onlyAlive = ref(false)
const skipDnsFailed = ref(true)
const useIPPorts = ref(false)
const domainNetwork = ref('')

watch(onlyIP, v => { if (v) onlyDomain.value = false })
watch(onlyDomain, v => { if (v) onlyIP.value = false })

const running = ref(false)
const paused = ref(false)
const totalHosts = ref(0)
const portsFound = ref(0)
const newCount = ref(0)
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

async function loadSettings() {
  path.value = await GetSetting('naabu_path')
  const p = await GetSetting('naabu_ports')
  if (p) ports.value = p
}
loadSettings()

EventsOn('naabu:start', (data: any) => {
  totalHosts.value = data.hosts
  appendLog(`[i] 开始扫描 ${data.hosts} 个目标（naabu 全局并发，无 host 进度）`)
})
EventsOn('naabu:log', (line: string) => appendLog(line))
EventsOn('naabu:port', (data: any) => {
  portsFound.value = data.count
  appendLog(`[+] ${data.host}:${data.port}`)
})
EventsOn('naabu:done', (data: any) => {
  running.value = false
  paused.value = false
  if (timerHandle) { clearInterval(timerHandle); timerHandle = null }
  newCount.value = data.new
  appendLog(
    `\n完成：${data.hosts} 个 host 共发现 ${data.ports} 个开放端口，写入 ${data.new} 条新资产，用时 ${elapsed.value}`
  )
  emit('scanned')
})
EventsOn('naabu:error', (msg: string) => {
  running.value = false
  if (timerHandle) { clearInterval(timerHandle); timerHandle = null }
  ElMessage.error(msg)
  appendLog(`[!] ${msg}`)
})

onBeforeUnmount(() => {
  EventsOff('naabu:start')
  EventsOff('naabu:log')
  EventsOff('naabu:port')
  EventsOff('naabu:done')
  EventsOff('naabu:error')
  if (timerHandle) clearInterval(timerHandle)
})

async function start() {
  if (!path.value.trim()) {
    ElMessage.warning('请先配置 naabu 路径')
    return
  }
  await SetSetting('naabu_path', path.value.trim())
  await SetSetting('naabu_ports', ports.value.trim())

  log.value = []
  totalHosts.value = 0
  portsFound.value = 0
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
    jobId.value = await RunNaabu(props.projectId, {
      naabu_path: path.value.trim(),
      ports: ports.value.trim(),
      rate: rate.value,
      concurrency: concurrency.value,
      timeout: timeout.value,
      retries: retries.value,
      scan_type: scanType.value,
      exclude_cdn: excludeCDN.value,
      verify: verify.value,
      only_ip: onlyIP.value,
      only_domain: onlyDomain.value,
      only_alive: onlyAlive.value,
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
    appendLog('[i] 已暂停（处理完当前一行后停止读取）')
  }
}

function stop() {
  if (jobId.value) CancelJob(jobId.value)
}

function close() {
  if (running.value) {
    ElMessage.warning('扫描进行中，可点「📥 收起」让它后台运行，或先停止')
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
    title="端口扫描 (naabu)"
    width="65%"
    top="5vh"
    draggable
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    :show-close="!running"
    class="naabu-dialog"
  >
    <el-form label-width="120px">
      <el-form-item label="naabu 路径">
        <el-input v-model="path" placeholder="C:\Tools\naabu.exe" />
      </el-form-item>
      <el-form-item label="端口范围 -p">
        <el-input v-model="ports" placeholder="80,443,1-1000 / - 表示全端口">
          <template #append>常用：80,443,8080,8443,3306,3389,6379,22</template>
        </el-input>
      </el-form-item>
    </el-form>

    <el-divider content-position="left">速率与并发</el-divider>
    <el-form label-width="120px" inline>
      <el-form-item>
        <template #label>
          rate
          <el-tooltip content="每秒发出的数据包数。默认 1000，大批量目标可调到 2000-5000；目标弱可降到 500">
            <el-icon><InfoFilled /></el-icon>
          </el-tooltip>
        </template>
        <el-input-number v-model="rate" :min="100" :max="50000" :step="500" />
      </el-form-item>
      <el-form-item>
        <template #label>
          concurrency -c
          <el-tooltip content="同时扫描的主机数。25 是默认值；上千 IP 可调到 50-100">
            <el-icon><InfoFilled /></el-icon>
          </el-tooltip>
        </template>
        <el-input-number v-model="concurrency" :min="1" :max="500" />
      </el-form-item>
      <el-form-item>
        <template #label>
          timeout
          <el-tooltip content="单端口连接超时（毫秒）。默认 1000，跨境/慢网络调到 2000-3000">
            <el-icon><InfoFilled /></el-icon>
          </el-tooltip>
        </template>
        <el-input-number v-model="timeout" :min="100" :max="60000" :step="500" />
      </el-form-item>
      <el-form-item>
        <template #label>
          retries
          <el-tooltip content="重试次数。默认 3，丢包高的网络可加大">
            <el-icon><InfoFilled /></el-icon>
          </el-tooltip>
        </template>
        <el-input-number v-model="retries" :min="0" :max="10" />
      </el-form-item>
    </el-form>

    <el-divider content-position="left">扫描方式</el-divider>
    <div class="checkbox-row">
      <el-radio-group v-model="scanType">
        <el-radio value="c">CONNECT 扫描 (默认，无需管理员权限)</el-radio>
        <el-radio value="s">SYN 扫描 (更快但需要管理员)</el-radio>
      </el-radio-group>
    </div>

    <el-divider content-position="left">扫描范围 / 过滤</el-divider>
    <div class="checkbox-row">
      <el-checkbox v-model="onlyIP">
        仅扫 IP 资产
        <el-tooltip content="域名常被 CDN 干扰，只扫 IP 更准确"><el-icon><InfoFilled /></el-icon></el-tooltip>
      </el-checkbox>
      <el-checkbox v-model="onlyDomain">
        仅扫域名资产
        <el-tooltip content="先扫 IP 端口后再单独扫域名端口时使用"><el-icon><InfoFilled /></el-icon></el-tooltip>
      </el-checkbox>
      <el-checkbox v-model="onlyAlive">
        仅扫已存活资产 (httpx alive)
        <el-tooltip content="只对 httpx 探活后 alive 的资产做端口扫描"><el-icon><InfoFilled /></el-icon></el-tooltip>
      </el-checkbox>
      <el-checkbox v-model="excludeCDN">
        跳过 CDN IP -exclude-cdn
        <el-tooltip content="naabu 内置 CDN IP 列表，跳过这些避免无效扫描"><el-icon><InfoFilled /></el-icon></el-tooltip>
      </el-checkbox>
      <el-checkbox v-model="verify">
        二次确认 -verify
        <el-tooltip content="对发现的端口再次验证，更准但慢约 30%"><el-icon><InfoFilled /></el-icon></el-tooltip>
      </el-checkbox>
      <el-checkbox v-model="skipDnsFailed">
        跳过 DNS 无效域名
        <el-tooltip content="跳过标记为 &quot;DNS无效&quot; 的域名，避免浪费时间"><el-icon><InfoFilled /></el-icon></el-tooltip>
      </el-checkbox>
      <el-checkbox v-model="useIPPorts">
        使用 IP 已发现端口
        <el-tooltip content="用 IP 资产已扫到的端口列表作为扫描范围，适合先扫 IP 再扫域名的场景"><el-icon><InfoFilled /></el-icon></el-tooltip>
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

    <!-- 进度（naabu 全局并发，没有 host 维度的进度，用 indeterminate 风格） -->
    <div v-if="running || portsFound > 0" class="progress-area">
      <el-progress
        :percentage="running ? 100 : 100"
        :indeterminate="running"
        :status="!running ? 'success' : ''"
        :duration="3"
      />
      <div class="meta">
        <span class="time">⏱ {{ elapsed }}</span>
        <span class="muted">{{ totalHosts }} 个目标 | 已发现 {{ portsFound }} 个端口</span>
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
.naabu-dialog {
  height: 90vh;
  display: flex;
  flex-direction: column;
  margin: 0 !important;
}
.naabu-dialog .el-dialog__body {
  flex: 1;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  padding-top: 10px;
  padding-bottom: 10px;
}
</style>
