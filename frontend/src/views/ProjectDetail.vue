<script setup lang="ts">
import { ref, computed, onMounted, watch, h } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox, ElTag, ElCheckbox } from 'element-plus'
import {
  ArrowLeft, Upload, Search, Delete, DocumentCopy, Download, Aim, Connection, Share, Document,
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

const route = useRoute()
const router = useRouter()
const projectId = Number(route.params.id)
const projectName = (route.query.name as string) || `项目 #${projectId}`

const tab = ref<'ip' | 'domain'>('ip')
const assets = ref<model.Asset[]>([])
const totalCount = ref(0)
const loading = ref(false)

const search = ref('')
const filterMode = ref<'all' | 'alive' | 'non-http' | 'unprobed' | 'dead'>('all')
const networkFilter = ref<'all' | 'intranet' | 'extranet'>('all')
const page = ref(1)
const pageSize = ref(50)
const sortBy = ref('')
const sortOrder = ref<'asc' | 'desc'>('asc')

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
    case 'non-http': return 'dead'
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
      sortBy.value,
      sortOrder.value,
      page.value,
      pageSize.value,
    )
    let items = res.items || []
    if (filterMode.value === 'non-http') {
      const httpPorts = new Set(['', '80', '443', '8080', '8443', '8000', '8888'])
      items = items.filter(a => !httpPorts.has(a.port || ''))
    }
    assets.value = items
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
  let items = all || []
  if (filterMode.value === 'non-http') {
    const httpPorts = new Set(['', '80', '443', '8080', '8443', '8000', '8888'])
    items = items.filter(a => !httpPorts.has(a.port || ''))
  }
  return items
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

async function exportCSV() {
  const all = await fetchAllAssets()
  if (all.length === 0) {
    ElMessage.warning('没有数据可导出')
    return
  }
  const header = ['host', 'port', 'type', 'sources', 'status', 'status_code', 'title', 'server']
  const rows = all.map((a) => [
    a.host, a.port, a.type, (a.sources || []).join(', '),
    a.status || '', a.status_code ?? '', (a.title || '').replace(/,/g, ' '), a.server || '',
  ])
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
  if (sortBy.value === key) {
    if (sortOrder.value === 'asc') {
      sortOrder.value = 'desc'
    } else {
      sortBy.value = ''
      sortOrder.value = 'asc'
    }
  } else {
    sortBy.value = key
    sortOrder.value = 'asc'
  }
  page.value = 1
  fetchPage()
}

const SORTABLE_COLS = new Set(['host', 'port', 'status', 'status_code', 'title', 'server', 'probed_at'])

