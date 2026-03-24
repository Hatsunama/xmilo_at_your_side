package game

type IdleBeat struct {
	AnchorID  string
	Facing    string
	Style     string
	HoldTicks int
}

type IdleLoop struct {
	Name  string
	Room  RoomID
	Beats []IdleBeat
}

type IdleDirector struct {
	roomLastLoop   map[RoomID]int
	currentRoom    RoomID
	currentLoop    *IdleLoop
	currentBeat    int
	remainingTicks int
}

func NewIdleDirector() *IdleDirector {
	return &IdleDirector{
		roomLastLoop: map[RoomID]int{},
	}
}

func (d *IdleDirector) Reset() {
	d.currentLoop = nil
	d.currentBeat = 0
	d.remainingTicks = 0
}

func (d *IdleDirector) Tick(g *Game) {
	if g.currentState != "idle" || g.milo.walkActive || len(g.currentRoute) > 0 {
		d.Reset()
		return
	}

	room := CanonicalRoomID(g.currentRoomID)
	loops := idleLoopsForRoom(room)
	if len(loops) == 0 {
		d.Reset()
		return
	}

	if d.currentLoop == nil || d.currentRoom != room {
		d.currentRoom = room
		d.selectLoop(room, loops)
	}

	if d.currentLoop == nil || len(d.currentLoop.Beats) == 0 {
		return
	}

	if d.remainingTicks <= 0 {
		beat := d.currentLoop.Beats[d.currentBeat]
		if g.currentAnchor != beat.AnchorID {
			toX, toY := g.cam.AnchorToScreen(beat.AnchorID)
			duration := WalkDurationMS(g.currentAnchor, beat.AnchorID)
			if duration == 0 {
				duration = 450
			}
			g.milo.StartWalk(toX, toY, WalkFacing(g.currentAnchor, beat.AnchorID), duration)
			g.currentAnchor = beat.AnchorID
		}
		g.milo.SetState("idle", g.currentRoomID, beat.Facing, true)
		g.milo.SetIdleStyle(beat.Style)
		d.remainingTicks = beat.HoldTicks
		d.currentBeat = (d.currentBeat + 1) % len(d.currentLoop.Beats)
		return
	}

	d.remainingTicks--
}

func (d *IdleDirector) selectLoop(room RoomID, loops []IdleLoop) {
	last := d.roomLastLoop[room]
	next := 0
	if len(loops) > 1 {
		next = (last + 1) % len(loops)
	}
	selected := loops[next]
	d.currentLoop = &selected
	d.currentBeat = 0
	d.remainingTicks = 0
	d.roomLastLoop[room] = next
}

func idleLoopsForRoom(room RoomID) []IdleLoop {
	switch room {
	case RoomMainHall:
		return []IdleLoop{
			{
				Name: "main_hall_center_watch",
				Room: RoomMainHall,
				Beats: []IdleBeat{
					{AnchorID: "main_hall_center", Facing: "s", Style: "home_ready", HoldTicks: 56},
					{AnchorID: "main_hall_left", Facing: "e", Style: "skeptical_shift", HoldTicks: 38},
					{AnchorID: "main_hall_center", Facing: "s", Style: "robe_settle", HoldTicks: 46},
				},
			},
			{
				Name: "main_hall_outward_wait",
				Room: RoomMainHall,
				Beats: []IdleBeat{
					{AnchorID: "main_hall_right", Facing: "w", Style: "attentive_wait", HoldTicks: 34},
					{AnchorID: "main_hall_door", Facing: "n", Style: "outward_glance", HoldTicks: 52},
					{AnchorID: "main_hall_right", Facing: "s", Style: "hat_tilt", HoldTicks: 32},
				},
			},
			{
				Name: "main_hall_small_pace",
				Room: RoomMainHall,
				Beats: []IdleBeat{
					{AnchorID: "main_hall_left", Facing: "s", Style: "ready_shift", HoldTicks: 28},
					{AnchorID: "main_hall_center", Facing: "e", Style: "attentive_wait", HoldTicks: 26},
					{AnchorID: "main_hall_right", Facing: "w", Style: "skeptical_shift", HoldTicks: 30},
					{AnchorID: "main_hall_center", Facing: "s", Style: "ready_shift", HoldTicks: 36},
				},
			},
		}
	case RoomArchive:
		return []IdleLoop{
			{
				Name: "archive_memory_check",
				Room: RoomArchive,
				Beats: []IdleBeat{
					{AnchorID: "archive_lectern", Facing: "n", Style: "calm_archive", HoldTicks: 50},
					{AnchorID: "archive_crystal", Facing: "n", Style: "observing", HoldTicks: 42},
					{AnchorID: "archive_lectern", Facing: "s", Style: "archive_read", HoldTicks: 34},
				},
			},
			{
				Name: "archive_scroll_drift",
				Room: RoomArchive,
				Beats: []IdleBeat{
					{AnchorID: "archive_shelf", Facing: "e", Style: "archive_read", HoldTicks: 44},
					{AnchorID: "archive_lectern", Facing: "w", Style: "calm_archive", HoldTicks: 34},
					{AnchorID: "archive_crystal", Facing: "n", Style: "observing", HoldTicks: 30},
				},
			},
		}
	case RoomTrophy:
		return []IdleLoop{
			{
				Name: "trophy_proud_pass",
				Room: RoomTrophy,
				Beats: []IdleBeat{
					{AnchorID: "trophy_display", Facing: "n", Style: "proud", HoldTicks: 40},
					{AnchorID: "trophy_pedestal", Facing: "w", Style: "reflective_proud", HoldTicks: 44},
					{AnchorID: "trophy_display", Facing: "s", Style: "composed", HoldTicks: 30},
				},
			},
			{
				Name: "trophy_reflective_pause",
				Room: RoomTrophy,
				Beats: []IdleBeat{
					{AnchorID: "trophy_wall", Facing: "e", Style: "reflective_proud", HoldTicks: 34},
					{AnchorID: "trophy_display", Facing: "n", Style: "proud", HoldTicks: 28},
					{AnchorID: "trophy_pedestal", Facing: "s", Style: "composed", HoldTicks: 38},
				},
			},
		}
	default:
		return nil
	}
}
