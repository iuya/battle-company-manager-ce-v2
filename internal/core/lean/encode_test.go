package lean

import (
	"encoding/json"
	"reflect"
)

// Encode writes a model value back as JSON in the lean format, keys sorted by encoding/json. It is the inverse of Bind for a document
// that bound without errors, which the round-trip and fuzz tests rely on.
func Encode(v any) ([]byte, error) {
	return json.Marshal(encodeTree(reflect.ValueOf(v)))
}
