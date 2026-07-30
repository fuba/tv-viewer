//go:build native && cgo

// dump-captions decodes the ARIB captions of a recorded MPEG-TS and prints the
// drawn regions, so the layout the broadcaster sends can be inspected directly.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/fuba/tv-viewer/internal/mpegts"
	"github.com/fuba/tv-viewer/internal/nativecaption"
)

func main() {
	limit := flag.Int("captions", 20, "stop after this many captions")
	flag.Parse()
	if flag.NArg() != 1 {
		log.Fatal("usage: dump-captions [-captions N] <file.ts>")
	}
	file, err := os.Open(flag.Arg(0))
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	decoders := make(map[uint16]*nativecaption.Decoder)
	defer func() {
		for _, decoder := range decoders {
			_ = decoder.Close()
		}
	}()

	seen := 0
	packets := map[byte]int{}
	failures := 0
	empty := 0
	demuxer := mpegts.NewDemuxer()
	_, err = demuxer.ReadPES(context.Background(), file, func(packet mpegts.PESPacket) error {
		packets[packet.Stream.StreamType]++
		if packet.Stream.StreamType != 0x06 || seen >= *limit {
			return nil
		}
		decoder := decoders[packet.PID]
		if decoder == nil {
			decoder, err = nativecaption.NewDecoder()
			if err != nil {
				return err
			}
			decoders[packet.PID] = decoder
		}
		caption, parsed, err := decoder.DecodePES(packet.Payload)
		if err != nil {
			failures++
			return nil //nolint:nilerr // malformed captions are skipped like in the pipeline
		}
		if !parsed || caption.Text == "" {
			empty++
			return nil
		}
		seen++
		fmt.Printf("\n#%d PID=%#x duration=%s plane=%dx%d text=%q\n",
			seen, packet.PID, caption.Duration, caption.Plane.Width, caption.Plane.Height, caption.Text)
		for i, row := range caption.Rows {
			fmt.Printf("  row[%d] bottom=%d text=%q\n", i, row.Bottom, row.Text)
			for j, span := range row.Spans {
				fmt.Printf("    span[%d] left=%d advance=%d font=%dx%d space=%d fg=%d bg=%d alpha=%d/%d text=%q\n",
					j, span.Left, span.Advance, span.FontWidth, span.FontHeight, span.HorizontalSpace,
					span.Foreground, span.Background, span.ForegroundAlpha, span.BackgroundAlpha, span.Text)
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("stopped: %v", err)
	}
	fmt.Printf("\nPES packets by stream type: %v (caption decode failures=%d empty=%d)\n", packets, failures, empty)
}
