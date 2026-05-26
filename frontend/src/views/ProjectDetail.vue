<script setup lang="ts">
import { ref, computed, onMounted, watch, h } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox, ElTag, ElCheckbox } from 'element-plus'
import {
  ArrowLeft, Upload, Search, Delete, DocumentCopy, Download, Aim, Connection, Share, Document,
  Monitor, Moon, Sunny,
} from '@element-plus/icons-vue'
import {
  ListAssetsPage, ListAssets, CountAssetStats, DeleteAssets, GetSetting, SetSetting,
  BatchAddTag, BatchRemoveTag,
} from '../../wailsjs/go/main/App'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import type { model } from '../../wailsjs/go/models'
import ImportDialog from '../components/ImportDialog.vue'
import HttpxDialog from '../components/HttpxDialog.vue'
import RustscanDialog from '../components/RustscanDialog.vue'
import NaabuDialog from '../components/NaabuDialog.vue'
import SubdomainDialog from '../components/SubdomainDialog.vue'
import NoteDialog from '../components/NoteDialog.vue'
import DnsDialog from '../components/DnsDialog.vue'
import { useTheme } from '../composables/useTheme'

const { theme, cycleTheme } = useTheme()

const route = useRoute()
const router = useRouter()
const projectId = Number(route.params.id)
const projectName = computed(() => (route.query.name as string) || `项目 #${projectId}`)

const tab = ref<'ip' | 'domain'>('ip')
const assets = ref<model.Asset[]>([])
const totalCount = ref(0)
const loading = ref(false)

const search = ref('')
const filterMode = ref<'all' | 'alive' | 'non-http' | 'unprobed' | 'dead'>('all')
const networkFilter = ref<'all' | 'intranet' | 'extranet'>('all')
const page = ref(1)
const pageSize = ref(50)
const sorts = ref<Array<{ key: string, order: 'asc' | 'desc' }>>([])

// 统计仪表盘数据（独立请求，不依赖分页）
const stats = ref<Record<string, number>>({ total: 0, ip: 0, domain: 0, alive: 0, dead: 0, unprobed: 0, ports: 0 })

const selected = ref<model.Asset[]>([])
const importVisible = ref(false)
const httpxVisible = ref(false)
const rustscanVisible = ref(false)
const naabuVisible = ref(false)
const subdomainVisible = ref(false)
const noteVisible = ref(false)
const dnsVisible = ref(false)

// filterMode 映射为后端 statusFilter
function getStatusFilter(): string {
  switch (filterMode.value) {
    case 'alive': return 'alive'
    case 'dead': return 'dead'
    case 'unprobed': return 'unprobed'
    case 'non-http': return 'non-http'
    default: return ''
  }
}

async function refreshStats() {
  try {
    stats.value = await CountAssetStats(projectId)
  } catch { /* ignore */ }
}

let searchTimer: number | null = null
function onSearchInput() {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = window.setTimeout(() => {
    page.value = 1
    fetchPage()
  }, 300)
}

async function fetchPage() {
  loading.value = true
  try {
    const res = await ListAssetsPage(
      projectId,
      tab.value,
      getStatusFilter(),
      search.value.trim(),
      networkFilter.value === 'all' ? '' : networkFilter.value,
      JSON.stringify(sorts.value),
      page.value,
      pageSize.value,
    )
    assets.value = res.items || []
    totalCount.value = res.total || 0
  } catch (e: any) {
    ElMessage.error('加载失败: ' + e)
  } finally {
    loading.value = false
  }
}

async function refresh() {
  await Promise.all([fetchPage(), refreshStats()])
}

function onPageSizeChange() {
  page.value = 1
  fetchPage()
}

// 切换 tab / 过滤模式时重置到第一页
watch([tab, filterMode, networkFilter], () => {
  page.value = 1
  selected.value = []
  selectedIds.value = new Set()
  if (tab.value === 'domain') networkFilter.value = 'all'
  refresh()
})

