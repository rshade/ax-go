package schema

import (
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"
)

// JSON Schema type names emitted for flag properties.
const (
	JSONBoolean = "boolean"
	JSONInteger = "integer"
	JSONNumber  = "number"
	JSONString  = "string"
)

// pflag type names, as returned by pflag.Value.Type().
const (
	typeBool          = "bool"
	typeCount         = "count"
	typeString        = "string"
	typeDuration      = "duration"
	typeFloat32       = "float32"
	typeFloat64       = "float64"
	typeInt           = "int"
	typeInt8          = "int8"
	typeInt16         = "int16"
	typeInt32         = "int32"
	typeInt64         = "int64"
	typeUint          = "uint"
	typeUint8         = "uint8"
	typeUint16        = "uint16"
	typeUint32        = "uint32"
	typeUint64        = "uint64"
	typeBoolSlice     = "boolSlice"
	typeIntSlice      = "intSlice"
	typeInt32Slice    = "int32Slice"
	typeInt64Slice    = "int64Slice"
	typeUintSlice     = "uintSlice"
	typeFloat32Slice  = "float32Slice"
	typeFloat64Slice  = "float64Slice"
	typeDurationSlice = "durationSlice"
	typeStringSlice   = "stringSlice"
	typeStringArray   = "stringArray"
	typeIP            = "ip"
	typeIPMask        = "ipMask"
	typeIPNet         = "ipNet"
	typeIPSlice       = "ipSlice"
	typeIPNetSlice    = "ipNetSlice"
	typeBytesHex      = "bytesHex"
	typeBytesBase64   = "bytesBase64"
	typeStringToStr   = "stringToString"
	typeStringToInt   = "stringToInt"
	typeStringToInt64 = "stringToInt64"
)

// Integer bit sizes and bases handed to strconv.
const (
	bits8   = 8
	bits16  = 16
	bits32  = 32
	bits64  = 64
	base10  = 10
	baseAny = 0
)

var errUnsupportedEnumType = errors.New("flag type does not support an enum")

// JSONSchemaType maps a pflag type name to the JSON Schema type of a scalar
// property. Any type it does not recognise, including custom pflag.Value types,
// is a string.
func JSONSchemaType(flagType string) string {
	switch flagType {
	case typeBool:
		return JSONBoolean
	case typeCount, typeInt, typeInt8, typeInt16, typeInt32, typeInt64,
		typeUint, typeUint8, typeUint16, typeUint32, typeUint64:
		return JSONInteger
	case typeFloat32, typeFloat64:
		return JSONNumber
	default:
		return JSONString
	}
}

// JSONSchemaArrayItemType maps a pflag slice type name to the JSON Schema type
// of its array items. ok is false for every non-slice type.
func JSONSchemaArrayItemType(flagType string) (string, bool) {
	switch flagType {
	case typeBoolSlice:
		return JSONBoolean, true
	case typeIntSlice, typeInt32Slice, typeInt64Slice, typeUintSlice:
		return JSONInteger, true
	case typeFloat32Slice, typeFloat64Slice:
		return JSONNumber, true
	case typeDurationSlice, typeIPSlice, typeIPNetSlice, typeStringArray, typeStringSlice:
		return JSONString, true
	default:
		return "", false
	}
}

// ScalarJSON converts a scalar flag value in CLI string form to the JSON value
// matching JSONSchemaType(flagType): bool, int64, uint64, float64 or string.
// Integers are parsed in base 10, so callers pass canonical values. ok is false
// for an empty value or one that does not parse as the type, so a string is
// never advertised for a non-string type, and for NaN or ±Inf, which pflag
// accepts but encoding/json cannot marshal.
func ScalarJSON(flagType, value string) (any, bool) {
	if value == "" {
		return nil, false
	}
	switch JSONSchemaType(flagType) {
	case JSONBoolean:
		parsed, err := strconv.ParseBool(value)
		return parsed, err == nil
	case JSONInteger:
		if strings.HasPrefix(flagType, typeUint) {
			parsed, err := strconv.ParseUint(value, base10, bits64)
			return parsed, err == nil
		}
		parsed, err := strconv.ParseInt(value, base10, bits64)
		return parsed, err == nil
	case JSONNumber:
		return finiteFloatJSON(value)
	default:
		return value, true
	}
}

// ArrayItemJSON converts one element of a slice flag to the JSON value matching
// itemType (from JSONSchemaArrayItemType). Integers are parsed in base 10;
// uintSlice elements become uint64 and every other integer slice int64. ok is
// false when the element does not parse or is a non-finite float.
func ArrayItemJSON(value, itemType, flagType string) (any, bool) {
	switch itemType {
	case JSONBoolean:
		parsed, err := strconv.ParseBool(value)
		return parsed, err == nil
	case JSONInteger:
		if flagType == typeUintSlice {
			parsed, err := strconv.ParseUint(value, base10, bits64)
			return parsed, err == nil
		}
		parsed, err := strconv.ParseInt(value, base10, bits64)
		return parsed, err == nil
	case JSONNumber:
		return finiteFloatJSON(value)
	default:
		return value, true
	}
}

