// Package webassets embeds the production Web/PWA shell into the server binary.
package webassets

import (
	"embed"
	"io/fs"
)

//go:embed dist
var files embed.FS

func FS() fs.FS {
	sub, err := fs.Sub(files, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
