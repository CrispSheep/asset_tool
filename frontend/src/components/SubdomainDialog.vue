<script setup lang="ts">
import { ref, computed, onBeforeUnmount, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Search, Plus, Right } from '@element-plus/icons-vue'
import { RunSubdomain, GetSetting, SetSetting } from '../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import { useScannerDialog } from '../composables/useScannerDialog'

const props = defineProps<{ projectId: number }>()
const emit = defineEmits<{ discovered: [] }>()
const visible = defineModel<boolean>('visible', { default: false })

const {
  running, paused, log, logEl, jobId, elapsed,
  appendLog, startTimer, stopTimer, resetLog,
  togglePause, stop, close,
} = useScannerDialog(visible)

// ── 工具配置 ──────────────────────────────────────────────
const tool = ref<'subfinder' | 'ksubdomain' | 'oneforall'>('subfinder')
const toolPath = ref('')
const pythonPath = ref('')
const sfThreads = ref(10)
const sfTimeout = ref(30)
const sfAll = ref(false)

// ── 探测队列 ──────────────────────────────────────────────
const domainInput = ref('')
const queue = ref<string[]>([])

// ── 探测结果 ──────────────────────────────────────────────
const results = ref<string[]>([])
const resultSearch = ref('')
const resultSelected = ref<string[]>([])

const filteredResults = computed(() => {
  const kw = resultSearch.value.trim().toLowerCase()
  if (!kw) return results.value
  return results.value.filter(d => d.toLowerCase().includes(kw))
})

const total = ref(0)
const processed = ref(0)
const newCount = ref(0)

// ── 队列操作 ──────────────────────────────────────────────
function addToQueue() {
  const text = domainInput.value.trim()
  if (!text) return
  const lines = text.split(/[\n,;]+/).map(l => l.trim()).filter(Boolean)
  for (const d of lines) {
    if (!queue.value.includes(d)) queue.value.push(d)
  }
  domainInput.value = ''
}

function removeFromQueue(d: string) {
  queue.value = queue.value.filter(x => x !== d)
}

function addSelectedToQueue() {
  for (const d of resultSelected.value) {
    if (!queue.value.includes(d)) queue.value.push(d)
  }
  resultSelected.value = []
  ElMessage.success(`已加入探测队列`)
}

// ── Settings ─────────────────────────────────────────────
async function loadSettings() {
  toolPath.value = await GetSetting(`${tool.value}_path`)
  if (tool.value === 'oneforall') {
    pythonPath.value = await GetSetting('oneforall_python_path')
  }
}

watch(tool, () => loadSettings())
watch(visible, (v) => { if (v) loadSettings() })

// ── 事件监听 ─────────────────────────────────────────────
EventsOn('subdomain:start', (data: any) => {
  total.value = data.total
  appendLog(`[i] 共 ${data.total} 个目标域名待探测`)
})
EventsOn('subdomain:log', (line: string) => appendLog(line))
EventsOn('subdomain:found', (domain: string) => {
  if (!results.value.includes(domain)) results.value.push(domain)
})
EventsOn('subdomain:progress', (data: any) => {
  processed.value = data.done
  newCount.value = data.new
})
EventsOn('subdomain:done', (data: any) => {
  running.value = false
  paused.value = false
  stopTimer()
  appendLog(`\n完成：新增 ${data.new} 条子域名，本次共发现 ${results.value.length} 条，用时 ${elapsed.value}`)
  emit('discovered')
})
EventsOn('subdomain:error', (msg: string) => {
  running.value = false
  stopTimer()
  ElMessage.error(msg)
  appendLog(`[!] ${msg}`)
})

onBeforeUnmount(() => {
  for (const e of ['subdomain:start', 'subdomain:log', 'subdomain:found',
                    'subdomain:progress', 'subdomain:done', 'subdomain:error']) {
    EventsOff(e)
  }
})

// ── 控制 ─────────────────────────────────────────────────
async function start() {
  if (!toolPath.value.trim()) {
    ElMessage.warning('请先配置工具路径')
    return
  }
  if (queue.value.length === 0) {
    ElMessage.warning('探测队列为空，请先添加目标域名')
    return
  }

  await SetSetting(`${tool.value}_path`, toolPath.value.trim())
  if (tool.value === 'oneforall') {
    await SetSetting('oneforall_python_path', pythonPath.value.trim())
  }

  resetLog()
  processed.value = 0
  total.value = 0
  newCount.value = 0
  paused.value = false
  running.value = true
  startTimer()

  const domains = [...queue.value]
  queue.value = []

  try {
    jobId.value = await RunSubdomain(props.projectId, {
      tool: tool.value,
      tool_path: toolPath.value.trim(),
      python_path: pythonPath.value.trim(),
      domains,
      threads: sfThreads.value,
      timeout: sfTimeout.value,
      all: sfAll.value,
    })
  } catch (e: any) {
    running.value = false
    stopTimer()
    ElMessage.error('启动失败: ' + e)
  }
}
</script>

