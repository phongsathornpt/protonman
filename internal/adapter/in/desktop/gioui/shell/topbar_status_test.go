//go:build desktop || desktop_gio

package shell

import "testing"

func TestConnectionStatusLabelDistinguishesSoftFailures(t *testing.T) {
	cases := map[string]string{
		"Runtime update failed · boom":           "Runtime failed",
		"Runtime not updated · no session":       "Not updated",
		"Runtime update returned an invalid one": "Runtime failed",
		"Session history failed · boom":          "History failed",
		"Prompt failed · boom":                   "Prompt failed",
		"Prompt not sent · busy":                 "Not sent",
		"Cancel failed · boom":                   "Cancel failed",
		"Connection failed · boom":               "Connection failed",
		"Start failed · boom":                    "Start failed",
		"Connected · session refresh failed":     "Refresh failed",
		"Updating runtime…":                      "Updating…",
		"Runtime updated":                        "Runtime updated",
		"Custom failed · boom":                   "Action failed",
		"Disconnected · retrying":                "Disconnected",
	}
	for status, want := range cases {
		if got := connectionStatusLabel(status); got != want {
			t.Errorf("connectionStatusLabel(%q) = %q, want %q", status, got, want)
		}
	}
}
