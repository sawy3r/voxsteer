//go:build desktop && windows

package main

import _ "embed"

//go:embed assets/voxsteer.ico
var embeddedTrayIcon []byte

func trayIcon() []byte {
	return embeddedTrayIcon
}
