package encoder

const broadcastPictureTicks = uint64(3003)
const broadcastPTSMask = uint64(1<<33 - 1)

type presentationTimestamp struct {
	pts   uint64
	order int64
	valid bool
}

type presentationTimeline struct {
	pending   []presentationTimestamp
	lastInput uint64
	lastOrder int64
	hasInput  bool
}

func (t *presentationTimeline) Add(pts uint64, valid bool) {
	if valid {
		pts &= broadcastPTSMask
		if t.hasInput {
			delta := (pts - t.lastInput) & broadcastPTSMask
			if delta > broadcastPTSMask/2 {
				t.lastOrder += int64(delta) - int64(broadcastPTSMask+1)
			} else {
				t.lastOrder += int64(delta)
			}
		} else {
			t.lastOrder = int64(pts)
		}
		t.lastInput, t.hasInput = pts, true
	} else if t.hasInput {
		t.lastInput = (t.lastInput + broadcastPictureTicks) & broadcastPTSMask
		t.lastOrder += int64(broadcastPictureTicks)
		pts, valid = t.lastInput, true
	}
	t.pending = append(t.pending, presentationTimestamp{pts: pts, order: t.lastOrder, valid: valid})
}

func (t *presentationTimeline) Pop() (uint64, bool) {
	if len(t.pending) == 0 {
		return 0, false
	}
	selected := 0
	for i := 1; i < len(t.pending); i++ {
		if t.pending[i].valid && (!t.pending[selected].valid || t.pending[i].order < t.pending[selected].order) {
			selected = i
		}
	}
	value := t.pending[selected]
	t.pending = append(t.pending[:selected], t.pending[selected+1:]...)
	return value.pts, value.valid
}
