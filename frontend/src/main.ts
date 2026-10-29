import { createApp } from 'vue'
import '@fontsource/instrument-serif/400.css'
import '@fontsource/instrument-serif/400-italic.css'
import '@fontsource-variable/jetbrains-mono'
import App from './App.vue'
import router from './router'
import './styles/base.css'
import './styles/admin.css'

const app = createApp(App).use(router)
// Resolve the initial route and its auth guard before rendering the shell.
// Otherwise a direct viewer visit briefly renders the admin navigation.
void router.isReady().then(() => app.mount('#app'))
