package api

import "github.com/evanw/esbuild/internal/compat"

type EngineName uint8

const (
	EngineChrome EngineName = iota
	EngineEdge
	EngineFirefox
	EngineIE
	EngineIOS
	EngineNode
	EngineOpera
	EngineSafari
)

func convertEngineName(engine EngineName) compat.Engine {
	switch engine {
	case EngineChrome:
		return compat.Chrome
	case EngineEdge:
		return compat.Edge
	case EngineFirefox:
		return compat.Firefox
	case EngineIE:
		return compat.IE
	case EngineIOS:
		return compat.IOS
	case EngineNode:
		return compat.Node
	case EngineOpera:
		return compat.Opera
	case EngineSafari:
		return compat.Safari
	default:
		panic("Invalid engine name")
	}
}