const tabLabel = (kind: 'ip' | 'domain') => {
  if (kind === tab.value) {
    return `${kind === 'ip' ? 'IP' : '域名'}列表 (${totalCount.value})`
  }
  // 非当前 tab 显示统计数
  const count = kind === 'ip' ? stats.value.ip : stats.value.domain
  return `${kind === 'ip' ? 'IP' : '域名'}列表 (${count})`
}

function statusType(s: string) {
  if (s === 'alive') return 'success'
  if (s === 'dead') return 'danger'
  return 'info'
}

function statusText(s: string) {
  if (s === 'alive') return 'alive'
  if (s === 'dead') return 'dead'
  return '未探活'
}

function fullURL(a: model.Asset) {
  if (a.host.startsWith('http://') || a.host.startsWith('https://')) return a.host
  const target = a.port ? `${a.host}:${a.port}` : a.host
  const scheme = ['443', '8443'].includes(a.port || '') ? 'https' : 'http'
  return `${scheme}://${target}`
}

function copyHost(a: model.Asset) {
  const text = a.port ? `${a.host}:${a.port}` : a.host
  navigator.clipboard.writeText(text)
  ElMessage.success(`已复制: ${text}`)
}

function openInBrowser(a: model.Asset) {
  BrowserOpenURL(fullURL(a))
}

// ── 标签系统 ──────────────────────────────────────────────
const TAG_COLORS: Record<string, string> = {
  '已授权': 'success', '未授权': 'danger',
  '高价值': 'warning', '低价值': 'info',
  '已测试': 'success', '待测试': '',
  '有漏洞': 'danger',
}
const PRESET_TAGS = ['已授权', '未授权', '高价值', '低价值', '已测试', '待测试', '有漏洞']

function tagColor(t: string): '' | 'success' | 'warning' | 'danger' | 'info' | 'primary' {
  return (TAG_COLORS[t] || '') as any
}

async function removeTag(id: number, tag: string) {
  await BatchRemoveTag([id], tag)
  refresh()
}

async function batchTag() {
  if (selected.value.length === 0) {
    ElMessage.warning('请先选中资产')
    return
  }
  try {
    const { value } = await ElMessageBox.prompt('输入标签名（或选择预设）', '批量打标签', {
      confirmButtonText: '添加',
      cancelButtonText: '取消',
      inputValidator: (v) => (v && v.trim() ? true : '标签不能为空'),
    })
    const ids = selected.value.map(a => a.id)
    await BatchAddTag(ids, value.trim())
    ElMessage.success(`已给 ${ids.length} 条资产添加标签「${value.trim()}」`)
    refresh()
  } catch { /* cancel */ }
}

function copyVisible() {
  if (assets.value.length === 0) {
    ElMessage.warning('没有数据可复制')
    return
  }
  const lines = assets.value.map((a) =>
    a.port ? `${a.host}:${a.port}` : a.host
  )
  navigator.clipboard.writeText(lines.join('\n'))
  ElMessage.success(`已复制 ${lines.length} 条到剪贴板`)
}

async function deleteVisible() {
  if (assets.value.length === 0) return
  try {
    await ElMessageBox.confirm(
      `将删除当前页可见的 ${assets.value.length} 条资产，建议先点复制备份`,
      '确认批量删除',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' }
    )
    await DeleteAssets(assets.value.map((a) => a.id))
    ElMessage.success('已删除')
    refresh()
  } catch { /* cancel */ }
}

async function deleteSelected() {
  if (selected.value.length === 0) return
  try {
    await ElMessageBox.confirm(
      `删除选中的 ${selected.value.length} 条资产？`,
      '确认删除',
      { type: 'warning' }
    )
    await DeleteAssets(selected.value.map((a) => a.id))
    selected.value = []
    selectedIds.value = new Set()
    refresh()
  } catch { /* cancel */ }
}

async function fetchAllAssets(): Promise<model.Asset[]> {
  const typeFilter = tab.value
  const statusFilter = getStatusFilter()
  const all = await ListAssets(projectId, typeFilter, statusFilter)
  return all || []
}

