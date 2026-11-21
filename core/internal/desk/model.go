package desk

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var placeholder = regexp.MustCompile(`(?i)^(|to be filled by o\.?e\.?m\.?|system manufacturer|system product name|system version|default string|o\.?e\.?m\.?|not applicable|not specified|none|n/a|unknown|invalid|undefined|type1productconfigid|x+|0+|1\.0|123456789)$`)

var brands = []struct {
	match *regexp.Regexp
	name  string
}{
	{regexp.MustCompile(`(?i)^micro-?star|^msi\b`), "MSI"},
	{regexp.MustCompile(`(?i)^asustek|^asus\b`), "ASUS"},
	{regexp.MustCompile(`(?i)^asrock`), "ASRock"},
	{regexp.MustCompile(`(?i)^gigabyte`), "Gigabyte"},
	{regexp.MustCompile(`(?i)^lenovo`), "Lenovo"},
	{regexp.MustCompile(`(?i)^hewlett-packard|^hp\b`), "HP"},
	{regexp.MustCompile(`(?i)^dell\b`), "Dell"},
	{regexp.MustCompile(`(?i)^acer\b`), "Acer"},
	{regexp.MustCompile(`(?i)^huawei`), "Huawei"},
	{regexp.MustCompile(`(?i)^honor\b`), "Honor"},
	{regexp.MustCompile(`(?i)^timi\b|^xiaomi`), "Xiaomi"},
	{regexp.MustCompile(`(?i)^samsung`), "Samsung"},
	{regexp.MustCompile(`(?i)^microsoft`), "Microsoft"},
	{regexp.MustCompile(`(?i)^apple\b`), "Apple"},
	{regexp.MustCompile(`(?i)^toshiba|^dynabook`), "Dynabook"},
	{regexp.MustCompile(`(?i)^fujitsu`), "Fujitsu"},
	{regexp.MustCompile(`(?i)^razer`), "Razer"},
	{regexp.MustCompile(`(?i)^intel\b`), "Intel"},
}

var twice = regexp.MustCompile(`(\S+)_(\S+)$`)

var corporate = regexp.MustCompile(`(?i)[\s,]+(co\.?|ltd\.?|inc\.?|corp\.?|corporation|limited|llc|gmbh|ag|s\.?a\.?|international|technology|technologies|computer|electronics)\.?$`)

func clean(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if placeholder.MatchString(s) {
		return ""
	}
	return s
}

func brand(maker string) string {
	for _, b := range brands {
		if b.match.MatchString(maker) {
			return b.name
		}
	}
	for {
		short := corporate.ReplaceAllString(maker, "")
		if short == maker {
			return maker
		}
		maker = short
	}
}

func Model(maker, product, version string) string {
	maker, product, version = clean(maker), clean(product), clean(version)
	name := brand(maker)
	if name == "Lenovo" && version != "" && !strings.EqualFold(version, product) {
		product = version
	}
	if m := twice.FindStringSubmatch(product); m != nil && strings.EqualFold(m[1], m[2]) {
		product = strings.TrimSuffix(product, "_"+m[2])
	}
	product = strings.Join(strings.Fields(strings.ReplaceAll(product, "_", " ")), " ")
	if product != "" && name != "" && strings.HasPrefix(strings.ToLower(product), strings.ToLower(name)) {
		name = ""
	}
	model := strings.TrimSpace(name + " " + product)
	if model == "" {
		return "PC"
	}
	if utf8.RuneCountInString(model) > 64 {
		model = strings.TrimSpace(string([]rune(model)[:64]))
	}
	return model
}
