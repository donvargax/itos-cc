package metricsdir

import (
	"encoding"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/donvargax/itos-cc/internal/snapshot/port"
)

// Undecodable is the Unreadable Read returns when the snapshot name was
// read but what it holds did not decode, err being encoding/json's. This
// adapter alone decodes snapshot files, so the wording is its own: the port
// asks only for a Reason naming no Go type or package, and the in-process
// scenario driver, standing in for this adapter, words through it. The
// Reason is in the snapshot's own terms, never the Go types a reader
// decodes into, which change whenever one moves (bug-3):
//   - a value of the wrong kind names its key path, keys joined by dots and
//     an array's index in brackets, the JSON kind it should be and the kind
//     it is: "files[0].file should be a string, not a number", or "the
//     snapshot" for the whole of it. A reader that decodes a part itself
//     (an UnmarshalJSON) names the key path from that part;
//   - JSON that does not parse keeps the parser's text, which names no
//     type: "unexpected end of JSON input";
//   - any other failure of the decoder, which names the Go type it decodes
//     into and no snapshot's reader meets, is said without its detail.
func Undecodable(name string, err error) *port.Unreadable {
	return &port.Unreadable{Name: name, Reason: decodeReason(err)}
}

func decodeReason(err error) string {
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) {
		return syntax.Error()
	}
	var mistyped *json.UnmarshalTypeError
	if errors.As(err, &mistyped) && mistyped.Type != nil {
		return keyPath(mistyped.Field) + " should be " + expectedKind(mistyped.Type, mistyped.Value) + ", not " + foundKind(mistyped.Value)
	}
	return "it does not decode as this snapshot"
}

// keyPath is encoding/json's dotted field path in the snapshot's terms: an
// array's index in brackets, files[0].file, and the snapshot itself when
// the value is the whole of it.
func keyPath(field string) string {
	if field == "" {
		return "the snapshot"
	}
	var path strings.Builder
	for i, key := range strings.Split(field, ".") {
		switch {
		case key != "" && strings.Trim(key, "0123456789") == "":
			path.WriteString("[" + key + "]")
		case i > 0:
			path.WriteString("." + key)
		default:
			path.WriteString(key)
		}
	}
	return path.String()
}

// textUnmarshaler is the interface a value read from a JSON string has.
var textUnmarshaler = reflect.TypeFor[encoding.TextUnmarshaler]()

// expectedKind is the JSON kind a value of type t is read from. value is what
// was found: a number literal there ("number 1.5") is one of the right kind
// that t cannot hold, so the kind is said more closely.
func expectedKind(t reflect.Type, value string) string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	literal := strings.HasPrefix(value, "number ")
	if reflect.PointerTo(t).Implements(textUnmarshaler) {
		return "a string"
	}
	switch t.Kind() {
	case reflect.String:
		return "a string"
	case reflect.Bool:
		return "a boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if literal {
			return "a whole number"
		}
		return "a number"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if literal {
			return "a whole number, 0 or more"
		}
		return "a number"
	case reflect.Float32, reflect.Float64:
		if literal {
			return "a number in range"
		}
		return "a number"
	case reflect.Slice, reflect.Array:
		return "an array"
	case reflect.Struct, reflect.Map:
		return "an object"
	default:
		return "a value of another kind"
	}
}

// foundKind is the JSON kind encoding/json found, or the number literal it
// found when that is what does not fit.
func foundKind(value string) string {
	switch value {
	case "string":
		return "a string"
	case "number":
		return "a number"
	case "bool":
		return "a boolean"
	case "array":
		return "an array"
	case "object":
		return "an object"
	}
	if literal, ok := strings.CutPrefix(value, "number "); ok {
		return literal
	}
	return value
}