async function exportTXT() {
  const all = await fetchAllAssets()
  if (all.length === 0) {
    ElMessage.warning('没有数据可导出')
    return
  }
  const lines = all.map((a) =>
    a.port ? `${a.host}:${a.port}` : a.host
  )
  download(`${projectName}_${tab.value}.txt`, lines.join('\n'))
}

function csvEscape(v: any): string {
  const s = String(v ?? '')
  if (s.includes(',') || s.includes('"') || s.includes('\n')) {
    return '"' + s.replace(/"/g, '""') + '"'
  }
  return s
}

async function exportCSV() {
  const all = await fetchAllAssets()
  if (all.length === 0) {
    ElMessage.warning('没有数据可导出')
    return
  }
  const header = ['host', 'port', 'type', 'sources', 'status', 'status_code', 'title', 'server']
  const rows = all.map((a) => [
    a.host, a.port, a.type, (a.sources || []).join(', '),
    a.status || '', a.status_code ?? '', a.title || '', a.server || '',
  ].map(csvEscape))
  const csv = '﻿' + [header, ...rows].map((r) => r.join(',')).join('\n')
  download(`${projectName}_${tab.value}.csv`, csv)
}

function download(filename: string, content: string) {
  const blob = new Blob([content], { type: 'text/plain;charset=utf-8' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = filename
  a.click()
  URL.revokeObjectURL(a.href)
  ElMessage.success(`已导出 ${filename}`)
}

// ── 虚拟表格列定义 ────────────────────────────────────────────────
const selectedIds = ref<Set<number>>(new Set())

function toggleRowSelection(row: model.Asset, checked: any) {
  if (checked) selectedIds.value.add(row.id)
  else selectedIds.value.delete(row.id)
  selectedIds.value = new Set(selectedIds.value)
  selected.value = assets.value.filter((a) => selectedIds.value.has(a.id))
}

function toggleAll(checked: any) {
  if (checked) {
    assets.value.forEach((a) => selectedIds.value.add(a.id))
  } else {
    assets.value.forEach((a) => selectedIds.value.delete(a.id))
  }
  selectedIds.value = new Set(selectedIds.value)
  selected.value = assets.value.filter((a) => selectedIds.value.has(a.id))
}

const allChecked = computed(() => {
  if (assets.value.length === 0) return false
  return assets.value.every((a) => selectedIds.value.has(a.id))
})

// 列宽（可拖拽调整 + 持久化到 settings 表）
const COL_WIDTH_KEY = 'asset_table_col_widths'
const defaultWidths: Record<string, number> = {
  sel: 44,
  host: 320,
  port: 80,
  sources: 120,
  tags: 140,
  status: 90,
  status_code: 80,
  title: 280,
  server: 140,
  probed_at: 160,
}
const colWidths = ref<Record<string, number>>({ ...defaultWidths })

async function loadColWidths() {
  try {
    const raw = await GetSetting(COL_WIDTH_KEY)
    if (raw) {
      const saved = JSON.parse(raw)
      colWidths.value = { ...defaultWidths, ...saved }
    }
  } catch { /* ignore */ }
}

let saveTimer: number | null = null
function saveColWidths() {
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = window.setTimeout(() => {
    SetSetting(COL_WIDTH_KEY, JSON.stringify(colWidths.value))
  }, 300)
}

function onColumnResize(col: any, width: number) {
  if (col?.key) {
    colWidths.value[col.key] = width
    saveColWidths()
  }
}

function toggleSort(key: string) {
  const idx = sorts.value.findIndex(s => s.key === key)
  if (idx === -1) {
    // 新列，加入排序末尾
    sorts.value.push({ key, order: 'asc' })
  } else if (sorts.value[idx].order === 'asc') {
    // 已有 ASC → 改 DESC
    sorts.value[idx].order = 'desc'
  } else {
    // 已有 DESC → 移除
    sorts.value.splice(idx, 1)
  }
  sorts.value = [...sorts.value]
  page.value = 1
  fetchPage()
}

function clearSorts() {
  sorts.value = []
  page.value = 1
  fetchPage()
}

function sortIndex(key: string): number {
  return sorts.value.findIndex(s => s.key === key)
}

const SORTABLE_COLS = new Set(['host', 'port', 'status', 'status_code', 'title', 'server', 'probed_at'])

// 自定义可拖拽表头：在 title 右侧叠加一个 resize handle
function makeResizableHeader(key: string, title: string) {
  return () => {
    const sortable = SORTABLE_COLS.has(key)
    const si = sortIndex(key)
    const sortObj = si >= 0 ? sorts.value[si] : null
    const children = [
      h('span', {
        class: ['th-title', sortable ? 'th-sortable' : ''],
        onClick: sortable ? () => toggleSort(key) : undefined,
      }, [
        title,
        sortable ? h('span', { class: 'th-sort-icons' }, [
          h('span', { class: ['th-sort-arrow', 'up', sortObj?.order === 'asc' ? 'active' : ''] }, '▲'),
          h('span', { class: ['th-sort-arrow', 'down', sortObj?.order === 'desc' ? 'active' : ''] }, '▼'),
        ]) : null,
        sorts.value.length > 1 && si >= 0 ? h('span', { class: 'th-sort-badge' }, String(si + 1)) : null,
      ]),
      h('div', {
        class: 'th-resize-handle',
        onMousedown: (e: MouseEvent) => startResize(e, key),
      }),
    ]
    return h('div', { class: 'resizable-th' }, children)
  }
}

function startResize(e: MouseEvent, key: string) {
  e.preventDefault()
  e.stopPropagation()
  const startX = e.clientX
  const startW = colWidths.value[key] || 100

  const onMove = (ev: MouseEvent) => {
    const delta = ev.clientX - startX
    const newW = Math.max(50, startW + delta)
    colWidths.value[key] = newW
  }
  const onUp = () => {
    window.removeEventListener('mousemove', onMove)
    window.removeEventListener('mouseup', onUp)
    saveColWidths()
  }
  window.addEventListener('mousemove', onMove)
  window.addEventListener('mouseup', onUp)
}

const columns = computed<any[]>(() => [
  {
    key: 'sel',
    width: colWidths.value.sel,
    cellRenderer: ({ rowData }: { rowData: model.Asset }) => h(ElCheckbox, {
      modelValue: selectedIds.value.has(rowData.id),
      'onUpdate:modelValue': (v: any) => toggleRowSelection(rowData, v),
    }),
    headerCellRenderer: () => h(ElCheckbox, {
      modelValue: allChecked.value,
      'onUpdate:modelValue': (v: any) => toggleAll(v),
    }),
  },
  {
    key: 'host', dataKey: 'host', title: 'host', width: colWidths.value.host,
    headerCellRenderer: makeResizableHeader('host', 'host'),
    cellRenderer: ({ rowData }: { rowData: model.Asset }) => h('a', {
      class: 'host-link',
      title: '单击复制 / 双击在浏览器打开',
      onClick: () => copyHost(rowData),
      onDblclick: () => openInBrowser(rowData),
    }, rowData.host),
  },
  {
    key: 'port', dataKey: 'port', title: 'port', width: colWidths.value.port,
    headerCellRenderer: makeResizableHeader('port', 'port'),
  },
  {
    key: 'sources', title: '来源', width: colWidths.value.sources,
    headerCellRenderer: makeResizableHeader('sources', '来源'),
    cellRenderer: ({ rowData }: { rowData: model.Asset }) => h('div', {},
      (rowData.sources || []).slice(0, 2).map((s) =>
        h(ElTag, { size: 'small', style: 'margin-right:4px' }, () => s)
      )
    ),
  },
  {
    key: 'tags', title: '标签', width: colWidths.value.tags,
    headerCellRenderer: makeResizableHeader('tags', '标签'),
    cellRenderer: ({ rowData }: { rowData: model.Asset }) => h('div', { style: 'display:flex;flex-wrap:wrap;gap:3px' },
      (rowData.tags || []).map((t) =>
        h(ElTag, {
          size: 'small',
          type: (TAG_COLORS[t] || undefined) as any,
          closable: true,
          onClose: () => removeTag(rowData.id, t),
          style: 'cursor:pointer',
        }, () => t)
      )
    ),
  },
  {
    key: 'status', title: '状态', width: colWidths.value.status,
    headerCellRenderer: makeResizableHeader('status', '状态'),
    cellRenderer: ({ rowData }: { rowData: model.Asset }) => h(ElTag, {
      type: statusType(rowData.status),
      size: 'small',
    }, () => statusText(rowData.status)),
  },
  {
    key: 'status_code', dataKey: 'status_code', title: '状态码', width: colWidths.value.status_code,
    headerCellRenderer: makeResizableHeader('status_code', '状态码'),
  },
  {
    key: 'title', dataKey: 'title', title: 'title', width: colWidths.value.title,
    headerCellRenderer: makeResizableHeader('title', 'title'),
  },
  {
    key: 'server', dataKey: 'server', title: 'server', width: colWidths.value.server,
    headerCellRenderer: makeResizableHeader('server', 'server'),
  },
  {
    key: 'probed_at', dataKey: 'probed_at', title: '探活时间', width: colWidths.value.probed_at,
    headerCellRenderer: makeResizableHeader('probed_at', '探活时间'),
  },
])

onMounted(async () => {
  await loadColWidths()
  refresh()
})
</script>

<template>
  <div class="page">
    <!-- 顶部：返回 + 项目名 + 搜索 + 操作 -->
    <div class="header">
      <el-button :icon="ArrowLeft" link @click="router.back()" class="back-btn" />
      <span class="project-title">{{ projectName }}</span>
      <span class="muted">#{{ projectId }}</span>
      <div class="spacer" />
      <el-input
        v-model="search"
        :prefix-icon="Search"
        placeholder="搜索 host / 状态码 / title / server …"
        clearable
        class="header-search"
        @input="onSearchInput"
        @clear="() => { page = 1; fetchPage() }"
      />
      <el-button :icon="Document" size="small" @click="noteVisible = true">笔记</el-button>
      <el-tooltip :content="theme === 'system' ? '跟随系统' : theme === 'dark' ? '暗色模式' : '亮色模式'" placement="bottom">
        <el-button :icon="theme === 'system' ? Monitor : theme === 'dark' ? Moon : Sunny" circle size="small" @click="cycleTheme" />
      </el-tooltip>
    </div>

    <!-- 统计卡片 -->
    <div class="stats-row">
      <div class="stat-card stat-total">
        <div class="stat-num">{{ stats.total }}</div>
        <div class="stat-label">总资产</div>
      </div>
      <div class="stat-card stat-ip">
        <div class="stat-num">{{ stats.ip }}</div>
        <div class="stat-label">IP</div>
      </div>
      <div class="stat-card stat-domain">
        <div class="stat-num">{{ stats.domain }}</div>
        <div class="stat-label">域名</div>
      </div>
      <div class="stat-card stat-alive">
        <div class="stat-num">{{ stats.alive }}</div>
        <div class="stat-label">存活</div>
      </div>
      <div class="stat-card stat-ports">
        <div class="stat-num">{{ stats.ports }}</div>
        <div class="stat-label">端口</div>
      </div>
    </div>

    <!-- 工具栏：直接图标按钮 -->
    <div class="toolbar">
      <el-button :icon="Upload" @click="importVisible = true">导入</el-button>
      <el-button :icon="Share" @click="subdomainVisible = true">子域名</el-button>
      <el-divider direction="vertical" />
      <el-button :icon="Aim" @click="dnsVisible = true">DNS</el-button>
      <el-button :icon="Aim" @click="httpxVisible = true">探活</el-button>
      <el-button :icon="Connection" @click="rustscanVisible = true">rustscan</el-button>
      <el-button :icon="Connection" @click="naabuVisible = true">naabu</el-button>
    </div>

    <!-- 过滤 + 操作栏 -->
    <div class="filter-bar">
      <el-tabs v-model="tab" class="inline-tabs">
        <el-tab-pane :label="tabLabel('ip')" name="ip" />
        <el-tab-pane :label="tabLabel('domain')" name="domain" />
      </el-tabs>
      <el-select v-model="filterMode" size="small" style="width: 140px">
        <el-option label="全部状态" value="all" />
        <el-option label="仅 HTTP 存活" value="alive" />
        <el-option label="仅非 HTTP 服务" value="non-http" />
        <el-option label="仅未探活" value="unprobed" />
        <el-option label="仅 dead" value="dead" />
      </el-select>
      <el-select v-if="tab === 'ip'" v-model="networkFilter" size="small" style="width: 120px">
        <el-option label="全部网段" value="all" />
        <el-option label="内网 IP" value="intranet" />
        <el-option label="外网 IP" value="extranet" />
      </el-select>
      <el-button v-if="sorts.length > 0" size="small" link type="info" @click="clearSorts">
        清除排序 ({{ sorts.length }})
      </el-button>
      <div class="spacer" />
      <el-button size="small" :icon="DocumentCopy" @click="copyVisible">复制</el-button>
      <el-button size="small" :icon="Download" @click="exportTXT">TXT</el-button>
      <el-button size="small" :icon="Download" @click="exportCSV">CSV</el-button>
      <el-button size="small" :icon="Delete" type="danger" plain @click="deleteVisible">删除</el-button>
    </div>

    <!-- 选中后的批量操作条 -->
    <div v-if="selected.length > 0" class="batch-bar">
      <span class="batch-count">已选中 {{ selected.length }} 条</span>
      <el-dropdown trigger="click" @command="(tag: string) => { if (tag !== '__custom__') BatchAddTag(selected.map(a=>a.id), tag).then(refresh) }">
        <el-button size="small" type="primary">打标签</el-button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item v-for="t in PRESET_TAGS" :key="t" :command="t">{{ t }}</el-dropdown-item>
            <el-dropdown-item divided command="__custom__" @click="batchTag">自定义…</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
      <el-button size="small" type="danger" :icon="Delete" @click="deleteSelected">删除选中</el-button>
      <el-button size="small" link @click="selected = []; selectedIds = new Set()">取消选择</el-button>
    </div>

    <!-- 表格 -->
    <div class="table-wrap" v-loading="loading">
      <el-auto-resizer>
        <template #default="{ height, width }">
          <el-table-v2
            :data="assets"
            :columns="columns"
            :width="width"
            :height="height"
            :row-height="44"
            :header-height="40"
            fixed
          />
        </template>
      </el-auto-resizer>
    </div>

    <!-- 分页 -->
    <div class="pagination-bar">
      <el-pagination
        v-model:current-page="page"
        v-model:page-size="pageSize"
        :total="totalCount"
        :page-sizes="[20, 50, 100, 200]"
        layout="total, sizes, prev, pager, next, jumper"
        background
        @current-change="fetchPage"
        @size-change="onPageSizeChange"
      />
    </div>

    <ImportDialog v-model:visible="importVisible" :project-id="projectId" @imported="refresh" />
    <HttpxDialog v-model:visible="httpxVisible" :project-id="projectId" @probed="refresh" />
    <RustscanDialog v-model:visible="rustscanVisible" :project-id="projectId" @scanned="refresh" />
    <NaabuDialog v-model:visible="naabuVisible" :project-id="projectId" @scanned="refresh" />
    <SubdomainDialog v-model:visible="subdomainVisible" :project-id="projectId" @discovered="refresh" />
    <NoteDialog v-model:visible="noteVisible" :project-id="projectId" :project-name="projectName" />
    <DnsDialog v-model:visible="dnsVisible" :project-id="projectId" @resolved="refresh" />
  </div>
</template>

<style scoped>
.page {
  height: 100vh;
  padding: 14px 18px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  box-sizing: border-box;
}

/* ── 顶部栏 ────────────────────────────────── */
.header {
  display: flex;
  align-items: center;
  gap: 8px;
}
.project-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary);
}
.back-btn {
  font-size: 18px;
  color: var(--text-muted);
}
.back-btn:hover {
  color: var(--text-primary);
}
.muted {
  color: var(--text-dimmed);
  font-size: 12px;
}
.header-search {
  width: 260px;
}
.spacer {
  flex: 1;
}

/* ── 统计卡片 ──────────────────────────────── */
.stats-row {
  display: flex;
  gap: 12px;
}
.stat-card {
  flex: 1;
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 10px 14px;
  text-align: center;
  box-shadow: var(--shadow-sm);
  border-top: 3px solid transparent;
  transition: box-shadow 0.2s;
}
.stat-card:hover {
  box-shadow: var(--shadow-md);
}
.stat-total { border-top-color: var(--text-primary); }
.stat-ip    { border-top-color: var(--accent); }
.stat-domain{ border-top-color: var(--info-purple); }
.stat-alive { border-top-color: var(--success); }
.stat-ports { border-top-color: var(--warning); }
.stat-num {
  font-size: 20px;
  font-weight: 700;
  line-height: 1.2;
}
.stat-total .stat-num { color: var(--text-primary); }
.stat-ip .stat-num    { color: var(--accent); }
.stat-domain .stat-num{ color: var(--info-purple); }
.stat-alive .stat-num { color: var(--success); }
.stat-ports .stat-num { color: var(--warning); }
.stat-label {
  font-size: 12px;
  color: var(--text-muted);
  margin-top: 2px;
}

/* ── 工具栏 ────────────────────────────────── */
.toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.toolbar :deep(.el-divider--vertical) {
  margin: 0 4px;
  border-left-color: var(--border);
}

/* ── 过滤栏 ────────────────────────────────── */
.filter-bar {
  display: flex;
  align-items: center;
  gap: 10px;
}
.inline-tabs {
  margin-bottom: -2px;
}
:deep(.inline-tabs .el-tabs__header) {
  margin: 0;
}
:deep(.inline-tabs .el-tabs__item) {
  color: var(--text-muted);
  font-size: 13px;
  height: 32px;
  line-height: 32px;
}
:deep(.inline-tabs .el-tabs__item:hover) {
  color: var(--text-regular);
}
:deep(.inline-tabs .el-tabs__item.is-active) {
  color: var(--accent);
  font-weight: 600;
}
:deep(.inline-tabs .el-tabs__active-bar) {
  background-color: var(--accent);
  height: 2px;
}
:deep(.inline-tabs .el-tabs__nav-wrap::after) {
  background-color: transparent;
}

/* ── 批量操作条 ────────────────────────────── */
.batch-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 6px 14px;
  background: var(--bg-surface);
  border: 1px solid var(--border);
  border-left: 3px solid var(--accent);
  border-radius: 6px;
  font-size: 13px;
}
.batch-count {
  color: var(--accent);
  font-weight: 600;
}

