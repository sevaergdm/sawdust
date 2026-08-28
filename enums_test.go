package sawdust

import (
	"fmt"
	"testing"
)

func TestEnums(t *testing.T) {
	tests := []struct {
		name  string
		input fmt.Stringer
		want  string
	}{
		{"physical known", TypeInt64, "int64"},
		{"physical unknown", PhysicalType(99), "PhysicalType(99)"},
		{"repetition known", RepetitionOptional, "optional"},
		{"repetition unknown", FieldRepetitionType(9), "FieldRepetitionType(9)"},
		{"converted known", ConvertedUTF8, "utf8"},
		{"converted unknown", ConvertedType(99), "ConvertedType(99)"},
		{"timeunit known", TimeMicros, "micros"},
		{"timeunit unknown", TimeUnit(9), "TimeUnit(9)"},
		{"encoding known", EncodingALP, "ALP"},
		{"encoding unknown", Encoding(99), "Encoding(99)"},
		{"codec known", CodecGZIP, "GZIP"},
		{"codec unknown", CompressionCodec(99), "CompressionCodec(99)"},
		{"page type known", DataPage, "DATA_PAGE"},
		{"page type unknown", PageType(99), "PageType(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.input.String()

			if got != tt.want {
				t.Errorf("want: %q, but got: %q", tt.want, got)
			}
		})
	}
}
