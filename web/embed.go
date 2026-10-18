// Package web holds the small demo front end, embedded into the binary.
package web

import "embed"

//go:embed index.html style.css app.js
var FS embed.FS