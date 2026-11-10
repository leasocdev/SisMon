import './styles/tokens.css';
import './styles/base.css';
import { mount } from 'svelte';
import App from './App.svelte';
import { startRouter } from './lib/router.svelte';

startRouter();
mount(App, { target: document.getElementById('app')! });
