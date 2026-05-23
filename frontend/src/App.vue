<script lang="ts" setup>
import { RouterView } from 'vue-router'
import { useTheme } from './composables/useTheme'

const { init } = useTheme()
init()
</script>

<template>
  <div class="app-root">
    <RouterView />
  </div>
</template>

<style scoped>
.app-root {
  width: 100%;
  height: 100vh;
  overflow: hidden;
}
</style>
