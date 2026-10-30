package schema

import _ "embed"

//go:embed report-v1.json
var Report []byte

//go:embed investigation-v1.json
var Investigation []byte

//go:embed collection-v1.json
var Collection []byte
