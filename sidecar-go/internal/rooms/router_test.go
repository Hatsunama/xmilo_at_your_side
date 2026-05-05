package rooms

import "testing"

func TestResolveRoutesTaskIntents(t *testing.T) {
	tests := []struct {
		name       string
		prompt     string
		intentHint string
		want       Route
	}{
		{
			name:   "planning prompt routes to war room",
			prompt: "help me plan the next build",
			want:   Route{Intent: "planning", RoomID: "war_room", AnchorID: "war_room_table"},
		},
		{
			name:   "analysis prompt routes to library",
			prompt: "summarize this report",
			want:   Route{Intent: "analysis", RoomID: "library", AnchorID: "library_desk"},
		},
		{
			name:       "hint overrides prompt classification",
			prompt:     "write a spell",
			intentHint: " analysis ",
			want:       Route{Intent: "analysis", RoomID: "library", AnchorID: "library_desk"},
		},
		{
			name:   "casual conversation stays in main hall",
			prompt: "hello Milo",
			want:   Route{Intent: "casual_conversation", RoomID: "main_hall", AnchorID: "main_hall_center"},
		},
		{
			name:   "unknown prompt falls back to main hall",
			prompt: "something ambiguous",
			want:   Route{Intent: "general", RoomID: "main_hall", AnchorID: "main_hall_center"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(tt.prompt, tt.intentHint)
			if got != tt.want {
				t.Fatalf("Resolve() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
