<script setup lang="ts">
import { ref, watch, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'
import { GetSetting, SetSetting } from '../../wailsjs/go/main/App'

const props = defineProps<{ projectId: number; projectName: string }>()
const visible = defineModel<boolean>('visible', { default: false })

const content = ref('')
const savedHint = ref('')
let saveTimer: number | null = null
let hintTimer: number | null = null
let loadedKey = ''

async function loadNote() {
  const key = `note_${props.projectId}`
  if (loadedKey === key) return
  loadedKey = key
  content.value = await GetSetting(key)
}

function flushSave() {
  if (saveTimer) {
    clearTimeout(saveTimer)
    saveTimer = null
  }
  doSave()
}

async function doSave() {
  if (!props.projectId) return
  await SetSetting(`note_${props.projectId}`, content.value)
  savedHint.value = '✓ 已保存'
  if (hintTimer) clearTimeout(hintTimer)
  hintTimer = window.setTimeout(() => (savedHint.value = ''), 1500)
}

function onInput() {
  // debounce 1s 自动保存
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = window.setTimeout(doSave, 1000)
}

function onKeydown(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key === 's') {
    e.preventDefault()
    flushSave()
  }
}

watch(visible, (v) => {
  if (v) loadNote()
})

onBeforeUnmount(() => {
  if (saveTimer) {
    clearTimeout(saveTimer)
    doSave()
  }
  if (hintTimer) clearTimeout(hintTimer)
})

function close() {
  flushSave()
  visible.value = false
}
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="`📝 笔记 - ${projectName}`"
    width="55%"
    top="6vh"
    draggable
    :modal="false"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    class="note-dialog"
    append-to-body
  >
    <el-input
      v-model="content"
      type="textarea"
      :autosize="false"
      placeholder="在此记录与本项目相关的任何信息……
（如：授权范围、目标说明、漏洞线索、待办；自动保存；Ctrl+S 立即保存）"
      class="note-editor"
      resize="none"
      @input="onInput"
      @keydown="onKeydown"
    />
    <div class="hint">{{ savedHint }}</div>

    <template #footer>
      <el-button @click="flushSave">立即保存 (Ctrl+S)</el-button>
      <el-button type="primary" @click="close">关闭</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.hint {
  height: 18px;
  color: var(--success);
  font-size: 12px;
  text-align: right;
  padding-right: 4px;
  margin-top: 4px;
}
</style>

<style>
.note-dialog {
  height: 80vh;
  display: flex;
  flex-direction: column;
  margin: 0 !important;
}
.note-dialog .el-dialog__body {
  flex: 1;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  padding-top: 10px;
  padding-bottom: 0;
}
.note-dialog .note-editor {
  flex: 1;
  display: flex;
}
.note-dialog .note-editor .el-textarea__inner {
  flex: 1;
  height: 100% !important;
  background: var(--bg-log);
  color: var(--text-regular);
  font-family: Consolas, 'Microsoft YaHei UI', monospace;
  font-size: 13px;
  line-height: 1.6;
  border: 1px solid var(--border);
  border-radius: 6px;
  resize: none;
}
</style>
