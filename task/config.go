package task

import "time"

type Config struct {
	Enabled  bool          `env:"ENABLED, default=true"`
	Interval time.Duration `env:"INTERVAL, default=24h"`
	// CheckInterval is the tick at which the artists due for a MusicBrainz
	// check are processed. Interval is how often each artist is checked at
	// least, so the work is spread over Interval in CheckInterval batches.
	CheckInterval time.Duration `env:"CHECK_INTERVAL, default=15m"`
}
