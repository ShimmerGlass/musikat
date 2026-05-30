package server

type Config struct {
	ListenAddr string `yaml:"listen_addr"`
	JWTKey     string `yaml:"jwt_key"`
}
