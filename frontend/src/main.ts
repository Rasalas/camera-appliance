import { createApp } from 'vue'
import '@fontsource/instrument-serif/400.css'
import '@fontsource/instrument-serif/400-italic.css'
import '@fontsource-variable/jetbrains-mono'
import App from './App.vue'
import router from './router'
import './styles/base.css'

createApp(App).use(router).mount('#app')
