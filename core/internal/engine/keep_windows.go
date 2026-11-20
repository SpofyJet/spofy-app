package engine

import "github.com/xtls/xray-core/proxy/tun"

func keep() func() { return tun.Keep() }
