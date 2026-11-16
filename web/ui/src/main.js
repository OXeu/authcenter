import { createApp } from 'vue'
import { pages } from './data.js'
import App from './App.vue'
import '../theme.css'

const source = document.querySelector('#page-data')
if (!source?.textContent) throw new Error('Missing page data')
const data = JSON.parse(source.textContent)
if (!pages.includes(data.page)) throw new Error('Unknown page')
createApp(App, { data }).mount('#app')
