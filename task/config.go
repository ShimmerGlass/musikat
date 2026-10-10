package task

import "time"

type Config struct {
	Enabled  bool          `env:"ENABLED, default=true"`
	Interval time.Duration `env:"INTERVAL, default=24h"`
	// CheckInterval is the tick at which the artists due for a MusicBrainz
	// check are processed. Interval is how often each artist is checked at
	// least, so the work is spread over Interval in CheckInterval batches.
	// A value <= 0 disables spreading: all due artists are checked every
	// Interval.
	CheckInterval time.Duration `env:"CHECK_INTERVAL, default=15m"`
}

// checkInterval is the effective tick of the scheduled MusicBrainz checks.
func (c Config) checkInterval() time.Duration {
	if c.CheckInterval <= 0 {
		return c.Interval
	}

	return c.CheckInterval
}
