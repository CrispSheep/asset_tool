<script setup lang="ts">
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { ImportCSV, ManualAddAssets } from '../../wailsjs/go/main/App'

const props = defineProps<{
  projectId: number
}>()
const emit = defineEmits<{ imported: [] }>()

const visible = defineModel<boolean>('visible', { default: false })
const tab = ref<'csv' | 'manual'>('csv')

// CSV
const sourceCSV = ref('Fofa')
const csvLoading = ref(false)

// 手动
const manualText = ref('')
const sourceManual = ref('手动')
const manualLoading = ref(false)

async function pickAndImportCSV() {
  csvLoading.value = true
  try {
    const r: any = await ImportCSV(props.projectId, sourceCSV.value)
    if (r.cancelled) return
    ElMessage.success(
      `识别 ${r.detected_ip + r.detected_dm} 条 / 新增 IP ${r.new_ip} 域名 ${r.new_domain} / 跳过 ${r.skipped}`
    )
    emit('imported')
    visible.value = false
  } catch (e: any) {
    ElMessage.error('导入失败: ' + e)
  } finally {
    csvLoading.value = false
  }
}

async function submitManual() {
  const lines = manualText.value
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)
  if (lines.length === 0) {
    ElMessage.warning('请输入至少一行')
    return
  }
  manualLoading.value = true
  try {
    const r = await ManualAddAssets(props.projectId, lines, sourceManual.value)
    ElMessage.success(`新增 IP ${r.new_ip} 域名 ${r.new_domain} / 已存在 ${r.skipped}`)
    emit('imported')
    manualText.value = ''
    visible.value = false
  } catch (e: any) {
    ElMessage.error('添加失败: ' + e)
  } finally {
    manualLoading.value = false
  }
}
</script>

<template>
  <el-dialog v-model="visible" title="导入资产" width="560px" :close-on-click-modal="false">
    <el-tabs v-model="tab">
      <el-tab-pane label="CSV 文件" name="csv">
        <el-form label-width="80px">
          <el-form-item label="来源标签">
            <el-select v-model="sourceCSV" filterable allow-create>
              <el-option label="Fofa" value="Fofa" />
              <el-option label="DNSgrep" value="DNSgrep" />
              <el-option label="Hunter" value="Hunter" />
              <el-option label="Quake" value="Quake" />
              <el-option label="手动" value="手动" />
            </el-select>
          </el-form-item>
        </el-form>
        <p style="color: var(--text-muted); font-size: 13px; margin: 8px 0">
          点击下方按钮选择 CSV 文件，自动识别 IP / 域名 / URL，去重并合并已存在资产的来源标签
        </p>
      </el-tab-pane>

      <el-tab-pane label="手动添加" name="manual">
        <el-form label-width="80px">
          <el-form-item label="目标列表">
            <el-input
              v-model="manualText"
              type="textarea"
              :rows="10"
              placeholder="每行一个，支持 IP / 域名 / host:port / URL&#10;192.168.1.1&#10;example.com&#10;sub.example.com:8080&#10;https://target.com"
            />
          </el-form-item>
          <el-form-item label="来源标签">
            <el-input v-model="sourceManual" />
          </el-form-item>
        </el-form>
      </el-tab-pane>
    </el-tabs>

    <template #footer>
      <el-button @click="visible = false">关闭</el-button>
      <el-button v-if="tab === 'csv'" type="primary" :loading="csvLoading" @click="pickAndImportCSV">
        选择文件并导入
      </el-button>
      <el-button v-else type="primary" :loading="manualLoading" @click="submitManual">
        添加
      </el-button>
    </template>
  </el-dialog>
</template>