// 自定义可拖拽表头：在 title 右侧叠加一个 resize handle
function makeResizableHeader(key: string, title: string) {
  return () => {
    const sortable = SORTABLE_COLS.has(key)
    const isActive = sortBy.value === key
    const children = [
      h('span', {
        class: ['th-title', sortable ? 'th-sortable' : ''],
        onClick: sortable ? () => toggleSort(key) : undefined,
      }, [
        title,
        sortable ? h('span', { class: 'th-sort-icons' }, [
          h('span', { class: ['th-sort-arrow', 'up', isActive && sortOrder.value === 'asc' ? 'active' : ''] }, '▲'),
          h('span', { class: ['th-sort-arrow', 'down', isActive && sortOrder.value === 'desc' ? 'active' : ''] }, '▼'),
        ]) : null,
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
    <!-- 顶部导航 -->
    <div class="header">
      <el-button :icon="ArrowLeft" @click="router.back()">返回</el-button>
      <h2>{{ projectName }}</h2>
      <span class="muted">#{{ projectId }}</span>
      <div class="spacer" />
    </div>

    <!-- 操作栏 -->
    <div class="toolbar">
      <el-button :icon="Upload" @click="importVisible = true">导入资产</el-button>
      <el-button :icon="Share" @click="subdomainVisible = true">子域名探测</el-button>
      <el-button :icon="Aim" type="warning" plain @click="dnsVisible = true">DNS 解析</el-button>
      <el-button :icon="Aim" type="primary" plain @click="httpxVisible = true">探活 (httpx)</el-button>
      <el-button :icon="Connection" type="primary" plain @click="rustscanVisible = true">端口扫描 (rustscan)</el-button>
      <el-button :icon="Connection" type="primary" plain @click="naabuVisible = true">端口扫描 (naabu)</el-button>
      <el-button :icon="Document" @click="noteVisible = true">📝 笔记</el-button>
      <div class="spacer" />
      <el-select v-model="filterMode" style="width: 180px">
        <el-option label="全部状态" value="all" />
        <el-option label="仅 HTTP 存活" value="alive" />
        <el-option label="仅非 HTTP 服务" value="non-http" />
        <el-option label="仅未探活" value="unprobed" />
        <el-option label="仅 dead" value="dead" />
      </el-select>
      <el-select v-if="tab === 'ip'" v-model="networkFilter" style="width: 140px">
        <el-option label="全部网段" value="all" />
        <el-option label="内网 IP" value="intranet" />
        <el-option label="外网 IP" value="extranet" />
      </el-select>
      <el-button :icon="DocumentCopy" @click="copyVisible">复制可见</el-button>
      <el-button :icon="Download" @click="exportTXT">导出 TXT</el-button>
      <el-button :icon="Download" @click="exportCSV">导出 CSV</el-button>
      <el-button :icon="Delete" type="danger" plain @click="deleteVisible">删除可见</el-button>
    </div>

    <!-- 统计仪表盘 -->
    <div class="stats-bar">
      <div class="stat-card">
        <span class="stat-num">{{ stats.total }}</span>
        <span class="stat-label">总资产</span>
      </div>
      <div class="stat-card stat-ip">
        <span class="stat-num">{{ stats.ip }}</span>
        <span class="stat-label">IP</span>
      </div>
      <div class="stat-card stat-domain">
        <span class="stat-num">{{ stats.domain }}</span>
        <span class="stat-label">域名</span>
      </div>
      <div class="stat-card stat-alive">
        <span class="stat-num">{{ stats.alive }}</span>
        <span class="stat-label">存活</span>
      </div>
      <div class="stat-card stat-dead">
        <span class="stat-num">{{ stats.dead }}</span>
        <span class="stat-label">死亡</span>
      </div>
      <div class="stat-card stat-unprobed">
        <span class="stat-num">{{ stats.unprobed }}</span>
        <span class="stat-label">未探活</span>
      </div>
      <div class="stat-card stat-ports">
        <span class="stat-num">{{ stats.ports }}</span>
        <span class="stat-label">开放端口</span>
      </div>
    </div>

    <!-- 搜索 -->
    <el-input
      v-model="search"
      :prefix-icon="Search"
      placeholder="搜索 host / 状态码 / title / server …"
      clearable
      @input="onSearchInput"
      @clear="() => { page = 1; fetchPage() }"
    />

    <!-- 选中后的批量操作条 -->
    <div v-if="selected.length > 0" class="batch-bar">
      已选中 {{ selected.length }} 条
      <el-dropdown trigger="click" @command="(tag: string) => { BatchAddTag(selected.map(a=>a.id), tag).then(refresh) }">
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

    <!-- Tabs + 表格 -->
    <el-tabs v-model="tab" class="tabs-area">
      <el-tab-pane :label="tabLabel('ip')" name="ip" />
      <el-tab-pane :label="tabLabel('domain')" name="domain" />
    </el-tabs>

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

    <ImportDialog
      v-model:visible="importVisible"
      :project-id="projectId"
      @imported="refresh"
    />

    <HttpxDialog
      v-model:visible="httpxVisible"
      :project-id="projectId"
      @probed="refresh"
    />

    <RustscanDialog
      v-model:visible="rustscanVisible"
      :project-id="projectId"
      @scanned="refresh"
    />

    <NaabuDialog
      v-model:visible="naabuVisible"
      :project-id="projectId"
      @scanned="refresh"
    />

    <SubdomainDialog
      v-model:visible="subdomainVisible"
      :project-id="projectId"
      @discovered="refresh"
    />

    <NoteDialog
      v-model:visible="noteVisible"
      :project-id="projectId"
      :project-name="projectName"
    />

    <DnsDialog
      v-model:visible="dnsVisible"
      :project-id="projectId"
      @resolved="refresh"
    />
  </div>
</template>

<style scoped>
.page {
  height: 100vh;
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  box-sizing: border-box;
}
.header {
  display: flex;
  align-items: center;
  gap: 12px;
}
.header h2 {
  margin: 0;
  color: #fff;
  font-size: 18px;
}
.muted {
  color: #6c7080;
  font-size: 13px;
}
.spacer {
  flex: 1;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.batch-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 6px 12px;
  background: #25282f;
  border-radius: 6px;
  color: #1890ff;
}
.tabs-area {
  margin-bottom: -8px;
}
:deep(.el-tabs__item) {
  color: #b8b8b8;
  font-size: 14px;
}
:deep(.el-tabs__item:hover) {
  color: #ffffff;
}
:deep(.el-tabs__item.is-active) {
  color: #1890ff;
  font-weight: 600;
}
:deep(.el-tabs__active-bar) {
  background-color: #1890ff;
}
:deep(.el-tabs__nav-wrap::after) {
  background-color: #3a3e4a;
}
.asset-table {
  flex: 1;
  background: transparent !important;
}
.table-wrap {
  flex: 1;
  border: 1px solid #3a3e4a;
  border-radius: 6px;
  overflow: hidden;
  background: #25282f;
}
.host-link {
  color: #40a9ff;
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
  background-color: #25282f !important;
}
:deep(.el-table-v2__header),
:deep(.el-table-v2__header-row),
:deep(.el-table-v2__header-cell) {
  background-color: #2a2d36 !important;
  color: #b8b8b8;
  font-weight: 600;
}
:deep(.el-table-v2__row),
:deep(.el-table-v2__row-cell) {
  background-color: #25282f !important;
  color: #e8e8e8;
  border-bottom: 1px solid #2f323a;
}
:deep(.el-table-v2__row:hover),
:deep(.el-table-v2__row:hover .el-table-v2__row-cell) {
  background-color: #2f323a !important;
}
:deep(.el-table-v2__empty) {
  background-color: #25282f !important;
  color: #6c7080;
}
:deep(.el-vl__wrapper),
:deep(.el-virtual-scrollbar),
:deep(.el-table-v2__body .el-vl__wrapper > div) {
  background-color: #25282f !important;
}

/* 列分隔线（默认细线，hover 加粗变蓝） */
:deep(.el-table-v2__header-cell) {
  position: relative;
  border-right: 1px solid #3a3e4a;
  padding: 0 !important;
}
:deep(.el-table-v2__header-cell:last-child) {
  border-right: none;
}
:deep(.el-table-v2__row-cell) {
  border-right: 1px solid #2f323a;
}
:deep(.el-table-v2__row-cell:last-child) {
  border-right: none;
}

/* 自定义可拖拽表头 */
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
  color: #e8e8e8;
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
  color: #555;
  line-height: 1;
  transition: color 0.15s;
}
:deep(.th-sort-arrow.active) {
  color: #1890ff;
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
  background-color: rgba(24, 144, 255, 0.5);
}
/* ── 表格深色主题（覆盖 Element Plus 默认） ───────────────── */
:deep(.el-table),
:deep(.el-table__inner-wrapper),
:deep(.el-table__body-wrapper),
:deep(.el-table__header-wrapper),
:deep(.el-table tr),
:deep(.el-table th.el-table__cell),
:deep(.el-table td.el-table__cell) {
  background-color: #25282f !important;
  color: #e8e8e8;
}
:deep(.el-table) {
  --el-table-bg-color: #25282f;
  --el-table-tr-bg-color: #25282f;
  --el-table-header-bg-color: #2a2d36;
  --el-table-row-hover-bg-color: #2f323a;
  --el-table-border-color: #3a3e4a;
  --el-table-header-text-color: #b8b8b8;
  --el-table-text-color: #e8e8e8;
  --el-table-fixed-box-shadow: none;
  border: 1px solid #3a3e4a;
  border-radius: 6px;
}
:deep(.el-table th.el-table__cell) {
  background-color: #2a2d36 !important;
  color: #b8b8b8 !important;
  font-weight: 600;
  border-bottom: 1px solid #3a3e4a;
}
:deep(.el-table tr:hover > td.el-table__cell) {
  background-color: #2f323a !important;
}
:deep(.el-table--enable-row-hover .el-table__body tr:hover > td.el-table__cell) {
  background-color: #2f323a !important;
}
:deep(.el-table__empty-block),
:deep(.el-table__empty-text) {
  background-color: #25282f !important;
  color: #6c7080;
}

/* ── 统计仪表盘 ───────────────────────────────────────── */
.stats-bar {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}
.stat-card {
  flex: 1;
  min-width: 100px;
  padding: 12px 16px;
  background: #25282f;
  border: 1px solid #3a3e4a;
  border-radius: 8px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  transition: border-color 0.15s;
}
.stat-card:hover {
  border-color: #4a4e5a;
}
.stat-num {
  font-size: 22px;
  font-weight: 700;
  color: #ffffff;
  line-height: 1;
}
.stat-label {
  font-size: 12px;
  color: #8c8c8c;
}
.stat-ip .stat-num { color: #1890ff; }
.stat-domain .stat-num { color: #722ed1; }
.stat-alive .stat-num { color: #52c41a; }
.stat-dead .stat-num { color: #ff4d4f; }
.stat-unprobed .stat-num { color: #8c8c8c; }
.stat-ports .stat-num { color: #faad14; }

/* ── 分页条 ───────────────────────────────────────── */
.pagination-bar {
  display: flex;
  justify-content: center;
  padding: 8px 0 4px;
  flex-shrink: 0;
}
:deep(.el-pagination) {
  --el-pagination-bg-color: #25282f;
  --el-pagination-text-color: #b8b8b8;
  --el-pagination-button-color: #b8b8b8;
  --el-pagination-button-bg-color: #2a2d36;
  --el-pagination-button-disabled-color: #555;
  --el-pagination-button-disabled-bg-color: #25282f;
  --el-pagination-hover-color: #1890ff;
}
:deep(.el-pagination.is-background .el-pager li) {
  background-color: #2a2d36;
  color: #b8b8b8;
  border: 1px solid #3a3e4a;
}
:deep(.el-pagination.is-background .el-pager li:hover) {
  color: #1890ff;
}
:deep(.el-pagination.is-background .el-pager li.is-active) {
  background-color: #1890ff;
  color: #fff;
  border-color: #1890ff;
}
:deep(.el-pagination.is-background .btn-prev),
:deep(.el-pagination.is-background .btn-next) {
  background-color: #2a2d36;
  color: #b8b8b8;
  border: 1px solid #3a3e4a;
}
:deep(.el-pagination .el-select .el-input .el-input__wrapper) {
  background-color: #2a2d36;
  border-color: #3a3e4a;
  box-shadow: none;
  color: #b8b8b8;
}
:deep(.el-pagination .el-pagination__editor .el-input__wrapper) {
  background-color: #2a2d36;
  border-color: #3a3e4a;
  box-shadow: none;
  color: #b8b8b8;
}
</style>
