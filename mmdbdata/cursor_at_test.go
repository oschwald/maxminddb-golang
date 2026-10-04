package mmdbdata_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

func TestCursorAtPreservesDecoderPosition(t *testing.T) {
	buffer := []byte{0x45, 'h', 'e', 'l', 'l', 'o', 0x45, 'w', 'o', 'r', 'l', 'd', 0x20, 0, 0x20, 6}
	d := mmdbdata.NewDecoder(buffer, 6)
	retained := d.CursorAt(0)
	for _, test := range []struct {
		offset uint
		want   string
	}{
		{0, "hello"}, {6, "world"}, {12, "hello"}, {14, "world"},
	} {
		value, _, err := d.CursorAt(test.offset).ReadString()
		require.NoError(t, err)
		require.Equal(t, test.want, value)
		require.Equal(t, uint(6), d.Offset())
	}
	_, next, err := d.Cursor().ReadString()
	require.NoError(t, err)
	require.NoError(t, d.Advance(next))
	value, _, err := retained.ReadString()
	require.NoError(t, err)
	require.Equal(t, "hello", value, "advancing the decoder must not move retained cursors")
}

func TestCursorAtPreservesSuccessorChecks(t *testing.T) {
	buffer := []byte{0x45, 'h', 'e', 'l', 'l', 'o', 0x45, 'w', 'o', 'r', 'l', 'd'}
	d := mmdbdata.NewDecoder(buffer, 0)
	require.Error(t, d.Advance(d.CursorAt(6)), "an arbitrary offset is not a proven successor")
	_, unrelated, err := d.CursorAt(6).ReadString()
	require.NoError(t, err)
	require.Error(t, d.Advance(unrelated))
	other := mmdbdata.NewDecoder(buffer, 0)
	_, foreign, err := other.CursorAt(0).ReadString()
	require.NoError(t, err)
	require.Error(t, d.Advance(foreign))
	_, next, err := d.CursorAt(0).ReadString()
	require.NoError(t, err)
	require.NoError(t, d.Advance(next))
	value, err := d.ReadString()
	require.NoError(t, err)
	require.Equal(t, "world", value)

	invalid := mmdbdata.NewDecoder(buffer, ^uint(0))
	require.Error(
		t,
		invalid.Advance(invalid.CursorAt(0)),
		"offset wraparound must not accept an unproven cursor",
	)
}

func TestCursorAtReadErrors(t *testing.T) {
	for _, test := range []struct {
		name   string
		buffer []byte
		offset uint
	}{
		{"end", []byte{0x41, 'x'}, 2},
		{"past_end", []byte{0x41, 'x'}, 3},
		{"maximum_offset", []byte{0x41, 'x'}, ^uint(0)},
		{"truncated", []byte{0x45, 'x'}, 0},
		{"wrong_kind", []byte{0x01, 0x07}, 0},
		{"invalid_pointer", []byte{0x20, 127}, 0},
		{"pointer_chain", []byte{0x20, 2, 0x20, 0}, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			d := mmdbdata.NewDecoder(test.buffer, 0)
			_, _, err := d.CursorAt(test.offset).ReadString()
			require.Error(t, err)
		})
	}
	var zero mmdbdata.Decoder
	_, _, err := zero.CursorAt(0).ReadString()
	require.Error(t, err)
}

func TestCursorAtSharesWarmCache(t *testing.T) {
	buffer := []byte{0x45, 'h', 'e', 'l', 'l', 'o', 0x20, 0, 0x20, 0}
	d := mmdbdata.NewDecoder(buffer, 0, mmdbdata.WithStringCache())
	for range 3 {
		value, err := d.ReadString()
		require.NoError(t, err)
		require.Equal(t, "hello", value)
	}
	var value string
	allocs := testing.AllocsPerRun(100, func() {
		for _, offset := range []uint{0, 6, 8} {
			var err error
			value, _, err = d.CursorAt(offset).ReadString()
			if err != nil {
				t.Fatal(err)
			}
		}
	})
	require.Zero(t, allocs, "offset cursors must reuse the decoder's warmed strings")
	require.Equal(t, "hello", value)

	other := mmdbdata.NewDecoder(
		[]byte{0x45, 'w', 'o', 'r', 'l', 'd'},
		0,
		mmdbdata.WithStringCache(),
	)
	for range 4 {
		value, _, err := other.CursorAt(0).ReadString()
		require.NoError(t, err)
		require.Equal(t, "world", value)
		value, _, err = d.CursorAt(0).ReadString()
		require.NoError(t, err)
		require.Equal(t, "hello", value)
	}
}

func TestCursorAtConcurrentReads(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(fmt.Sprintf("cached_%t", cached), func(t *testing.T) {
			buffer := []byte{
				0x45,
				'h',
				'e',
				'l',
				'l',
				'o',
				0x45,
				'w',
				'o',
				'r',
				'l',
				'd',
				0x20,
				0,
				0x20,
				6,
			}
			var options []mmdbdata.DecoderOption
			if cached {
				options = append(options, mmdbdata.WithStringCache())
			}
			d := mmdbdata.NewDecoder(buffer, 0, options...)
			offsets := []uint{0, 6, 12, 14}
			want := []string{"hello", "world", "hello", "world"}
			start := make(chan struct{})
			var workers sync.WaitGroup
			for worker := range 12 {
				workers.Go(func() {
					<-start
					for i := range 1000 {
						index := (worker + i) % len(offsets)
						value, _, err := d.CursorAt(offsets[index]).ReadString()
						if err != nil || value != want[index] {
							t.Errorf("offset %d: got %q, err %v", offsets[index], value, err)
							return
						}
					}
				})
			}
			close(start)
			workers.Wait()
			require.Zero(t, d.Offset())
		})
	}
}

func ExampleNewDecoder_cursorAt() {
	buffer := []byte{0x45, 'h', 'e', 'l', 'l', 'o', 0x45, 'w', 'o', 'r', 'l', 'd'}
	d := mmdbdata.NewDecoder(buffer, 0, mmdbdata.WithStringCache())
	for _, offset := range []uint{6, 0, 6} {
		value, _, err := d.CursorAt(offset).ReadString()
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(value)
	}
	// Output:
	// world
	// hello
	// world
}