<template>
  <el-dialog v-model="visible" title="子域名探测" width="65%" top="3vh" draggable
    :close-on-click-modal="false" :close-on-press-escape="false" :show-close="!running"
    class="subdomain-dialog">

    <!-- 工具选择 + 路径 -->
    <el-form label-width="100px" size="default">
      <el-form-item label="探测工具">
        <el-radio-group v-model="tool">
          <el-radio value="subfinder">subfinder（被动收集，快）</el-radio>
          <el-radio value="ksubdomain">ksubdomain（DNS 爆破）</el-radio>
          <el-radio value="oneforall">Oneforall（综合，需 venv）</el-radio>
        </el-radio-group>
      </el-form-item>
      <el-form-item label="工具路径">
        <el-input v-model="toolPath" :placeholder="
          tool === 'subfinder' ? 'C:\\Tools\\subfinder.exe' :
          tool === 'ksubdomain' ? 'C:\\Tools\\ksubdomain.exe' :
          'C:\\Tools\\OneForAll\\oneforall.py'" />
      </el-form-item>
      <el-form-item v-if="tool === 'oneforall'" label="Python 路径">
        <el-input v-model="pythonPath" placeholder="venv\Scripts\python.exe（留空走系统 python）" />
      </el-form-item>
      <template v-if="tool === 'subfinder'">
        <el-form-item label="并发 / 超时">
          <el-input-number v-model="sfThreads" :min="1" :max="200" style="width:100px" />
          <span style="margin:0 12px;color:var(--text-muted)">线程</span>
          <el-input-number v-model="sfTimeout" :min="5" :max="300" style="width:100px" />
          <span style="margin-left:8px;color:var(--text-muted)">秒</span>
          <el-checkbox v-model="sfAll" style="margin-left:16px">-all 全部源</el-checkbox>
        </el-form-item>
      </template>
    </el-form>

    <el-divider content-position="left">目标域名</el-divider>

    <!-- 输入 + 添加 -->
    <div class="input-row">
      <el-input v-model="domainInput" placeholder="输入主域名（如 whu.edu.cn），支持多行粘贴"
        type="textarea" :rows="2" @keyup.ctrl.enter="addToQueue" />
      <el-button type="primary" :icon="Plus" @click="addToQueue" style="align-self:flex-start;margin-top:2px">
        添加到队列
      </el-button>
    </div>

    <!-- 探测队列 -->
    <div class="queue-area" v-if="queue.length > 0">
      <span class="label">探测队列 ({{ queue.length }})：</span>
      <el-tag v-for="d in queue" :key="d" closable @close="removeFromQueue(d)"
        type="primary" size="default" style="margin: 2px 4px">
        {{ d }}
      </el-tag>
    </div>

    <!-- 探测结果 -->
    <el-divider content-position="left" v-if="results.length > 0">
      探测结果（本次发现 {{ results.length }} 条）
    </el-divider>

    <div v-if="results.length > 0" class="results-area">
      <div class="results-toolbar">
        <el-input v-model="resultSearch" :prefix-icon="Search" placeholder="搜索子域名…"
          clearable style="flex:1" size="small" />
        <span class="muted">{{ filteredResults.length }} / {{ results.length }}</span>
        <el-button size="small" :icon="Right" :disabled="resultSelected.length === 0"
          @click="addSelectedToQueue">
          选中 {{ resultSelected.length }} 条加入队列
        </el-button>
      </div>
      <el-checkbox-group v-model="resultSelected" class="result-list">
        <el-checkbox v-for="d in filteredResults" :key="d" :value="d" :label="d" />
      </el-checkbox-group>
    </div>

    <!-- 进度 -->
    <div v-if="running || total > 0" class="progress-area">
      <el-progress
        :percentage="total === 0 ? 0 : Math.floor((processed / total) * 100)"
        :status="!running ? 'success' : ''" />
      <div class="meta">
        <span class="time">⏱ {{ elapsed }}</span>
        <span class="muted">{{ processed }} / {{ total }} | 新增 {{ newCount }} 条</span>
      </div>
    </div>

    <div ref="logEl" class="log">
      <div v-for="(l, i) in log" :key="i" class="log-line">{{ l }}</div>
    </div>

    <template #footer>
      <el-button @click="close" :disabled="running">关闭</el-button>
      <el-button v-if="running" @click="visible = false">📥 收起后台</el-button>
      <el-button v-if="running" @click="togglePause">{{ paused ? '▶ 继续' : '⏸ 暂停' }}</el-button>
      <el-button v-if="running" type="danger" @click="stop">停止</el-button>
      <el-button v-else type="primary" @click="start" :disabled="queue.length === 0">
        开始探测 ({{ queue.length }})
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.input-row { display: flex; gap: 10px; }
.queue-area {
  margin-top: 10px; padding: 8px 10px;
  background: var(--bg-surface); border-radius: 6px;
  display: flex; flex-wrap: wrap; align-items: center; gap: 4px;
}
.queue-area .label { color: var(--text-regular); font-size: 13px; margin-right: 6px; }
.results-area { display: flex; flex-direction: column; gap: 6px; }
.results-toolbar { display: flex; align-items: center; gap: 10px; }
.result-list {
  padding: 8px 10px; max-height: 180px; overflow-y: auto;
  background: var(--bg-log); border: 1px solid var(--border); border-radius: 6px;
  display: flex; flex-direction: column; gap: 2px;
}
.result-list :deep(.el-checkbox) { height: auto; margin-right: 0; }
.progress-area {
  margin-top: 10px; padding: 8px 12px;
  background: var(--bg-surface); border-radius: 6px;
}
.meta { display: flex; justify-content: space-between; margin-top: 4px; font-size: 12px; }
.time { color: var(--accent); font-weight: 600; }
.muted { color: var(--text-muted); font-size: 12px; }
.log {
  margin-top: 8px; flex: 1; min-height: 100px; max-height: 200px; overflow-y: auto;
  background: var(--bg-log); color: var(--text-regular);
  font-family: Consolas, monospace; font-size: 12px;
  padding: 8px 10px; border-radius: 6px; border: 1px solid var(--border);
}
.log-line { white-space: pre-wrap; line-height: 1.4; }
</style>

<style>
.subdomain-dialog { height: 92vh; display: flex; flex-direction: column; margin: 0 !important; }
.subdomain-dialog .el-dialog__body {
  flex: 1; overflow-y: auto; display: flex; flex-direction: column;
  padding-top: 10px; padding-bottom: 10px;
}
</style>
