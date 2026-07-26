package streamframe

import (
	"bytes"
	"testing"
	"time"
)

func TestVideoRoundTripPreservesPTS(t *testing.T) {
	want := Video{Data: []byte{1, 2, 3}, Duration: 16_683 * time.Microsecond, PTS: 123_456, HasPTS: true}
	var encoded bytes.Buffer
	if err := WriteVideo(&encoded, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadVideo(&encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Data, want.Data) || got.Duration != want.Duration || !got.HasPTS || got.PTS != want.PTS {
		t.Fatalf("round trip = %#v, want %#v", got, want)
	}
}

func TestVideoRoundTripWithoutPTS(t *testing.T) {
	var encoded bytes.Buffer
	if err := WriteVideo(&encoded, Video{Data: []byte{1}, Duration: 33 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadVideo(&encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.HasPTS {
		t.Fatalf("unexpected PTS %d", got.PTS)
	}
}
