package web

import "embed"

//go:embed index.html app.css app.js md.js manifest.json sw.js icon-192.png icon-512.png
var FS embed.FS
