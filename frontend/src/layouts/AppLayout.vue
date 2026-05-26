<template>
  <div class="app-shell">
    <aside class="sidebar">
      <div class="brand">camera-appliance</div>
      <nav>
        <RouterLink to="/">Dashboard</RouterLink>
        <RouterLink to="/setup">Übersicht</RouterLink>
        <RouterLink to="/cameras">Kameras</RouterLink>
        <RouterLink to="/display">Anzeige</RouterLink>
        <RouterLink to="/settings">Einstellungen</RouterLink>
        <RouterLink to="/events">Ereignisse</RouterLink>
        <RouterLink to="/backup">Backup</RouterLink>
      </nav>
    </aside>
    <main class="content">
      <RouterView />
    </main>
  </div>
</template>
