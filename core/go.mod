module github.com/SpofyJet/spofy-app/core

go 1.27

require (
	github.com/tailscale/wf v0.0.0-20240214030419-6fbb0a674ee6
	github.com/xtls/xray-core v0.0.0-00010101000000-000000000000
	golang.org/x/sys v0.50.0
	golang.zx2c4.com/wintun v0.0.0-20230126152724-0fa3db229ce2
	golang.zx2c4.com/wireguard v0.0.0-20261024220952-b3bc704ef0ee
)

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/apernet/quic-go v0.65.1-0.20261109005002-2895f039137a // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/huin/goupnp v1.3.1 // indirect
	github.com/jackpal/go-nat-pmp v1.1.0 // indirect
	github.com/juju/ratelimit v1.0.2 // indirect
	github.com/klauspost/compress v1.20.5 // indirect
	github.com/klauspost/cpuid/v2 v2.4.2 // indirect
	github.com/koron/go-ssdp v0.9.2 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/libp2p/go-nat v1.0.1-0.20250821073202-01afc089f138 // indirect
	github.com/libp2p/go-netroute v0.4.0 // indirect
	github.com/miekg/dns v1.1.73 // indirect
	github.com/molecule-man/go-brrr v1.3.1 // indirect
	github.com/pion/dtls/v3 v3.1.16 // indirect
	github.com/pion/logging v0.2.4 // indirect
	github.com/pion/stun/v3 v3.1.7 // indirect
	github.com/pion/transport/v4 v4.1.3 // indirect
	github.com/pion/transport/v5 v5.1.5 // indirect
	github.com/pires/go-proxyproto v0.16.0 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/refraction-networking/utls v1.8.3-0.20261108164504-a13c59ffe98b // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/vishvananda/netlink v1.3.1 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	github.com/wlynxg/anet v0.0.5 // indirect
	github.com/xtls/reality v0.0.0-20261108101329-4bb46b2afd8b // indirect
	github.com/yuin/gopher-lua v1.1.2 // indirect
	go4.org/netipx v0.0.0-20260823151212-3075585bcbeb // indirect
	golang.org/x/crypto v0.59.0 // indirect
	golang.org/x/crypto/x509roots/fallback v0.0.0-20261115193255-739070949446 // indirect
	golang.org/x/exp v0.0.0-20261120182113-8801fbdfac81 // indirect
	golang.org/x/exp/typeparams v0.0.0-20261120182113-8801fbdfac81 // indirect
	golang.org/x/mod v0.43.0 // indirect
	golang.org/x/net v0.63.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/text v0.44.0 // indirect
	golang.org/x/tools v0.53.0 // indirect
	golang.zx2c4.com/wireguard/windows v1.1.1 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261118221215-c8830f628b3f // indirect
	google.golang.org/grpc v1.85.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	honnef.co/go/tools v0.8.2 // indirect
	layeh.com/gopher-luar v1.0.11 // indirect
	lukechampine.com/blake3 v1.5.1 // indirect
)

replace github.com/xtls/xray-core => ../../spofy-xray-core

replace github.com/xtls/reality => ../../spofy-xray-core/third_party/reality

replace github.com/apernet/quic-go => ../../spofy-xray-core/third_party/quic-go