/* ── 表格 ──────────────────────────────────── */
.table-wrap {
  flex: 1;
  border: 1px solid var(--border);
  border-radius: 8px;
  overflow: hidden;
  background: var(--bg-card);
  box-shadow: var(--shadow-sm);
}
.host-link {
  color: var(--accent-hover);
  cursor: pointer;
  text-decoration: none;
}
.host-link:hover {
  text-decoration: underline;
}

/* ── 虚拟表格深色主题 ───────────────────── */
:deep(.el-table-v2),
:deep(.el-table-v2__root),
:deep(.el-table-v2__main),
:deep(.el-table-v2__body),
:deep(.el-table-v2 [role="grid"]) {
  background-color: var(--bg-card) !important;
}
:deep(.el-table-v2__header),
:deep(.el-table-v2__header-row),
:deep(.el-table-v2__header-cell) {
  background-color: var(--bg-surface) !important;
  color: var(--text-regular);
  font-weight: 600;
}
:deep(.el-table-v2__row),
:deep(.el-table-v2__row-cell) {
  background-color: var(--bg-card) !important;
  color: var(--text-primary);
  border-bottom: 1px solid var(--border-light);
}
:deep(.el-table-v2__row:hover),
:deep(.el-table-v2__row:hover .el-table-v2__row-cell) {
  background-color: var(--border-light) !important;
}
:deep(.el-table-v2__empty) {
  background-color: var(--bg-card) !important;
  color: var(--text-dimmed);
}
:deep(.el-vl__wrapper),
:deep(.el-virtual-scrollbar),
:deep(.el-table-v2__body .el-vl__wrapper > div) {
  background-color: var(--bg-card) !important;
}

