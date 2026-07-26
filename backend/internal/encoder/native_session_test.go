//go:build native && cgo

package encoder

import (
	"reflect"
	"testing"
)

func TestSelectAudioChannels(t *testing.T) {
	input := []int16{10, 20, 30, 40}
	tests := []struct {
		mode AudioMode
		want []int16
	}{
		{AudioModeBoth, []int16{10, 20, 30, 40}},
		{AudioModeMain, []int16{10, 10, 30, 30}},
		{AudioModeSub, []int16{20, 20, 40, 40}},
	}
	for _, test := range tests {
		if got := selectAudioChannels(input, 2, test.mode); !reflect.DeepEqual(got, test.want) {
			t.Errorf("mode %s = %v, want %v", test.mode, got, test.want)
		}
	}
}

func TestNewSubtitlePipeDisabledReturnsNilInterface(t *testing.T) {
	reader, writer := newSubtitlePipe(false)
	if reader != nil || writer != nil {
		t.Fatalf("disabled subtitle pipe = (%v, %v), want nil values", reader, writer)
	}
}

func TestNewSubtitlePipeEnabledReturnsPipe(t *testing.T) {
	reader, writer := newSubtitlePipe(true)
	if reader == nil || writer == nil {
		t.Fatalf("enabled subtitle pipe = (%v, %v), want pipe values", reader, writer)
	}
	_ = reader.Close()
	_ = writer.Close()
}
