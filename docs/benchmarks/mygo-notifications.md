# MyGo native notifications (review P5)

MyGo v0.10 exposes `mygo.NotificationsSupported()` and
`mygo.NewNotification(mygo.NotificationOptions{...}).Show()`, so no fallback
capability gap is claimed: the shell wires a real notification binding.

## Wiring

- The renderer calls `shellService.NotifyApproval(title, body)` only from the
  focus-gated hook in `frontend/src/lib/notifications.svelte.ts`.
- The Go binding (`shell/mygo/notifications.go`) checks
  `NotificationsSupported()` and `Window.IsFocused()` before showing the
  notification. A denied permission is logged as a user choice, not a failure.
- The fake-IPC seam injects a browser notifier, so Playwright asserts the focus
  decision (`approval.spec.ts`) without delivering a real macOS notification.

## Not verified here

Actual delivery and the first-run permission prompt require a packaged,
signed app and cannot be asserted in this repository's test seams. The code
path is real; only live delivery remains a manual packaged-app check.
