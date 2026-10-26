// Package locale owns application messages. Chinese source messages are stable
// lookup keys; interpolation stays with fmt so errors preserve Go's %w wrapping.
// User data (network names, secrets, Core event payloads) is never translated.
package locale

import (
	"embed"
	"encoding/json"
	"strings"
	"sync/atomic"
)

//go:embed en.json
var catalogs embed.FS
var english = func() map[string]string {
	data, err := catalogs.ReadFile("en.json")
	if err != nil {
		panic(err)
	}
	var values map[string]string
	if err := json.Unmarshal(data, &values); err != nil {
		panic(err)
	}
	return values
}()
var language atomic.Value

func init() { language.Store("zh-Hans") }
func Normalize(tag string) string {
	if strings.HasPrefix(strings.ToLower(tag), "en") {
		return "en"
	}
	return "zh-Hans"
}
func Set(tag string)   { language.Store(Normalize(tag)) }
func Language() string { return language.Load().(string) }
func In(tag, text string) string {
	if Normalize(tag) == "en" {
		if value, ok := english[text]; ok {
			return value
		}
	}
	return text
}
func T(text string) string { return In(Language(), text) }
