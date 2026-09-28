<!-- ============================================================
     管理端出口（v2.2）：管理菜单已并入 AppShell 主侧栏，
     此层只保留子页出口与轻转场；鉴权由路由守卫保证。
     各子页自带 page-head（标题 + 描述 + 操作），避免与布局层重复。
     ============================================================ -->
<template>
  <RouterView v-slot="{ Component, route: r }">
    <Transition name="page-sub" mode="out-in">
      <component :is="Component" :key="r.path" />
    </Transition>
  </RouterView>
</template>
