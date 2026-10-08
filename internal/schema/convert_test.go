package schema

import (
	"reflect"
	"testing"
)

func TestJSONSchemaType(t *testing.T) {
	cases := []struct {
		flagType string
		want     string
	}{
		{"bool", "boolean"},
		{"count", "integer"},
		{"int", "integer"},
		{"int8", "integer"},
		{"uint64", "integer"},
		{"float32", "number"},
		{"float64", "number"},
		{"string", "string"},
		{"duration", "string"},
		{"format", "string"},
	}
	for _, tc := range cases {
		t.Run(tc.flagType, func(t *testing.T) {
			if got := JSONSchemaType(tc.flagType); got != tc.want {
				t.Fatalf("JSONSchemaType(%q) = %q, want %q", tc.flagType, got, tc.want)
			}
		})
	}
}

func TestJSONSchemaArrayItemType(t *testing.T) {
	cases := []struct {
		flagType string
		want     string
		ok       bool
	}{
		{"boolSlice", "boolean", true},
		{"intSlice", "integer", true},
		{"int32Slice", "integer", true},
		{"int64Slice", "integer", true},
		{"uintSlice", "integer", true},
		{"float32Slice", "number", true},
		{"float64Slice", "number", true},
		{"durationSlice", "string", true},
		{"ipSlice", "string", true},
		{"ipNetSlice", "string", true},
		{"stringArray", "string", true},
		{"stringSlice", "string", true},
		{"string", "", false},
		{"int", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.flagType, func(t *testing.T) {
			got, ok := JSONSchemaArrayItemType(tc.flagType)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("JSONSchemaArrayItemType(%q) = (%q, %v), want (%q, %v)", tc.flagType, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestScalarJSON(t *testing.T) {
	cases := []struct {
		name     string
		flagType string
		value    string
		want     any
		ok       bool
	}{
		{"bool", "bool", "true", true, true},
		{"bool invalid", "bool", "yes", nil, false},
		{"int", "int", "-7", int64(-7), true},
		{"int8", "int8", "5", int64(5), true},
		{"int64", "int64", "9223372036854775807", int64(9223372036854775807), true},
		{"count", "count", "2", int64(2), true},
		{"uint", "uint", "7", uint64(7), true},
		{"uint64", "uint64", "18446744073709551615", uint64(18446744073709551615), true},
		{"uint negative", "uint", "-1", nil, false},
		{"int invalid", "int", "x", nil, false},
		{"float", "float64", "1.5", 1.5, true},
		{"float invalid", "float32", "x", nil, false},
		{"string", "string", "svc", "svc", true},
		{"duration", "duration", "45s", "45s", true},
		{"custom", "format", "json", "json", true},
		{"empty", "string", "", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ScalarJSON(tc.flagType, tc.value)
			if ok != tc.ok {
				t.Fatalf("ScalarJSON(%q, %q) ok = %v, want %v", tc.flagType, tc.value, ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("ScalarJSON(%q, %q) = %#v, want %#v", tc.flagType, tc.value, got, tc.want)
			}
		})
	}
}

func TestArrayItemJSON(t *testing.T) {
	cases := []struct {
		name     string
		value    string
		itemType string
		flagType string
		want     any
		ok       bool
	}{
		{"bool", "false", "boolean", "boolSlice", false, true},
		{"bool invalid", "x", "boolean", "boolSlice", nil, false},
		{"int", "-3", "integer", "intSlice", int64(-3), true},
		{"uint", "3", "integer", "uintSlice", uint64(3), true},
		{"uint negative", "-3", "integer", "uintSlice", nil, false},
		{"int invalid", "x", "integer", "int64Slice", nil, false},
		{"float", "2.5", "number", "float64Slice", 2.5, true},
		{"float invalid", "x", "number", "float32Slice", nil, false},
		{"string", "a", "string", "stringSlice", "a", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ArrayItemJSON(tc.value, tc.itemType, tc.flagType)
			if ok != tc.ok {
				t.Fatalf("ArrayItemJSON ok = %v, want %v", ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("ArrayItemJSON = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestValidateValue(t *testing.T) {
	cases := []struct {
		name     string
		flagType string
		value    string
		wantErr  bool
	}{
		{"bool", "bool", "true", false},
		{"bool invalid", "bool", "maybe", true},
		{"int", "int", "-5", false},
		{"int hex", "int", "0x10", false},
		{"int invalid", "int", "three", true},
		{"int8 in range", "int8", "127", false},
		{"int8 out of range", "int8", "300", true},
		{"int16 out of range", "int16", "40000", true},
		{"int32", "int32", "-2147483648", false},
		{"int64 out of range", "int64", "9223372036854775808", true},
		{"count", "count", "3", false},
		{"uint", "uint", "5", false},
		{"uint negative", "uint", "-1", true},
		{"uint8 out of range", "uint8", "256", true},
		{"uint16", "uint16", "65535", false},
		{"uint32 out of range", "uint32", "4294967296", true},
		{"uint64", "uint64", "18446744073709551615", false},
		{"float32", "float32", "1.5", false},
		{"float64 invalid", "float64", "x", true},
		{"duration", "duration", "45s", false},
		{"duration invalid", "duration", "abc", true},
		{"string", "string", "anything", false},
		{"custom unchecked", "format", "anything", false},
		{"stringSlice", "stringSlice", "a,b", false},
		{"stringSlice bad csv", "stringSlice", `"a`, true},
		{"stringArray", "stringArray", `a,"b`, false},
		{"intSlice", "intSlice", "1,2", false},
		{"intSlice invalid", "intSlice", "1,x", true},
		{"int32Slice", "int32Slice", "1,-2", false},
		{"int32Slice out of range", "int32Slice", "1,2147483648", true},
		{"int64Slice", "int64Slice", "1,2", false},
		{"uintSlice", "uintSlice", "1,2", false},
		{"uintSlice negative", "uintSlice", "1,-2", true},
		{"boolSlice", "boolSlice", "true,false", false},
		{"boolSlice invalid", "boolSlice", "true,x", true},
		{"float32Slice", "float32Slice", "1.5,2", false},
		{"float64Slice invalid", "float64Slice", "1.5,x", true},
		{"durationSlice", "durationSlice", "1s,2m", false},
		{"durationSlice invalid", "durationSlice", "1s,x", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateValue(tc.flagType, tc.value)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateValue(%q, %q) err = %v, wantErr %v", tc.flagType, tc.value, err, tc.wantErr)
			}
		})
	}
}

func TestCanonicalEnumValue(t *testing.T) {
	cases := []struct {
		name     string
		flagType string
		value    string
		want     string
		wantErr  bool
	}{
		{"string unchanged", "string", "Json", "Json", false},
		{"string empty", "string", "", "", false},
		{"custom unchanged", "format", " x ", " x ", false},
		{"int leading zero", "int", "03", "3", false},
		{"int plus", "int", "+5", "5", false},
		{"int negative zero", "int", "-0", "0", false},
		{"int hex", "int", "0x10", "16", false},
		{"int octal matches pflag", "int", "010", "8", false},
		{"int invalid", "int", "x", "", true},
		{"int empty", "int", "", "", true},
		{"int8 overflow", "int8", "300", "", true},
		{"int64 overflow", "int64", "9223372036854775808", "", true},
		{"uint16", "uint16", "007", "7", false},
		{"uint negative", "uint", "-1", "", true},
		{"uint8 overflow", "uint8", "256", "", true},
		{"bool unsupported", "bool", "true", "", true},
		{"count unsupported", "count", "1", "", true},
		{"float unsupported", "float64", "1", "", true},
		{"duration unsupported", "duration", "1s", "", true},
		{"slice unsupported", "stringSlice", "a", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CanonicalEnumValue(tc.flagType, tc.value)
			if (err != nil) != tc.wantErr {
				t.Fatalf("CanonicalEnumValue(%q, %q) err = %v, wantErr %v", tc.flagType, tc.value, err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("CanonicalEnumValue(%q, %q) = %q, want %q", tc.flagType, tc.value, got, tc.want)
			}
		})
	}
}

func TestExampleJSON(t *testing.T) {
	cases := []struct {
		name     string
		flagType string
		example  string
		want     any
		ok       bool
	}{
		{"int decimal", "int", "5", int64(5), true},
		{"int hex canonicalised", "int", "0x10", int64(16), true},
		{"int octal as pflag stores it", "int", "010", int64(8), true},
		{"count", "count", "2", int64(2), true},
		{"uint", "uint8", "7", uint64(7), true},
		{"int invalid", "int", "x", nil, false},
		{"bool", "bool", "true", true, true},
		{"float", "float64", "1.5", 1.5, true},
		{"duration", "duration", "45s", "45s", true},
		{"custom", "format", "json", "json", true},
		{"stringSlice", "stringSlice", "a,b", []any{"a", "b"}, true},
		{"stringSlice bad csv", "stringSlice", `"a`, nil, false},
		{"stringArray single element", "stringArray", "a,b", []any{"a,b"}, true},
		{"intSlice", "intSlice", "1,2", []any{int64(1), int64(2)}, true},
		{"int64Slice hex", "int64Slice", "0x10,2", []any{int64(16), int64(2)}, true},
		{"int32Slice invalid", "int32Slice", "1,x", nil, false},
		{"uintSlice", "uintSlice", "3", []any{uint64(3)}, true},
		{"boolSlice invalid", "boolSlice", "x", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ExampleJSON(tc.flagType, tc.example)
			if ok != tc.ok {
				t.Fatalf("ExampleJSON(%q, %q) ok = %v, want %v", tc.flagType, tc.example, ok, tc.ok)
			}
			if ok && !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ExampleJSON(%q, %q) = %#v, want %#v", tc.flagType, tc.example, got, tc.want)
			}
		})
	}
}