func finiteFloatJSON(value string) (any, bool) {
	parsed, err := strconv.ParseFloat(value, bits64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return nil, false
	}
	return parsed, true
}

// SplitSliceValue splits a slice flag value in CLI form into its elements the
// way pflag's Set does: CSV for stringSlice and boolSlice, a single element for
// stringArray, and a plain comma split for every other slice type.
func SplitSliceValue(flagType, value string) ([]string, error) {
	switch flagType {
	case typeStringArray:
		return []string{value}, nil
	case typeStringSlice, typeBoolSlice:
		elements, err := csv.NewReader(strings.NewReader(value)).Read()
		if err != nil {
			return nil, fmt.Errorf("parse %s value as CSV: %w", flagType, err)
		}
		return elements, nil
	default:
		return strings.Split(value, ","), nil
	}
}

// ValidateValue reports whether value, in CLI form, is accepted by a flag of
// flagType, using the same parsing rules pflag applies in Set: integers in the
// base and bit size pflag uses, durations through time.ParseDuration, slice
// values split per SplitSliceValue with each element checked, and the ip, ipMask,
// ipNet, bytes, stringTo* and ip slice types through pflag's own Set. Custom
// pflag.Value types are unchecked and return nil, as are func, boolfunc and
// time, whose Set runs an author callback or depends on per-flag formats.
func ValidateValue(flagType, value string) error {
	if handled, err := validateWithPflag(flagType, value); handled {
		return err
	}
	if _, ok := JSONSchemaArrayItemType(flagType); ok {
		return validateSliceValue(flagType, value)
	}
	return validateScalarValue(flagType, value)
}

func validateScalarValue(flagType, value string) error {
	var err error
	switch flagType {
	case typeBool:
		_, err = strconv.ParseBool(value)
	case typeCount, typeInt, typeInt8, typeInt16, typeInt32, typeInt64,
		typeUint, typeUint8, typeUint16, typeUint32, typeUint64:
		_, err = CanonicalEnumValue(integerParseType(flagType), value)
	case typeFloat32:
		_, err = strconv.ParseFloat(value, bits32)
	case typeFloat64:
		_, err = strconv.ParseFloat(value, bits64)
	case typeDuration:
		_, err = time.ParseDuration(value)
	}
	if err != nil {
		return fmt.Errorf("value is not a valid %s: %w", flagType, err)
	}
	return nil
}

func validateSliceValue(flagType, value string) error {
	elements, err := SplitSliceValue(flagType, value)
	if err != nil {
		return err
	}
	for _, element := range elements {
		var elementErr error
		switch flagType {
		case typeBoolSlice:
			_, elementErr = strconv.ParseBool(element)
		case typeIntSlice:
			_, elementErr = strconv.Atoi(element)
		case typeInt32Slice:
			_, elementErr = strconv.ParseInt(element, baseAny, bits32)
		case typeInt64Slice:
			_, elementErr = strconv.ParseInt(element, baseAny, bits64)
		case typeUintSlice:
			_, elementErr = strconv.ParseUint(element, base10, 0)
		case typeFloat32Slice:
			_, elementErr = strconv.ParseFloat(element, bits32)
		case typeFloat64Slice:
			_, elementErr = strconv.ParseFloat(element, bits64)
		case typeDurationSlice:
			_, elementErr = time.ParseDuration(element)
		}
		if elementErr != nil {
			return fmt.Errorf("element is not a valid %s item: %w", flagType, elementErr)
		}
	}
	return nil
}

// validateWithPflag checks value with pflag's own Set on a scratch FlagSet for
// the built-in types ax-go does not parse itself, so validation cannot drift
// from what the real flag accepts. handled is false for every other type.
func validateWithPflag(flagType, value string) (bool, error) {
	scratch := pflag.NewFlagSet(flagType, pflag.ContinueOnError)
	switch flagType {
	case typeIP:
		scratch.IP(flagType, nil, "")
	case typeIPMask:
		scratch.IPMask(flagType, nil, "")
	case typeIPNet:
		scratch.IPNet(flagType, net.IPNet{}, "")
	case typeIPSlice:
		scratch.IPSlice(flagType, nil, "")
	case typeIPNetSlice:
		scratch.IPNetSlice(flagType, nil, "")
	case typeBytesHex:
		scratch.BytesHex(flagType, nil, "")
	case typeBytesBase64:
		scratch.BytesBase64(flagType, nil, "")
	case typeStringToStr:
		scratch.StringToString(flagType, nil, "")
	case typeStringToInt:
		scratch.StringToInt(flagType, nil, "")
	case typeStringToInt64:
		scratch.StringToInt64(flagType, nil, "")
	default:
		return false, nil
	}
	if err := scratch.Set(flagType, value); err != nil {
		return true, fmt.Errorf("value is not a valid %s: %w", flagType, err)
	}
	return true, nil
}

