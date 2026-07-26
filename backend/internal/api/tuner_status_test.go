package api

import (
	"testing"

	"github.com/fuba/tv-viewer/internal/mirakurun"
)

func TestSummarizeTunersSeparatesViewerEPGAndOtherUsage(t *testing.T) {
	tuners := []mirakurun.Tuner{
		{
			Types: []string{"GR"}, IsUsing: true,
			Users: []mirakurun.TunerUser{
				{ID: "Mirakurun:getEPG()"},
				{Agent: "tv-viewer/1.0", URL: "/api/channels/GR/16/stream"},
			},
		},
		{
			Types: []string{"GR"}, IsUsing: true,
			Users: []mirakurun.TunerUser{{Agent: "BBA/0.1.0"}},
		},
		{
			Types: []string{"GR"}, IsFree: true,
			Users: []mirakurun.TunerUser{{Agent: "tv-viewer/1.0"}},
		},
		{
			Types: []string{"BS", "CS"}, IsUsing: true,
			Users: []mirakurun.TunerUser{{ID: "Mirakurun:getEPG()"}},
		},
	}

	got := summarizeTuners(tuners)
	if len(got) != 2 {
		t.Fatalf("status count = %d, want 2", len(got))
	}
	if got[0] != (TunerStatus{Type: "GR", Total: 3, Using: 2, Free: 1, ViewerUsing: 1, EPGUsing: 1, OtherUsing: 1, ViewerShared: 1}) {
		t.Fatalf("GR status = %+v", got[0])
	}
	if got[1] != (TunerStatus{Type: "BS/CS", Total: 1, Using: 1, EPGUsing: 1}) {
		t.Fatalf("BS/CS status = %+v", got[1])
	}
}
