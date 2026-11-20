package xcfgtest

import (
	"encoding/json"

	"github.com/SpofyJet/spofy-app/core/internal/xcfg"
)

func Socks(server json.RawMessage, port int, level string) ([]byte, error) {
	built, err := xcfg.Build(server, xcfg.Options{Mode: xcfg.ModeTun, LogLevel: level})
	if err != nil {
		return nil, err
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(built, &out); err != nil {
		return nil, err
	}
	var inbounds []map[string]json.RawMessage
	if err := json.Unmarshal(out["inbounds"], &inbounds); err != nil || len(inbounds) != 1 {
		return nil, xcfg.ErrConfig
	}
	in := map[string]any{
		"tag": xcfg.TagTun, "protocol": "socks", "listen": "127.0.0.1", "port": port,
		"settings": map[string]any{"auth": "password", "accounts": []map[string]string{{"user": "u", "pass": "p"}}, "udp": true},
		"sniffing": inbounds[0]["sniffing"],
	}
	if out["inbounds"], err = json.Marshal([]any{in}); err != nil {
		return nil, err
	}
	return json.Marshal(out)
}
