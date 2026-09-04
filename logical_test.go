package sawdust

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestToTimes(t *testing.T) {
	tests := []struct {
		name    string
		input   []*int64
		tsType  TimestampType
		want    TimestampValues
		wantErr bool
	}{
		{
			name:    "microseconds",
			input:   []*int64{ptr(int64(1788504051000000))},
			tsType:  TimestampType{IsAdjustedToUTC: true, Unit: TimeMicros},
			want:    TimestampValues([]*time.Time{ptr(time.Date(2026, 9, 4, 6, 40, 51, 0, time.UTC))}),
			wantErr: false,
		},
		{
			name:    "error: invalid timestamp type",
			input:   []*int64{ptr(int64(1788504051000000))},
			tsType:  TimestampType{IsAdjustedToUTC: true, Unit: 99},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toTimes(tt.input, tt.tsType)
			if (err != nil) != tt.wantErr {
				t.Fatalf("expected no error, but got: %v", err)
			}

			if tt.wantErr {
				return
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):%s\n", diff)
			}
		})
	}
}