/* 列分隔线 */
:deep(.el-table-v2__header-cell) {
  position: relative;
  border-right: 1px solid var(--border);
  padding: 0 !important;
}
:deep(.el-table-v2__header-cell:last-child) {
  border-right: none;
}
:deep(.el-table-v2__row-cell) {
  border-right: 1px solid var(--border-light);
}
:deep(.el-table-v2__row-cell:last-child) {
  border-right: none;
}

/* 可拖拽表头 */
:deep(.resizable-th) {
  position: relative;
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  padding: 0 12px;
  user-select: none;
  box-sizing: border-box;
}
:deep(.th-title) {
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  display: flex;
  align-items: center;
  gap: 4px;
}
:deep(.th-sortable) {
  cursor: pointer;
}
:deep(.th-sortable:hover) {
  color: var(--text-primary);
}
:deep(.th-sort-icons) {
  display: inline-flex;
  flex-direction: column;
  line-height: 1;
  gap: 0;
  font-size: 8px;
  margin-left: 2px;
  flex-shrink: 0;
}
:deep(.th-sort-arrow) {
  color: var(--text-dimmed);
  line-height: 1;
  transition: color 0.15s;
}
:deep(.th-sort-arrow.active) {
  color: var(--accent);
}
:deep(.th-sort-badge) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 14px;
  height: 14px;
  font-size: 9px;
  font-weight: 700;
  color: #fff;
  background: var(--accent);
  border-radius: 50%;
  margin-left: 3px;
  line-height: 1;
}
:deep(.th-resize-handle) {
  position: absolute;
  right: -3px;
  top: 0;
  bottom: 0;
  width: 8px;
  cursor: col-resize;
  z-index: 10;
  background: transparent;
  transition: background-color 0.15s;
}
:deep(.th-resize-handle:hover),
:deep(.th-resize-handle:active) {
  background-color: color-mix(in srgb, var(--accent) 50%, transparent);
}

