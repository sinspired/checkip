package data

import (
	_ "embed"
)

//go:embed GeoLite2-City.mmdb.zst
var EmbeddedMaxMindDBCity []byte
