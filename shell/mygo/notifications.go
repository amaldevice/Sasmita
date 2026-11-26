package main

import (
	"errors"

	"github.com/egoist/mygo"
)

// desktopNotifier is the small seam the focus gate and the shell test use. The
// real implementation delegates to MyGo's native notification API.
type desktopNotifier interface {
	Supported() bool
	Frontmost() bool
	Show(title, body string) error
}

type mygoNotifier struct{ window *mygo.Window }

func (m mygoNotifier) Supported() bool { return mygo.NotificationsSupported() }

func (m mygoNotifier) Frontmost() bool { return m.window != nil && m.window.IsFocused() }

func (m mygoNotifier) Show(title, body string) error {
	n := mygo.NewNotification(mygo.NotificationOptions{
		ID: "sasmita-approval", Group: "Sasmita", Title: title, Body: body,
	})
	err := n.Show()
	// A denied permission is a user choice, not a run failure.
	if errors.Is(err, mygo.ErrNotificationsDenied) {
		return nil
	}
	return err
}

// notifyApproval delivers a notification only when the app is in the
// background and the platform supports native notifications (story 33).
func notifyApproval(notifier desktopNotifier, title, body string) error {
	if notifier == nil || !notifier.Supported() || notifier.Frontmost() {
		return nil
	}
	return notifier.Show(title, body)
}
