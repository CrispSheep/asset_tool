import { ref, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'
import { PauseJob, ResumeJob, CancelJob } from '../../wailsjs/go/main/App'

export function useScannerDialog(visible: { value: boolean }) {
  const running = ref(false)
  const paused = ref(false)
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

  function startTimer() {
    startedAt.value = Date.now()
    elapsed.value = '00:00'
    if (timerHandle) clearInterval(timerHandle)
    timerHandle = window.setInterval(() => {
      elapsed.value = fmt(Date.now() - startedAt.value)
    }, 1000)
  }

  function stopTimer() {
    if (timerHandle) { clearInterval(timerHandle); timerHandle = null }
  }

  function resetLog() {
    log.value = []
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
      appendLog('[i] 已暂停')
    }
  }

  function stop() {
    if (jobId.value) CancelJob(jobId.value)
  }

  function close() {
    if (running.value) {
      ElMessage.warning('任务进行中，可点「收起」后台运行，或先停止')
      return
    }
    visible.value = false
  }

  function hide() {
    visible.value = false
  }

  onBeforeUnmount(() => {
    stopTimer()
  })

  return {
    running, paused, log, logEl, jobId, elapsed,
    appendLog, startTimer, stopTimer, resetLog,
    togglePause, stop, close, hide,
  }
}
