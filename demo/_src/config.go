package config

import "os"

func Load() error {
	dsn := os.Getenv("DATABASE_URL")
	key := os.Getenv("STRIPE_SECRET_KEY")
	level := os.Getenv("LOG_LEVEL")
	_ = dsn
	_ = key
	_ = level
	return nil
}
