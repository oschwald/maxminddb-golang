package maxminddbtag

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		tag     string
		want    Options
		wantErr string
	}{
		{name: "empty", tag: "", want: Options{}},
		{name: "name", tag: "items", want: Options{Name: "items", HasName: true}},
		{name: "ignored", tag: "-", want: Options{Ignored: true}},
		{
			name: "maximum",
			tag:  "items,maxsize:32",
			want: Options{Name: "items", HasName: true, MaxSize: 32, HasMaxSize: true},
		},
		{
			name: "default name maximum zero",
			tag:  ",maxsize:0",
			want: Options{MaxSize: 0, HasMaxSize: true},
		},
		{
			name: "quoted comma name",
			tag:  "'items,primary',maxsize:4",
			want: Options{Name: "items,primary", HasName: true, MaxSize: 4, HasMaxSize: true},
		},
		{
			name: "quoted empty name",
			tag:  "'',maxsize:1",
			want: Options{Name: "", HasName: true, MaxSize: 1, HasMaxSize: true},
		},
		{
			name: "unknown option",
			tag:  "items,future,maxsize:2",
			want: Options{Name: "items", HasName: true, MaxSize: 2, HasMaxSize: true},
		},
		{
			name: "unknown numeric option value",
			tag:  "items,future:1,maxsize:2",
			want: Options{Name: "items", HasName: true, MaxSize: 2, HasMaxSize: true},
		},
		{
			name: "unknown identifier option value",
			tag:  "items,future:value,maxsize:2",
			want: Options{Name: "items", HasName: true, MaxSize: 2, HasMaxSize: true},
		},
		{
			name: "unknown quoted option value",
			tag:  "items,future:'one,two',maxsize:2",
			want: Options{Name: "items", HasName: true, MaxSize: 2, HasMaxSize: true},
		},
		{name: "unknown missing option value", tag: "items,future:", wantErr: "missing value"},
		{name: "underscore spelling", tag: "items,max_size:2", wantErr: "specify \"maxsize\""},
		{name: "capital spelling", tag: "items,MaxSize:2", wantErr: "specify \"maxsize\""},
		{name: "quoted option", tag: "items,'maxsize':2", wantErr: "unnecessarily quoted"},
		{name: "equals separator", tag: "items,maxsize=2", wantErr: "missing value"},
		{name: "missing value", tag: "items,maxsize:", wantErr: "missing value"},
		{name: "negative value", tag: "items,maxsize:-1", wantErr: "invalid value"},
		{name: "duplicate", tag: "items,maxsize:1,maxsize:2", wantErr: "duplicate"},
		{name: "trailing comma", tag: "items,maxsize:1,", wantErr: "trailing"},
		{name: "unquoted dash name", tag: "-,maxsize:1", wantErr: "must be quoted"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.tag)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
