package codex

import "time"

// NewsFreshFor is how long a Tibo/news check stays valid before UI and advisor treat it as stale.
const NewsFreshFor = 3 * time.Hour
