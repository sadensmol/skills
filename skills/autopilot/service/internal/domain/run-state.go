package domain

// RunState is the wave progress a run writes to run/state.json in its run folder.
type RunState struct {
	Wave int `json:"wave"`
	// Parts is the number of parts in the wave.
	Parts int `json:"parts"`
	// Running holds the ids of the parts that work now.
	Running []string `json:"running"`
}
