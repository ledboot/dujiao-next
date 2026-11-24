<script setup lang="ts">
import { RouterView } from 'vue-router'
import NoticeHost from '@/components/NoticeHost.vue'
import ConfirmDialogHost from '@/components/ConfirmDialogHost.vue'
</script>

<template>
  <RouterView v-slot="{ Component }">
    <Transition name="admin-page" mode="out-in">
      <component :is="Component" />
    </Transition>
  </RouterView>
  <NoticeHost />
  <ConfirmDialogHost />
</template>
