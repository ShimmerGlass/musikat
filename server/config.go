package server

type Config struct {
	ListenAddr string `env:"LISTEN_ADDR, default=:8080"`
	JWTKey     string `env:"JWT_KEY"`
}
