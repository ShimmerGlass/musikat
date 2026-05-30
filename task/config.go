package task

import "time"

type Config struct {
	Enabled  bool          `env:"ENABLED, default=true"`
	Interval time.Duration `env:"INTERVAL, default=24h"`
}