// integerParseType maps count, whose Set parses an int, onto int so the shared
// integer parser covers it.
func integerParseType(flagType string) string {
	if flagType == typeCount {
		return typeInt
	}
	return flagType
}

// CanonicalEnumValue returns the form in which value is compared for enum
// membership on a flag of flagType. string and custom pflag.Value types compare
// exactly, so value is returned unchanged. Integer types are parsed as pflag
// parses them (base prefix allowed, at the type's bit size) and re-formatted in
// base 10, so "03", "+3" and "0x3" all canonicalise to "3". Every other built-in
// type (bool, count, floats, duration, slices, ip, ...) returns an error, as does
// an integer value that does not parse or overflows the bit size.
func CanonicalEnumValue(flagType, value string) (string, error) {
	switch flagType {
	case typeInt, typeInt64:
		return canonicalSigned(value, bits64)
	case typeInt8:
		return canonicalSigned(value, bits8)
	case typeInt16:
		return canonicalSigned(value, bits16)
	case typeInt32:
		return canonicalSigned(value, bits32)
	case typeUint, typeUint64:
		return canonicalUnsigned(value, bits64)
	case typeUint8:
		return canonicalUnsigned(value, bits8)
	case typeUint16:
		return canonicalUnsigned(value, bits16)
	case typeUint32:
		return canonicalUnsigned(value, bits32)
	case typeString:
		return value, nil
	}
	if isBuiltinFlagType(flagType) {
		return "", fmt.Errorf("%s: %w", flagType, errUnsupportedEnumType)
	}
	return value, nil
}

func canonicalSigned(value string, bitSize int) (string, error) {
	parsed, err := strconv.ParseInt(value, baseAny, bitSize)
	if err != nil {
		return "", fmt.Errorf("parse integer: %w", err)
	}
	return strconv.FormatInt(parsed, base10), nil
}

func canonicalUnsigned(value string, bitSize int) (string, error) {
	parsed, err := strconv.ParseUint(value, baseAny, bitSize)
	if err != nil {
		return "", fmt.Errorf("parse unsigned integer: %w", err)
	}
	return strconv.FormatUint(parsed, base10), nil
}

// isBuiltinFlagType reports whether flagType is a type name pflag itself
// defines. Any other name belongs to a custom pflag.Value.
func isBuiltinFlagType(flagType string) bool {
	if _, ok := JSONSchemaArrayItemType(flagType); ok {
		return true
	}
	switch flagType {
	case typeBool, typeCount, typeString, typeDuration, typeFloat32, typeFloat64,
		typeInt, typeInt8, typeInt16, typeInt32, typeInt64,
		typeUint, typeUint8, typeUint16, typeUint32, typeUint64,
		typeIP, typeIPMask, typeIPNet, typeBytesHex, typeBytesBase64,
		typeStringToStr, typeStringToInt, typeStringToInt64,
		"func", "boolfunc", "time":
		return true
	default:
		return false
	}
}

// ExampleJSON converts a CLI-form example to the JSON value of an
// inputSchema "examples" element: a typed scalar, or for a slice flag a []any
// of typed elements. Integers are first canonicalised the way pflag parses
// them, so the advertised number is the one the flag would store. ok is false
// when any part fails to convert; callers then omit the example.
func ExampleJSON(flagType, example string) (any, bool) {
	if itemType, isSlice := JSONSchemaArrayItemType(flagType); isSlice {
		return sliceExampleJSON(flagType, itemType, example)
	}
	if JSONSchemaType(flagType) == JSONInteger {
		canonical, err := CanonicalEnumValue(integerParseType(flagType), example)
		if err != nil {
			return nil, false
		}
		example = canonical
	}
	return ScalarJSON(flagType, example)
}

func sliceExampleJSON(flagType, itemType, example string) (any, bool) {
	elements, err := SplitSliceValue(flagType, example)
	if err != nil {
		return nil, false
	}
	scalarType, baseZero := baseZeroSliceElementType(flagType)
	values := make([]any, 0, len(elements))
	for _, element := range elements {
		if baseZero {
			if element, err = CanonicalEnumValue(scalarType, element); err != nil {
				return nil, false
			}
		}
		converted, ok := ArrayItemJSON(element, itemType, flagType)
		if !ok {
			return nil, false
		}
		values = append(values, converted)
	}
	return values, true
}

// baseZeroSliceElementType names the scalar type whose parsing matches the
// elements of an integer slice pflag parses with a base prefix. intSlice and
// uintSlice parse base 10, which ArrayItemJSON already matches.
func baseZeroSliceElementType(flagType string) (string, bool) {
	switch flagType {
	case typeInt32Slice:
		return typeInt32, true
	case typeInt64Slice:
		return typeInt64, true
	default:
		return "", false
	}
}
