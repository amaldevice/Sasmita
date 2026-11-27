import { mount } from 'svelte';
import { isMyGo } from 'mygo-runtime';
import './app.css';
import App from './App.svelte';
import { setNotifier } from './lib/notifications.svelte';
import { shellService } from './mygo';

// Only the real shell gets a native notifier. The fake seam injects its own
// bridge, so its focus-gate tests keep exercising the same decision path.
if (isMyGo()) {
  setNotifier((title, body) => {
    void shellService.notifyApproval(title, body);
  });
}

export default mount(App, { target: document.getElementById('app')! });
