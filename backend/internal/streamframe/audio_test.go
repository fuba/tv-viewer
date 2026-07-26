package streamframe

import (
	"bytes"
	"testing"
	"time"
)

func TestAudioRoundTripWithPTS(t *testing.T) {
	var buffer bytes.Buffer
	want := Audio{Data: []byte{1, 2, 3}, Duration: 20 * time.Millisecond, PTS: 90_000, HasPTS: true}
	if err := WriteAudio(&buffer, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadAudio(&buffer)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Data, want.Data) || got.Duration != want.Duration || got.PTS != want.PTS || !got.HasPTS {
		t.Fatalf("ReadAudio() = %+v, want %+v", got, want)
	}
}
