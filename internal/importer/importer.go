// Package importer composes the supported parser registry explicitly.
package importer

import (
	"github.com/amb1s1/tracefull/internal/parse"
	"github.com/amb1s1/tracefull/internal/parse/dublin"
	"github.com/amb1s1/tracefull/internal/parse/mtr"
	"github.com/amb1s1/tracefull/internal/parse/paris"
	"github.com/amb1s1/tracefull/internal/parse/tracepath"
	"github.com/amb1s1/tracefull/internal/parse/traceroute"
	"github.com/amb1s1/tracefull/internal/parse/tracert"
)

func Registry() parse.Registry {
	return parse.New(dublin.Parser{}, mtr.Parser{}, paris.Parser{}, tracert.Parser{}, tracepath.Parser{}, traceroute.Parser{})
}
