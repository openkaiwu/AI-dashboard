package codex

import "time"

// NewsFreshFor is how long a Tibo/news check stays valid before UI and advisor treat it as stale.
// The radar automation re-checks every 15 minutes; 45 minutes is 3 missed cycles.
const NewsFreshFor = 45 * time.Minute
