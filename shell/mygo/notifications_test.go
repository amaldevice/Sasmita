package main

import "testing"

type notifierFake struct {
	supported bool
	frontmost bool
	shown     []string
}

func (n *notifierFake) Supported() bool { return n.supported }
func (n *notifierFake) Frontmost() bool { return n.frontmost }
func (n *notifierFake) Show(title, body string) error {
	n.shown = append(n.shown, title+" — "+body)
	return nil
}

// A macOS notification is only useful when Sasmita is in the background.
func TestApprovalNotificationIsFocusGated(t *testing.T) {
	cases := []struct {
		name      string
		supported bool
		frontmost bool
		want      int
	}{
		{"background", true, false, 1},
		{"frontmost", true, true, 0},
		{"unsupported", false, false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &notifierFake{supported: tc.supported, frontmost: tc.frontmost}
			if err := notifyApproval(fake, "Approval needed", "Overwrite chart"); err != nil {
				t.Fatal(err)
			}
			if len(fake.shown) != tc.want {
				t.Fatalf("shown = %v, want %d notifications", fake.shown, tc.want)
			}
		})
	}
}