/* ── 分页条 ────────────────────────────────── */
.pagination-bar {
  display: flex;
  justify-content: center;
  padding: 6px 0 2px;
  flex-shrink: 0;
}
:deep(.el-pagination) {
  --el-pagination-bg-color: var(--bg-card);
  --el-pagination-text-color: var(--text-regular);
  --el-pagination-button-color: var(--text-regular);
  --el-pagination-button-bg-color: var(--bg-surface);
  --el-pagination-button-disabled-color: var(--text-dimmed);
  --el-pagination-button-disabled-bg-color: var(--bg-card);
  --el-pagination-hover-color: var(--accent);
}
:deep(.el-pagination.is-background .el-pager li) {
  background-color: var(--bg-surface);
  color: var(--text-regular);
  border: 1px solid var(--border);
}
:deep(.el-pagination.is-background .el-pager li:hover) {
  color: var(--accent);
}
:deep(.el-pagination.is-background .el-pager li.is-active) {
  background-color: var(--accent);
  color: #fff;
  border-color: var(--accent);
}
:deep(.el-pagination.is-background .btn-prev),
:deep(.el-pagination.is-background .btn-next) {
  background-color: var(--bg-surface);
  color: var(--text-regular);
  border: 1px solid var(--border);
}
:deep(.el-pagination .el-select .el-input .el-input__wrapper) {
  background-color: var(--bg-surface);
  border-color: var(--border);
  box-shadow: none;
  color: var(--text-regular);
}
:deep(.el-pagination .el-pagination__editor .el-input__wrapper) {
  background-color: var(--bg-surface);
  border-color: var(--border);
  box-shadow: none;
  color: var(--text-regular);
}
</style>
