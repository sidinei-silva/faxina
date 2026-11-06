import { mount } from 'svelte'
import '@fontsource-variable/bricolage-grotesque'
import '@fontsource/atkinson-hyperlegible/400.css'
import '@fontsource/atkinson-hyperlegible/700.css'
import './style.css'
import App from './App.svelte'

const app = mount(App, { target: document.getElementById('app')! })

export default app
