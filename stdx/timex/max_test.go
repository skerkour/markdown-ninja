package timex

import (
	"reflect"
	"testing"
	"time"
)

func TestMax(t *testing.T) {
	now := time.Now().UTC()
	afterNow := time.Now().UTC().Add(time.Hour)
	type args struct {
		x     time.Time
		y     time.Time
		times []time.Time
	}
	tests := []struct {
		name string
		args args
		want time.Time
	}{
		{
			name: "x < y",
			args: args{
				x:     now,
				y:     afterNow,
				times: []time.Time{},
			},
			want: afterNow,
		},
		{
			name: "x > y",
			args: args{
				x:     afterNow,
				y:     now,
				times: []time.Time{},
			},
			want: afterNow,
		},
		{
			name: "x > y",
			args: args{
				x:     afterNow,
				y:     now,
				times: []time.Time{},
			},
			want: afterNow,
		},
		{
			name: "x = y < times",
			args: args{
				x: now,
				y: now,
				times: []time.Time{
					now,
					now,
					afterNow,
					now,
				},
			},
			want: afterNow,
		},
		{
			name: "x = y > times",
			args: args{
				x: afterNow,
				y: afterNow,
				times: []time.Time{
					now,
					now,
					now,
				},
			},
			want: afterNow,
		},
		{
			name: "x = y",
			args: args{
				x:     now,
				y:     now,
				times: []time.Time{},
			},
			want: now,
		},
		{
			name: "x = y = times",
			args: args{
				x: now,
				y: now,
				times: []time.Time{
					now,
					now,
					now,
					now,
				},
			},
			want: now,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Max(tt.args.x, tt.args.y, tt.args.times...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Max() = %v, want %v", got, tt.want)
			}
		})
	}
}
